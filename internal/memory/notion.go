package memory

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const defaultUserID = "default"

// NotionUserFactStore persists user facts to a Notion "User Profile" page.
type NotionUserFactStore struct {
	client     *notionClient
	profileID  string
	defaultUID string
}

// NewNotionUserFactStore creates a Notion-backed user fact store.
// Auth: internal integration token (NOTION_TOKEN) per https://developers.notion.com/docs/authorization#internal-integration-auth-flow-set-up.
// NOTION_USER_PROFILE_PAGE_ID is the page ID; the page must be shared with the integration (Add connections).
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
		client:     newNotionClient(token),
		profileID:  strings.TrimSpace(profileID),
		defaultUID: defaultUserID,
	}, nil
}

// GetProfile returns the full text of the user profile page (for system prompt).
func (n *NotionUserFactStore) GetProfile(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		userID = n.defaultUID
	}
	blocks, err := n.client.GetBlockChildren(ctx, n.profileID)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range blocks {
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
	return n.client.AppendBlockChildren(ctx, n.profileID, []string{fact})
}

// NotionPageFetcherImpl fetches page text via the Notion API (implements NotionPageFetcher).
type NotionPageFetcherImpl struct {
	client *notionClient
}

// NewNotionPageFetcher creates a fetcher using NOTION_TOKEN (internal integration; see Notion authorization docs).
func NewNotionPageFetcher() (*NotionPageFetcherImpl, error) {
	token := os.Getenv("NOTION_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("NOTION_TOKEN is required")
	}
	return &NotionPageFetcherImpl{client: newNotionClient(token)}, nil
}

// GetPageText returns the plain text of a Notion page (block children).
func (n *NotionPageFetcherImpl) GetPageText(ctx context.Context, pageID string) (string, error) {
	blocks, err := n.client.GetBlockChildren(ctx, pageID)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range blocks {
		text := blockToPlainText(block)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}
