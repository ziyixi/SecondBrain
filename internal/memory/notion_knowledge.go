package memory

import (
	"context"
	"fmt"

	"github.com/yourusername/secondbrain/internal/config"
)

// NotionKnowledgeStore creates and appends to pages in a Notion database (knowledge graph).
type NotionKnowledgeStore struct {
	client     *notionClient
	databaseID string
	titleProp  string
}

// NewNotionKnowledgeStore creates a store from the central config.
func NewNotionKnowledgeStore(cfg *config.Config) (*NotionKnowledgeStore, error) {
	if cfg.NotionToken == "" {
		return nil, fmt.Errorf("NOTION_TOKEN is required")
	}
	if cfg.NotionKnowledgeDatabaseID == "" {
		return nil, fmt.Errorf("NOTION_KNOWLEDGE_DATABASE_ID is required")
	}
	return &NotionKnowledgeStore{
		client:     newNotionClient(cfg.NotionToken),
		databaseID: cfg.NotionKnowledgeDatabaseID,
		titleProp:  cfg.NotionKnowledgeTitleProperty,
	}, nil
}

// CreatePage creates a new page in the knowledge database with the given title and content (one paragraph block).
// Returns the new page's ID.
func (n *NotionKnowledgeStore) CreatePage(ctx context.Context, title string, content string) (string, error) {
	return n.client.CreatePage(ctx, n.databaseID, n.titleProp, title, content)
}

// AppendToPage appends a paragraph block with content to an existing page.
func (n *NotionKnowledgeStore) AppendToPage(ctx context.Context, pageID string, content string) error {
	return n.client.AppendBlockChildren(ctx, pageID, []string{content})
}
