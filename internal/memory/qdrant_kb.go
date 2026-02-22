package memory

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"github.com/yourusername/secondbrain/internal/config"
)

const defaultCollection = "knowledge"

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
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			port = n
		}
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

// Embed returns the vector for the given text (uses the same embedder as Upsert/Search).
func (q *QdrantKnowledgeBase) Embed(ctx context.Context, text string) ([]float32, error) {
	return q.embedder(ctx, text)
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

// PointTypeTopic and PointTypeContent distinguish category (topic) vectors from content chunk vectors for dynamic categorization.
const (
	PointTypeTopic   = "topic"
	PointTypeContent = "content"
)

// Upsert stores in Qdrant only the embedding vector and the Notion page ID.
// The full knowledge content lives in Notion; Qdrant is a lightweight vector index for retrieval.
// We use text only to compute the embedding; no document text is stored in Qdrant.
func (q *QdrantKnowledgeBase) Upsert(ctx context.Context, notionPageID string, text string) error {
	return q.UpsertPointWithType(ctx, notionPageID, text, PointTypeContent)
}

// UpsertPointWithType stores a vector + notion_page_id + type (topic or content) in Qdrant.
// Use type "topic" for category/title embeddings, "content" for chunk embeddings.
func (q *QdrantKnowledgeBase) UpsertPointWithType(ctx context.Context, notionPageID string, text string, pointType string) error {
	vec, err := q.embedder(ctx, text)
	if err != nil {
		return err
	}
	if err := q.ensureCollection(ctx, uint64(len(vec))); err != nil {
		return err
	}
	pointID := uuid.New().String()
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
					"type":           pointType,
				}),
			},
		},
	})
	return err
}

// FindSimilarTopic runs a vector search restricted to payload type "topic" with a score threshold.
// Returns the best matching notion_page_id and its score if score >= minScore (e.g. 0.85 for cosine similarity).
func (q *QdrantKnowledgeBase) FindSimilarTopic(ctx context.Context, topicVec []float32, minScore float32) (notionPageID string, score float32, ok bool) {
	if err := q.ensureCollection(ctx, uint64(len(topicVec))); err != nil {
		return "", 0, false
	}
	scored, err := q.qc.Query(ctx, &qdrant.QueryPoints{
		CollectionName: q.collection,
		Query:          qdrant.NewQuery(topicVec...),
		Limit:          ptr(uint64(1)),
		ScoreThreshold: &minScore,
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{qdrant.NewMatchKeyword("type", PointTypeTopic)},
		},
		WithPayload: qdrant.NewWithPayload(true),
	})
	if err != nil || len(scored) == 0 {
		return "", 0, false
	}
	sp := scored[0]
	pageID := valueAsString(sp.Payload, "notion_page_id")
	return pageID, sp.Score, pageID != ""
}

// Search runs vector search in Qdrant (vectors + notion_page_id only), then fetches full text from Notion.
// Notion is the source of truth for knowledge; Qdrant only holds the vector → page ID index.
// If fetcher is set, we fetch each hit's text from Notion and optionally re-rank by keyword (hybrid).
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
	prefetchLimit := limit * 3
	if min := config.KBSearchPrefetchMin(); prefetchLimit < min {
		prefetchLimit = min
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
	type scoredDoc struct {
		pageID string
		rrf    float64
	}
	rrfK := config.KBRRFK()
	byID := make(map[string]*scoredDoc)
	for rank, sp := range scored {
		pageID := valueAsString(sp.Payload, "notion_page_id")
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
		byID[pageID] = &scoredDoc{pageID: pageID, rrf: rrfDense}
	}
	var out []DocumentHit
	for _, d := range byID {
		text := ""
		if q.fetcher != nil {
			full, err := q.fetcher.GetPageText(ctx, d.pageID)
			if err == nil && full != "" {
				text = truncateText(full, config.KBMaxTextChars())
			}
		}
		out = append(out, DocumentHit{NotionPageID: d.pageID, Text: text, Score: d.rrf})
	}
	sortByScoreDesc(out)
	if len(out) > limit {
		out = out[:limit]
	}
	// Optional: re-rank by keyword over fetched text (hybrid)
	if q.fetcher != nil && len(out) > 0 {
		queryTokens := tokenizeLower(query)
		for _, h := range out {
			kwScore := keywordScore(queryTokens, h.Text)
			rrfKw := 1.0 / (float64(rrfK) + 1.0/(kwScore+0.01))
			h.Score += rrfKw
		}
		sortByScoreDesc(out)
	}
	return out, nil
}

func sortByScoreDesc(hits []DocumentHit) {
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
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
