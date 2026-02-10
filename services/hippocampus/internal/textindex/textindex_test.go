package textindex

import (
	"testing"
)

func TestNewIndex(t *testing.T) {
	idx := New()
	if idx == nil {
		t.Fatal("expected non-nil index")
	}
	if idx.Count("test") != 0 {
		t.Errorf("expected 0, got %d", idx.Count("test"))
	}
}

func TestAddAndSearch(t *testing.T) {
	idx := New()

	idx.Add("test", Document{
		ID:      "1",
		Content: "The PhaseNet-TF model extends the original PhaseNet architecture for seismic signal detection.",
		Metadata: map[string]string{"type": "research"},
	})
	idx.Add("test", Document{
		ID:      "2",
		Content: "Kubernetes deployment patterns for microservices and container orchestration.",
		Metadata: map[string]string{"type": "devops"},
	})
	idx.Add("test", Document{
		ID:      "3",
		Content: "Deep learning techniques for earthquake detection and seismic wave analysis.",
		Metadata: map[string]string{"type": "research"},
	})

	hits := idx.Search("test", "seismic detection", 3, nil)
	if len(hits) == 0 {
		t.Fatal("expected search results")
	}

	// Documents about seismic detection should rank highest
	if hits[0].ID != "1" && hits[0].ID != "3" {
		t.Errorf("expected seismic-related doc first, got %q", hits[0].ID)
	}
}

func TestSearchWithFilters(t *testing.T) {
	idx := New()

	idx.Add("test", Document{
		ID:      "1",
		Content: "Machine learning for signal detection",
		Metadata: map[string]string{"type": "research"},
	})
	idx.Add("test", Document{
		ID:      "2",
		Content: "Signal processing and detection algorithms",
		Metadata: map[string]string{"type": "devops"},
	})

	hits := idx.Search("test", "signal detection", 10, map[string]string{"type": "research"})
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit with filter, got %d", len(hits))
	}
	if hits[0].ID != "1" {
		t.Errorf("expected doc 1, got %q", hits[0].ID)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	idx := New()
	idx.Add("test", Document{ID: "1", Content: "some content"})

	hits := idx.Search("test", "", 5, nil)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits for empty query, got %d", len(hits))
	}
}

func TestSearchEmptyCollection(t *testing.T) {
	idx := New()
	hits := idx.Search("nonexistent", "query", 5, nil)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(hits))
	}
}

func TestSearchScoreNormalization(t *testing.T) {
	idx := New()

	idx.Add("test", Document{ID: "1", Content: "alpha beta gamma"})
	idx.Add("test", Document{ID: "2", Content: "alpha alpha alpha"})

	hits := idx.Search("test", "alpha", 2, nil)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	// Top result should have score of 1.0 (normalized)
	if hits[0].Score < 0.99 || hits[0].Score > 1.01 {
		t.Errorf("expected top score ~1.0, got %f", hits[0].Score)
	}

	// Second result should have lower score
	if hits[1].Score >= hits[0].Score {
		t.Errorf("expected second score < first score")
	}
}

func TestDelete(t *testing.T) {
	idx := New()

	idx.Add("test", Document{ID: "1", Content: "hello world"})
	idx.Add("test", Document{ID: "2", Content: "hello there"})

	if idx.Count("test") != 2 {
		t.Fatalf("expected 2, got %d", idx.Count("test"))
	}

	idx.Delete("test", "1")

	if idx.Count("test") != 1 {
		t.Errorf("expected 1 after delete, got %d", idx.Count("test"))
	}

	hits := idx.Search("test", "hello", 10, nil)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit after delete, got %d", len(hits))
	}
	if hits[0].ID != "2" {
		t.Errorf("expected doc 2, got %q", hits[0].ID)
	}
}

func TestCollectionIsolation(t *testing.T) {
	idx := New()

	idx.Add("col1", Document{ID: "1", Content: "alpha beta"})
	idx.Add("col2", Document{ID: "2", Content: "alpha gamma"})

	hits := idx.Search("col1", "alpha", 10, nil)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit in col1, got %d", len(hits))
	}
	if hits[0].ID != "1" {
		t.Errorf("expected doc 1, got %q", hits[0].ID)
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello, World! This is a TEST 123.")
	expected := []string{"hello", "world", "this", "is", "a", "test", "123"}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
	for i, tok := range tokens {
		if tok != expected[i] {
			t.Errorf("token %d: expected %q, got %q", i, expected[i], tok)
		}
	}
}
