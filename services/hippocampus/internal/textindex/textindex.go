package textindex

import (
	"math"
	"sort"
	"strings"
	"sync"
)

// Document represents an indexed document.
type Document struct {
	ID       string
	Content  string
	Metadata map[string]string
}

// SearchHit represents a full-text search result with a BM25 score.
type SearchHit struct {
	ID       string
	Score    float64
	Content  string
	Metadata map[string]string
}

// Index is an in-memory BM25 full-text search index.
// Inspired by qmd's BM25 search via SQLite FTS5.
type Index struct {
	mu   sync.RWMutex
	docs map[string]*indexedDoc // collection -> id -> doc
	// BM25 parameters
	k1 float64
	b  float64
}

type indexedDoc struct {
	id       string
	content  string
	metadata map[string]string
	terms    map[string]int // term -> frequency
	length   int            // total word count
}

// New creates a new full-text search index with default BM25 parameters.
func New() *Index {
	return &Index{
		docs: make(map[string]*indexedDoc),
		k1:   1.2,
		b:    0.75,
	}
}

// Add indexes a document for full-text search within a collection.
func (idx *Index) Add(collection string, doc Document) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	key := collection + "\x00" + doc.ID
	terms := tokenize(doc.Content)
	freq := termFrequency(terms)

	idx.docs[key] = &indexedDoc{
		id:       doc.ID,
		content:  doc.Content,
		metadata: doc.Metadata,
		terms:    freq,
		length:   len(terms),
	}
}

// Delete removes a document from the index.
func (idx *Index) Delete(collection string, id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.docs, collection+"\x00"+id)
}

// Search performs BM25-ranked full-text search within a collection.
func (idx *Index) Search(collection, query string, topK int, filters map[string]string) []SearchHit {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Collect docs for this collection
	var collDocs []*indexedDoc
	for key, doc := range idx.docs {
		if strings.HasPrefix(key, collection+"\x00") {
			collDocs = append(collDocs, doc)
		}
	}

	if len(collDocs) == 0 {
		return nil
	}

	// Compute average document length
	avgDL := idx.avgDocLength(collDocs)
	n := float64(len(collDocs))

	// Compute IDF for each query term
	idf := make(map[string]float64)
	for _, term := range queryTerms {
		df := 0
		for _, doc := range collDocs {
			if doc.terms[term] > 0 {
				df++
			}
		}
		// IDF formula: log((N - df + 0.5) / (df + 0.5) + 1)
		idf[term] = math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1)
	}

	// Score each document
	type scored struct {
		doc   *indexedDoc
		score float64
	}
	var results []scored
	for _, doc := range collDocs {
		// Apply filters
		if !matchFilters(doc.metadata, filters) {
			continue
		}

		score := 0.0
		for _, term := range queryTerms {
			tf := float64(doc.terms[term])
			dl := float64(doc.length)
			// BM25 formula
			num := tf * (idx.k1 + 1)
			denom := tf + idx.k1*(1-idx.b+idx.b*dl/avgDL)
			score += idf[term] * num / denom
		}

		if score > 0 {
			results = append(results, scored{doc: doc, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}

	// Normalize scores to 0-1 range
	maxScore := 0.0
	if len(results) > 0 {
		maxScore = results[0].score
	}

	hits := make([]SearchHit, topK)
	for i := 0; i < topK; i++ {
		normalizedScore := results[i].score
		if maxScore > 0 {
			normalizedScore = results[i].score / maxScore
		}
		hits[i] = SearchHit{
			ID:       results[i].doc.id,
			Score:    normalizedScore,
			Content:  results[i].doc.content,
			Metadata: results[i].doc.metadata,
		}
	}
	return hits
}

// Count returns the number of documents in a collection.
func (idx *Index) Count(collection string) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	count := 0
	prefix := collection + "\x00"
	for key := range idx.docs {
		if strings.HasPrefix(key, prefix) {
			count++
		}
	}
	return count
}

func (idx *Index) avgDocLength(docs []*indexedDoc) float64 {
	if len(docs) == 0 {
		return 0
	}
	total := 0
	for _, d := range docs {
		total += d.length
	}
	return float64(total) / float64(len(docs))
}

func matchFilters(metadata, filters map[string]string) bool {
	if len(filters) == 0 {
		return true
	}
	for k, v := range filters {
		if metadata[k] != v {
			return false
		}
	}
	return true
}

// tokenize splits text into lowercase terms.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	return words
}

// termFrequency counts the frequency of each term.
func termFrequency(terms []string) map[string]int {
	freq := make(map[string]int)
	for _, t := range terms {
		freq[t]++
	}
	return freq
}
