package memory

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

const (
	defaultCollection = "knowledge"
	maxTextChars      = 6000 // ~1500 tokens at ~4 chars/token
	rrfK              = 60
)

// NotionPageFetcher fetches raw text for a Notion page by ID.
type NotionPageFetcher interface {
	GetPageText(ctx context.Context, pageID string) (string, error)
}

// QdrantKnowledgeBase implements KnowledgeBase with Qdrant and optional hybrid (dense + keyword) scoring.
type QdrantKnowledgeBase struct {
	qc          *qdrant.Client
	collection  string
	embedder    func(ctx context.Context, text string) ([]float32, error)
	fetcher     NotionPageFetcher
	vectorSize  uint64
	mu          sync.Mutex
	initialized bool
}

// NewQdrantKnowledgeBase creates a knowledge base backed by Qdrant.
// QDRANT_HOST, QDRANT_PORT (default 6334), QDRANT_COLLECTION (default "knowledge") from env.
func NewQdrantKnowledgeBase(embedder func(ctx context.Context, text string) ([]float32, error), fetcher NotionPageFetcher) (*QdrantKnowledgeBase, error) {
	host := os.Getenv("QDRANT_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 6334
	if p := os.Getenv("QDRANT_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	collection := os.Getenv("QDRANT_COLLECTION")
	if collection == "" {
		collection = defaultCollection
	}
	client, err := qdrant.NewClient(&qdrant.Config{Host: host, Port: port})
	if err != nil {
		return nil, err
	}
	return &QdrantKnowledgeBase{
		qc:         client,
		collection: collection,
		embedder:   embedder,
		fetcher:    fetcher,
	}, nil
}

// Close closes the Qdrant client.
func (q *QdrantKnowledgeBase) Close() error {
	return q.qc.Close()
}

func (q *QdrantKnowledgeBase) ensureCollection(ctx context.Context, vectorSize uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.initialized && q.vectorSize == vectorSize {
		return nil
	}
	n := uint64(2)
	err := q.qc.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: q.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     vectorSize,
			Distance: qdrant.Distance_Cosine,
		}),
		OptimizersConfig: &qdrant.OptimizersConfigDiff{DefaultSegmentNumber: &n},
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			// Collection already exists, treat as success
		} else {
			return err
		}
	}
	q.vectorSize = vectorSize
	q.initialized = true
	return nil
}

// Upsert stores or updates a document by Notion page ID (embedding is generated lazily on first Search if needed).
func (q *QdrantKnowledgeBase) Upsert(ctx context.Context, notionPageID string, text string) error {
	vec, err := q.embedder(ctx, text)
	if err != nil {
		return err
	}
	if err := q.ensureCollection(ctx, uint64(len(vec))); err != nil {
		return err
	}
	pointID := notionPageID
	if _, err := uuid.Parse(notionPageID); err != nil {
		pointID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(notionPageID)).String()
	}
	trunc := truncateText(text, maxTextChars)
	wait := true
	_, err = q.qc.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: q.collection,
		Wait:           &wait,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewID(pointID),
				Vectors: qdrant.NewVectors(vec...),
				Payload: qdrant.NewValueMap(map[string]any{
					"notion_page_id": notionPageID,
					"text":           trunc,
				}),
			},
		},
	})
	return err
}

// Search runs hybrid search: dense vector search in Qdrant + simple keyword score over payload text, then RRF merge.
func (q *QdrantKnowledgeBase) Search(ctx context.Context, query string, limit int) ([]DocumentHit, error) {
	if limit <= 0 {
		limit = 5
	}
	queryVec, err := q.embedder(ctx, query)
	if err != nil {
		return nil, err
	}
	if err := q.ensureCollection(ctx, uint64(len(queryVec))); err != nil {
		return nil, err
	}
	// Fetch more for hybrid re-rank; then we'll do keyword scoring and RRF
	prefetchLimit := limit * 3
	if prefetchLimit < 20 {
		prefetchLimit = 20
	}
	scored, err := q.qc.Query(ctx, &qdrant.QueryPoints{
		CollectionName: q.collection,
		Query:          qdrant.NewQuery(queryVec...),
		Limit:          ptr(uint64(prefetchLimit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}
	// Build keyword score (simple token overlap) and RRF merge with dense rank
	queryTokens := tokenizeLower(query)
	type scoredDoc struct {
		pageID string
		text   string
		rrf    float64
	}
	byID := make(map[string]*scoredDoc)
	for rank, sp := range scored {
		pageID := ""
		text := ""
		if sp.Payload != nil {
			pageID = valueAsString(sp.Payload, "notion_page_id")
			text = valueAsString(sp.Payload, "text")
		}
		if pageID == "" && sp.Id != nil {
			pageID = sp.Id.GetUuid()
			if pageID == "" {
				pageID = fmt.Sprintf("%d", sp.Id.GetNum())
			}
		}
		if pageID == "" {
			continue
		}
		rrfDense := 1.0 / (float64(rrfK) + float64(rank+1))
		if d, ok := byID[pageID]; ok {
			d.rrf += rrfDense
			continue
		}
		kwScore := keywordScore(queryTokens, text)
		rrfKw := 1.0 / (float64(rrfK) + 1.0/(kwScore+0.01))
		byID[pageID] = &scoredDoc{pageID: pageID, text: text, rrf: rrfDense + rrfKw}
	}
	// Sort by RRF and take top limit; optionally fetch full text via fetcher
	var out []DocumentHit
	for _, d := range byID {
		text := d.text
		if q.fetcher != nil {
			full, err := q.fetcher.GetPageText(ctx, d.pageID)
			if err == nil && full != "" {
				text = truncateText(full, maxTextChars)
			}
		}
		out = append(out, DocumentHit{NotionPageID: d.pageID, Text: text, Score: d.rrf})
	}
	// Sort by score desc and trim to limit
	sortByScoreDesc(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func sortByScoreDesc(hits []DocumentHit) {
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Score > hits[i].Score {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
}

var tokenRe = regexp.MustCompile(`[a-zA-Z0-9]+`)

func tokenizeLower(s string) []string {
	lower := strings.ToLower(s)
	matches := tokenRe.FindAllString(lower, -1)
	seen := make(map[string]bool)
	var out []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func keywordScore(queryTokens []string, doc string) float64 {
	if len(queryTokens) == 0 {
		return 0
	}
	docLower := strings.ToLower(doc)
	var hits int
	for _, t := range queryTokens {
		if strings.Contains(docLower, t) {
			hits++
		}
	}
	return float64(hits) / float64(len(queryTokens))
}

func truncateText(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "..."
}

func ptr[V any](v V) *V { return &v }

// valueAsString extracts a string from Qdrant payload map (map[string]*qdrant.Value).
func valueAsString(payload map[string]*qdrant.Value, key string) string {
	if payload == nil {
		return ""
	}
	v, ok := payload[key]
	if !ok || v == nil {
		return ""
	}
	// qdrant.Value is a protobuf oneof; check for string kind
	if s := v.GetStringValue(); s != "" {
		return s
	}
	return ""
}
