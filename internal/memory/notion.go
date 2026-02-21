package memory

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jomei/notionapi"
)

const defaultUserID = "default"

// NotionUserFactStore persists user facts to a Notion "User Profile" page.
type NotionUserFactStore struct {
	client     *notionapi.Client
	profileID  string
	defaultUID string
}

// NewNotionUserFactStore creates a Notion-backed user fact store.
// NOTION_TOKEN and NOTION_USER_PROFILE_PAGE_ID (page ID for the profile block) are read from env.
func NewNotionUserFactStore() (*NotionUserFactStore, error) {
	token := os.Getenv("NOTION_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("NOTION_TOKEN is required")
	}
	profileID := os.Getenv("NOTION_USER_PROFILE_PAGE_ID")
	if profileID == "" {
		return nil, fmt.Errorf("NOTION_USER_PROFILE_PAGE_ID is required")
	}
	return &NotionUserFactStore{
		client:     notionapi.NewClient(notionapi.Token(token)),
		profileID:  profileID,
		defaultUID: defaultUserID,
	}, nil
}

// GetProfile returns the full text of the user profile page (for system prompt).
func (n *NotionUserFactStore) GetProfile(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		userID = n.defaultUID
	}
	// Use the single profile page ID; in a multi-user setup you'd resolve userID -> page ID
	blockID := notionapi.BlockID(n.profileID)
	children, err := n.client.Block.GetChildren(ctx, blockID, &notionapi.Pagination{PageSize: 100})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range children.Results {
		text := blockToPlainText(block)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// AppendFact appends a fact to the user's profile page (as a new paragraph block).
func (n *NotionUserFactStore) AppendFact(ctx context.Context, userID string, fact string) error {
	if userID == "" {
		userID = n.defaultUID
	}
	blockID := notionapi.BlockID(n.profileID)
	_, err := n.client.Block.AppendChildren(ctx, blockID, &notionapi.AppendBlockChildrenRequest{
		Children: []notionapi.Block{
			&notionapi.ParagraphBlock{
				BasicBlock: notionapi.BasicBlock{Object: "block", Type: notionapi.BlockTypeParagraph},
				Paragraph: notionapi.Paragraph{
					RichText: []notionapi.RichText{{Type: "text", Text: &notionapi.Text{Content: fact}}},
				},
			},
		},
	})
	return err
}

func blockToPlainText(block notionapi.Block) string {
	switch b := block.(type) {
	case *notionapi.ParagraphBlock:
		return richTextToPlain(b.Paragraph.RichText)
	case *notionapi.Heading1Block:
		return richTextToPlain(b.Heading1.RichText)
	case *notionapi.Heading2Block:
		return richTextToPlain(b.Heading2.RichText)
	case *notionapi.Heading3Block:
		return richTextToPlain(b.Heading3.RichText)
	case *notionapi.BulletedListItemBlock:
		return richTextToPlain(b.BulletedListItem.RichText)
	case *notionapi.NumberedListItemBlock:
		return richTextToPlain(b.NumberedListItem.RichText)
	case *notionapi.ToDoBlock:
		return richTextToPlain(b.ToDo.RichText)
	case *notionapi.QuoteBlock:
		return richTextToPlain(b.Quote.RichText)
	case *notionapi.CalloutBlock:
		return richTextToPlain(b.Callout.RichText)
	default:
		return ""
	}
}

func richTextToPlain(rts []notionapi.RichText) string {
	var s strings.Builder
	for _, rt := range rts {
		if rt.PlainText != "" {
			s.WriteString(rt.PlainText)
		}
	}
	return s.String()
}

// NotionPageFetcherImpl fetches page text via the Notion API (implements NotionPageFetcher).
type NotionPageFetcherImpl struct {
	client *notionapi.Client
}

// NewNotionPageFetcher creates a fetcher using NOTION_TOKEN.
func NewNotionPageFetcher() (*NotionPageFetcherImpl, error) {
	token := os.Getenv("NOTION_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("NOTION_TOKEN is required")
	}
	return &NotionPageFetcherImpl{client: notionapi.NewClient(notionapi.Token(token))}, nil
}

// GetPageText returns the plain text of a Notion page (block children).
func (n *NotionPageFetcherImpl) GetPageText(ctx context.Context, pageID string) (string, error) {
	blockID := notionapi.BlockID(pageID)
	children, err := n.client.Block.GetChildren(ctx, blockID, &notionapi.Pagination{PageSize: 100})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range children.Results {
		text := blockToPlainText(block)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}
