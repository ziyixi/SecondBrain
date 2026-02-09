package vectorstore

import (
	"testing"
)

func TestInMemoryStoreUpsertAndCount(t *testing.T) {
	store := NewInMemoryStore()

	err := store.Upsert("test", []Record{
		{ID: "1", Vector: []float32{1, 0, 0}, Payload: map[string]string{"doc": "a"}},
		{ID: "2", Vector: []float32{0, 1, 0}, Payload: map[string]string{"doc": "b"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.Count("test") != 2 {
		t.Errorf("expected 2, got %d", store.Count("test"))
	}
}

func TestInMemoryStoreSearch(t *testing.T) {
	store := NewInMemoryStore()

	store.Upsert("test", []Record{
		{ID: "1", Vector: []float32{1, 0, 0}, Payload: map[string]string{"content": "hello"}},
		{ID: "2", Vector: []float32{0, 1, 0}, Payload: map[string]string{"content": "world"}},
		{ID: "3", Vector: []float32{0.9, 0.1, 0}, Payload: map[string]string{"content": "similar"}},
	})

	// Search for vector similar to [1, 0, 0]
	hits, err := store.Search("test", []float32{1, 0, 0}, 2, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}

	// First result should be most similar
	if hits[0].ID != "1" {
		t.Errorf("expected first hit to be '1', got %q", hits[0].ID)
	}
}

func TestInMemoryStoreSearchWithFilters(t *testing.T) {
	store := NewInMemoryStore()

	store.Upsert("test", []Record{
		{ID: "1", Vector: []float32{1, 0, 0}, Payload: map[string]string{"type": "email"}},
		{ID: "2", Vector: []float32{0.9, 0.1, 0}, Payload: map[string]string{"type": "slack"}},
		{ID: "3", Vector: []float32{0.8, 0.2, 0}, Payload: map[string]string{"type": "email"}},
	})

	hits, err := store.Search("test", []float32{1, 0, 0}, 10, map[string]string{"type": "email"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hits) != 2 {
		t.Errorf("expected 2 hits with type=email, got %d", len(hits))
	}

	for _, h := range hits {
		if h.Payload["type"] != "email" {
			t.Errorf("expected type=email, got %q", h.Payload["type"])
		}
	}
}

func TestInMemoryStoreSearchEmptyCollection(t *testing.T) {
	store := NewInMemoryStore()

	hits, err := store.Search("nonexistent", []float32{1, 0, 0}, 5, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(hits))
	}
}

func TestInMemoryStoreDelete(t *testing.T) {
	store := NewInMemoryStore()

	store.Upsert("test", []Record{
		{ID: "1", Vector: []float32{1, 0, 0}},
		{ID: "2", Vector: []float32{0, 1, 0}},
	})

	deleted, err := store.Delete("test", []string{"1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	if store.Count("test") != 1 {
		t.Errorf("expected 1 remaining, got %d", store.Count("test"))
	}
}

func TestInMemoryStoreDeleteNonExistent(t *testing.T) {
	store := NewInMemoryStore()

	deleted, err := store.Delete("nonexistent", []string{"1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestInMemoryStoreUpsertOverwrite(t *testing.T) {
	store := NewInMemoryStore()

	store.Upsert("test", []Record{
		{ID: "1", Vector: []float32{1, 0, 0}, Payload: map[string]string{"v": "old"}},
	})
	store.Upsert("test", []Record{
		{ID: "1", Vector: []float32{0, 1, 0}, Payload: map[string]string{"v": "new"}},
	})

	if store.Count("test") != 1 {
		t.Errorf("expected 1, got %d", store.Count("test"))
	}

	hits, _ := store.Search("test", []float32{0, 1, 0}, 1, nil)
	if hits[0].Payload["v"] != "new" {
		t.Errorf("expected 'new', got %q", hits[0].Payload["v"])
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float32
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{"opposite", []float32{1, 0, 0}, []float32{-1, 0, 0}, -1.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if abs(got-tc.expected) > 0.01 {
				t.Errorf("expected %f, got %f", tc.expected, got)
			}
		})
	}
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
