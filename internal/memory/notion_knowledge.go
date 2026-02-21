package memory

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jomei/notionapi"
)

const (
	defaultKnowledgeTitleProperty = "Name"
)

// NotionKnowledgeStore creates and appends to pages in a Notion database (knowledge graph).
// NOTION_TOKEN, NOTION_KNOWLEDGE_DATABASE_ID required. NOTION_KNOWLEDGE_TITLE_PROPERTY (default "Name") optional.
type NotionKnowledgeStore struct {
	client     *notionapi.Client
	databaseID notionapi.DatabaseID
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
		client:     notionapi.NewClient(notionapi.Token(token)),
		databaseID: notionapi.DatabaseID(strings.TrimSpace(dbID)),
		titleProp:  titleProp,
	}, nil
}

// CreatePage creates a new page in the knowledge database with the given title and content (one paragraph block).
// Returns the new page's ID.
func (n *NotionKnowledgeStore) CreatePage(ctx context.Context, title string, content string) (string, error) {
	req := &notionapi.PageCreateRequest{
		Parent: notionapi.Parent{
			Type:       notionapi.ParentTypeDatabaseID,
			DatabaseID: n.databaseID,
		},
		Properties: notionapi.Properties{
			n.titleProp: notionapi.TitleProperty{
				Title: []notionapi.RichText{
					{Type: "text", Text: &notionapi.Text{Content: title}}},
			},
		},
	}
	if content != "" {
		req.Children = []notionapi.Block{
			&notionapi.ParagraphBlock{
				BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeParagraph},
				Paragraph: notionapi.Paragraph{
					RichText: []notionapi.RichText{{Type: "text", Text: &notionapi.Text{Content: content}}},
				},
			},
		}
	}
	page, err := n.client.Page.Create(ctx, req)
	if err != nil {
		return "", err
	}
	return string(page.ID), nil
}

// AppendToPage appends a paragraph block with content to an existing page.
func (n *NotionKnowledgeStore) AppendToPage(ctx context.Context, pageID string, content string) error {
	blockID := notionapi.BlockID(pageID)
	_, err := n.client.Block.AppendChildren(ctx, blockID, &notionapi.AppendBlockChildrenRequest{
		Children: []notionapi.Block{
			&notionapi.ParagraphBlock{
				BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeParagraph},
				Paragraph: notionapi.Paragraph{
					RichText: []notionapi.RichText{{Type: "text", Text: &notionapi.Text{Content: content}}},
				},
			},
		},
	})
	return err
}
