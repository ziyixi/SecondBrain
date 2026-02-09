package embedder

import (
	"math"
	"math/rand"
)

// Embedder generates vector embeddings from text.
type Embedder interface {
	Embed(texts []string) ([][]float32, error)
	Dimension() int
}

// MockEmbedder generates deterministic random embeddings for testing/development.
// In production, this would be replaced with an actual embedding service call
// (e.g., OpenAI text-embedding-3-large, or a local model via HTTP).
type MockEmbedder struct {
	dim  int
	seed int64
}

// NewMockEmbedder creates a new MockEmbedder.
func NewMockEmbedder(dimension int) *MockEmbedder {
	return &MockEmbedder{dim: dimension, seed: 42}
}

// Embed generates mock embeddings based on text hashing for reproducibility.
func (e *MockEmbedder) Embed(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		results[i] = e.embedSingle(text)
	}
	return results, nil
}

// Dimension returns the embedding vector dimension.
func (e *MockEmbedder) Dimension() int {
	return e.dim
}

func (e *MockEmbedder) embedSingle(text string) []float32 {
	// Use text hash as seed for deterministic embeddings
	seed := e.seed
	for _, c := range text {
		seed = seed*31 + int64(c)
	}

	rng := rand.New(rand.NewSource(seed))
	vec := make([]float32, e.dim)
	var norm float64
	for j := 0; j < e.dim; j++ {
		vec[j] = float32(rng.NormFloat64())
		norm += float64(vec[j]) * float64(vec[j])
	}

	// L2-normalize
	norm = math.Sqrt(norm)
	if norm > 0 {
		for j := range vec {
			vec[j] = float32(float64(vec[j]) / norm)
		}
	}

	return vec
}
