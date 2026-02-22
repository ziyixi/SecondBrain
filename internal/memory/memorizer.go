package memory

import (
	"context"
	"fmt"

	"github.com/yourusername/secondbrain/internal/config"
)

// Memorizer implements dynamic categorization: route new information to an existing
// Notion page (by topic similarity) or create a new page, then index in Qdrant.
type Memorizer interface {
	// MemorizeInformation stores content under the given topic: finds or creates a category
	// (Notion page), writes content there, and indexes vectors in Qdrant.
	MemorizeInformation(ctx context.Context, topic string, content string) (message string, err error)
}

// NotionKnowledgeWriter is implemented by NotionKnowledgeStore and fakes for tests.
type NotionKnowledgeWriter interface {
	CreatePage(ctx context.Context, title string, content string) (string, error)
	AppendToPage(ctx context.Context, pageID string, content string) error
}

// MemorizerService implements Memorizer using Notion (knowledge database) and Qdrant.
type MemorizerService struct {
	kb        *QdrantKnowledgeBase
	notion    NotionKnowledgeWriter
	threshold float32
}

// NewMemorizerService creates a memorizer that uses the given Qdrant KB and Notion knowledge writer.
// Topic similarity threshold: pass 0 to use the value from cfg (MEMORIZE_TOPIC_SIMILARITY_THRESHOLD, default 0.85).
func NewMemorizerService(kb *QdrantKnowledgeBase, notion NotionKnowledgeWriter, topicThreshold float32, cfg *config.Config) *MemorizerService {
	if topicThreshold <= 0 && cfg != nil {
		topicThreshold = cfg.TopicSimilarityThreshold
	}
	if topicThreshold <= 0 {
		topicThreshold = config.DefaultTopicSimilarityThreshold
	}
	return &MemorizerService{kb: kb, notion: notion, threshold: topicThreshold}
}

// MemorizeInformation implements Memorizer.
func (m *MemorizerService) MemorizeInformation(ctx context.Context, topic string, content string) (string, error) {
	if topic == "" || content == "" {
		return "", fmt.Errorf("topic and content are required")
	}
	topicVec, err := m.kb.Embed(ctx, topic)
	if err != nil {
		return "", fmt.Errorf("embed topic: %w", err)
	}
	existingPageID, score, found := m.kb.FindSimilarTopic(ctx, topicVec, m.threshold)
	if found {
		if err := m.notion.AppendToPage(ctx, existingPageID, content); err != nil {
			return "", fmt.Errorf("append to existing page: %w", err)
		}
		if err := m.kb.UpsertPointWithType(ctx, existingPageID, content, PointTypeContent); err != nil {
			return "", fmt.Errorf("index content in Qdrant: %w", err)
		}
		return fmt.Sprintf("Appended to existing category (similarity %.2f): %s", score, existingPageID), nil
	}
	newPageID, err := m.notion.CreatePage(ctx, topic, content)
	if err != nil {
		return "", fmt.Errorf("create Notion page: %w", err)
	}
	if err := m.kb.UpsertPointWithType(ctx, newPageID, topic, PointTypeTopic); err != nil {
		return "", fmt.Errorf("index topic in Qdrant: %w", err)
	}
	if err := m.kb.UpsertPointWithType(ctx, newPageID, content, PointTypeContent); err != nil {
		return "", fmt.Errorf("index content in Qdrant: %w", err)
	}
	return fmt.Sprintf("Created new category page: %s", newPageID), nil
}
