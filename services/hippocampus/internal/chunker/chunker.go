package chunker

import (
	"strings"

	"github.com/google/uuid"
)

// Chunk represents a piece of text with metadata.
type Chunk struct {
	ID         string
	DocumentID string
	Content    string
	Index      int
	Metadata   map[string]string
}

// Strategy defines chunking behavior.
type Strategy interface {
	Chunk(documentID, text string, metadata map[string]string) []Chunk
}

// FixedSizeChunker splits text into fixed-size word chunks with overlap.
type FixedSizeChunker struct {
	ChunkSize int
	Overlap   int
}

// Chunk splits text into fixed-size chunks.
func (c *FixedSizeChunker) Chunk(documentID, text string, metadata map[string]string) []Chunk {
	if text == "" {
		return nil
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []Chunk
	start := 0
	index := 0

	for start < len(words) {
		end := start + c.ChunkSize
		if end > len(words) {
			end = len(words)
		}

		chunkText := strings.Join(words[start:end], " ")
		meta := copyMetadata(metadata)

		chunks = append(chunks, Chunk{
			ID:         uuid.New().String(),
			DocumentID: documentID,
			Content:    chunkText,
			Index:      index,
			Metadata:   meta,
		})

		start = end - c.Overlap
		if start <= start-c.ChunkSize+c.Overlap && end == len(words) {
			break
		}
		index++
	}

	return chunks
}

// SemanticChunker splits text at sentence boundaries.
type SemanticChunker struct {
	MaxChunkSize int
}

// Chunk splits text into semantic chunks.
func (c *SemanticChunker) Chunk(documentID, text string, metadata map[string]string) []Chunk {
	if text == "" {
		return nil
	}

	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil
	}

	var chunks []Chunk
	var current []string
	currentSize := 0
	index := 0

	for _, sentence := range sentences {
		sentWords := len(strings.Fields(sentence))

		if currentSize+sentWords > c.MaxChunkSize && len(current) > 0 {
			chunkText := strings.Join(current, " ")
			meta := copyMetadata(metadata)
			chunks = append(chunks, Chunk{
				ID:         uuid.New().String(),
				DocumentID: documentID,
				Content:    chunkText,
				Index:      index,
				Metadata:   meta,
			})
			current = nil
			currentSize = 0
			index++
		}

		current = append(current, sentence)
		currentSize += sentWords
	}

	if len(current) > 0 {
		chunkText := strings.Join(current, " ")
		meta := copyMetadata(metadata)
		chunks = append(chunks, Chunk{
			ID:         uuid.New().String(),
			DocumentID: documentID,
			Content:    chunkText,
			Index:      index,
			Metadata:   meta,
		})
	}

	return chunks
}

// HierarchicalChunker splits text at section header boundaries.
type HierarchicalChunker struct {
	MaxChunkSize int
}

// Chunk splits text into hierarchical chunks.
func (c *HierarchicalChunker) Chunk(documentID, text string, metadata map[string]string) []Chunk {
	if text == "" {
		return nil
	}

	sections := splitSections(text)
	var chunks []Chunk
	index := 0

	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}

		words := strings.Fields(section)
		if len(words) <= c.MaxChunkSize {
			meta := copyMetadata(metadata)
			chunks = append(chunks, Chunk{
				ID:         uuid.New().String(),
				DocumentID: documentID,
				Content:    section,
				Index:      index,
				Metadata:   meta,
			})
			index++
		} else {
			sub := &SemanticChunker{MaxChunkSize: c.MaxChunkSize}
			subChunks := sub.Chunk(documentID, section, metadata)
			for _, sc := range subChunks {
				sc.Index = index
				chunks = append(chunks, sc)
				index++
			}
		}
	}

	return chunks
}

// NewStrategy creates a Strategy from a name.
func NewStrategy(name string, chunkSize, overlap int) Strategy {
	switch name {
	case "semantic":
		return &SemanticChunker{MaxChunkSize: chunkSize}
	case "hierarchical":
		return &HierarchicalChunker{MaxChunkSize: chunkSize}
	default:
		return &FixedSizeChunker{ChunkSize: chunkSize, Overlap: overlap}
	}
}

func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	for _, r := range text {
		current.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			s := strings.TrimSpace(current.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}

	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}

	return sentences
}

func splitSections(text string) []string {
	lines := strings.Split(text, "\n")
	var sections []string
	var current []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || (len(trimmed) > 0 && trimmed == strings.ToUpper(trimmed) && len(trimmed) > 3) {
			if len(current) > 0 {
				sections = append(sections, strings.Join(current, "\n"))
				current = nil
			}
		}
		current = append(current, line)
	}

	if len(current) > 0 {
		sections = append(sections, strings.Join(current, "\n"))
	}

	return sections
}

func copyMetadata(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
