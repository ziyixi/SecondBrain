package vectorstore

import (
	"math"
	"sort"
	"sync"
)

// Record represents a vector with payload.
type Record struct {
	ID      string
	Vector  []float32
	Payload map[string]string
}

// SearchHit represents a search result.
type SearchHit struct {
	ID      string
	Score   float32
	Payload map[string]string
}

// Store is the interface for vector storage backends.
type Store interface {
	Upsert(collection string, records []Record) error
	Search(collection string, vector []float32, topK int, filters map[string]string) ([]SearchHit, error)
	Delete(collection string, ids []string) (int, error)
	Count(collection string) int
}

// InMemoryStore is an in-memory vector store for development and testing.
type InMemoryStore struct {
	mu          sync.RWMutex
	collections map[string]map[string]Record
}

// NewInMemoryStore creates a new in-memory vector store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		collections: make(map[string]map[string]Record),
	}
}

// Upsert adds or updates records in a collection.
func (s *InMemoryStore) Upsert(collection string, records []Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.collections[collection]; !ok {
		s.collections[collection] = make(map[string]Record)
	}

	for _, r := range records {
		s.collections[collection][r.ID] = r
	}
	return nil
}

// Search finds the top-K most similar vectors using cosine similarity.
func (s *InMemoryStore) Search(collection string, vector []float32, topK int, filters map[string]string) ([]SearchHit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	coll, ok := s.collections[collection]
	if !ok {
		return nil, nil
	}

	type scored struct {
		id      string
		score   float32
		payload map[string]string
	}

	var results []scored
	for _, record := range coll {
		// Apply filters
		if filters != nil {
			match := true
			for k, v := range filters {
				if record.Payload[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		score := cosineSimilarity(vector, record.Vector)
		results = append(results, scored{
			id:      record.ID,
			score:   score,
			payload: record.Payload,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}

	hits := make([]SearchHit, topK)
	for i := 0; i < topK; i++ {
		hits[i] = SearchHit{
			ID:      results[i].id,
			Score:   results[i].score,
			Payload: results[i].payload,
		}
	}

	return hits, nil
}

// Delete removes records from a collection.
func (s *InMemoryStore) Delete(collection string, ids []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	coll, ok := s.collections[collection]
	if !ok {
		return 0, nil
	}

	deleted := 0
	for _, id := range ids {
		if _, exists := coll[id]; exists {
			delete(coll, id)
			deleted++
		}
	}
	return deleted, nil
}

// Count returns the number of records in a collection.
func (s *InMemoryStore) Count(collection string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.collections[collection])
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return float32(dot / denom)
}
