package embedder

import (
	"math"
	"testing"
)

func TestMockEmbedderDimension(t *testing.T) {
	e := NewMockEmbedder(384)
	if e.Dimension() != 384 {
		t.Errorf("expected dimension 384, got %d", e.Dimension())
	}
}

func TestMockEmbedderEmbed(t *testing.T) {
	e := NewMockEmbedder(128)
	texts := []string{"hello world", "second text"}

	embeddings, err := e.Embed(texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}

	for i, emb := range embeddings {
		if len(emb) != 128 {
			t.Errorf("embedding %d: expected 128 dims, got %d", i, len(emb))
		}
	}
}

func TestMockEmbedderNormalized(t *testing.T) {
	e := NewMockEmbedder(64)
	embeddings, _ := e.Embed([]string{"test"})

	vec := embeddings[0]
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)

	// Should be approximately 1.0 (L2-normalized)
	if math.Abs(norm-1.0) > 0.01 {
		t.Errorf("expected L2 norm ~1.0, got %f", norm)
	}
}

func TestMockEmbedderDeterministic(t *testing.T) {
	e := NewMockEmbedder(32)

	emb1, _ := e.Embed([]string{"same text"})
	emb2, _ := e.Embed([]string{"same text"})

	for i := range emb1[0] {
		if emb1[0][i] != emb2[0][i] {
			t.Errorf("embeddings differ at index %d: %f vs %f", i, emb1[0][i], emb2[0][i])
			break
		}
	}
}

func TestMockEmbedderDifferentTexts(t *testing.T) {
	e := NewMockEmbedder(32)

	emb1, _ := e.Embed([]string{"text A"})
	emb2, _ := e.Embed([]string{"text B"})

	same := true
	for i := range emb1[0] {
		if emb1[0][i] != emb2[0][i] {
			same = false
			break
		}
	}

	if same {
		t.Error("different texts should produce different embeddings")
	}
}
