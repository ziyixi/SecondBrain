package memory

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	defaultKnowledgeTitleProperty = "Name"
)

// NotionKnowledgeStore creates and appends to pages in a Notion database (knowledge graph).
// Auth: internal integration token (NOTION_TOKEN); see https://developers.notion.com/docs/authorization.
// NOTION_KNOWLEDGE_DATABASE_ID required; database must be shared with the integration. NOTION_KNOWLEDGE_TITLE_PROPERTY (default "Name") optional.
type NotionKnowledgeStore struct {
	client     *notionClient
	databaseID string
	titleProp  string
}

// NewNotionKnowledgeStore creates a store that writes to a Notion database.
func NewNotionKnowledgeStore() (*NotionKnowledgeStore, error) {
	token := os.Getenv("NOTION_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("NOTION_TOKEN is required")
	}
	dbID := os.Getenv("NOTION_KNOWLEDGE_DATABASE_ID")
	if dbID == "" {
		return nil, fmt.Errorf("NOTION_KNOWLEDGE_DATABASE_ID is required")
	}
	titleProp := os.Getenv("NOTION_KNOWLEDGE_TITLE_PROPERTY")
	if titleProp == "" {
		titleProp = defaultKnowledgeTitleProperty
	}
	return &NotionKnowledgeStore{
		client:     newNotionClient(token),
		databaseID: strings.TrimSpace(dbID),
		titleProp:  titleProp,
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
