package chunker

import (
	"strings"
	"testing"
)

func TestFixedSizeChunker(t *testing.T) {
	c := &FixedSizeChunker{ChunkSize: 5, Overlap: 2}
	text := "one two three four five six seven eight nine ten"

	chunks := c.Chunk("doc-1", text, map[string]string{"source": "test"})

	if len(chunks) == 0 {
		t.Fatal("expected chunks to be generated")
	}

	for _, ch := range chunks {
		if ch.DocumentID != "doc-1" {
			t.Errorf("expected doc-1, got %q", ch.DocumentID)
		}
		if ch.ID == "" {
			t.Error("expected non-empty chunk ID")
		}
		if ch.Content == "" {
			t.Error("expected non-empty content")
		}
		if ch.Metadata["source"] != "test" {
			t.Error("expected metadata to be preserved")
		}
	}
}

func TestFixedSizeChunkerEmpty(t *testing.T) {
	c := &FixedSizeChunker{ChunkSize: 10, Overlap: 2}
	chunks := c.Chunk("doc-1", "", nil)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestSemanticChunker(t *testing.T) {
	c := &SemanticChunker{MaxChunkSize: 10}
	text := "This is sentence one. This is sentence two. And here is sentence three. Another sentence four."

	chunks := c.Chunk("doc-2", text, nil)

	if len(chunks) == 0 {
		t.Fatal("expected chunks to be generated")
	}

	for _, ch := range chunks {
		if ch.DocumentID != "doc-2" {
			t.Errorf("expected doc-2, got %q", ch.DocumentID)
		}
	}
}

func TestSemanticChunkerEmpty(t *testing.T) {
	c := &SemanticChunker{MaxChunkSize: 10}
	chunks := c.Chunk("doc-1", "", nil)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestHierarchicalChunker(t *testing.T) {
	c := &HierarchicalChunker{MaxChunkSize: 20}
	text := `# Introduction
This is the introduction section with some content.

# Methodology
Here we describe the methodology used in our research.

# Results
The results show significant improvements.`

	chunks := c.Chunk("doc-3", text, nil)

	if len(chunks) == 0 {
		t.Fatal("expected chunks to be generated")
	}

	// Verify all chunks belong to the correct document
	for _, ch := range chunks {
		if ch.DocumentID != "doc-3" {
			t.Errorf("expected doc-3, got %q", ch.DocumentID)
		}
	}
}

func TestNewStrategy(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"fixed", "*chunker.FixedSizeChunker"},
		{"semantic", "*chunker.SemanticChunker"},
		{"hierarchical", "*chunker.HierarchicalChunker"},
		{"unknown", "*chunker.FixedSizeChunker"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStrategy(tc.name, 100, 10)
			if s == nil {
				t.Fatal("expected non-nil strategy")
			}
		})
	}
}

func TestSplitSentences(t *testing.T) {
	sentences := splitSentences("Hello world. How are you? Fine!")
	if len(sentences) != 3 {
		t.Errorf("expected 3 sentences, got %d: %v", len(sentences), sentences)
	}
}

func TestCopyMetadata(t *testing.T) {
	orig := map[string]string{"a": "1", "b": "2"}
	cp := copyMetadata(orig)

	if cp["a"] != "1" || cp["b"] != "2" {
		t.Error("copy should match original")
	}

	cp["a"] = "changed"
	if orig["a"] == "changed" {
		t.Error("modifying copy should not affect original")
	}
}

func TestCopyMetadataNil(t *testing.T) {
	cp := copyMetadata(nil)
	if cp == nil {
		t.Error("expected non-nil map")
	}
}

func TestChunkContentPreservation(t *testing.T) {
	c := &FixedSizeChunker{ChunkSize: 100, Overlap: 0}
	text := "word1 word2 word3"
	chunks := c.Chunk("doc-1", text, nil)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	if !strings.Contains(chunks[0].Content, "word1") {
		t.Error("chunk should contain original words")
	}
}
