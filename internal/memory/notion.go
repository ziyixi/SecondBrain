package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourusername/secondbrain/internal/config"
)

const defaultUserID = "default"

// NotionUserFactStore persists user facts to a Notion "User Profile" page.
type NotionUserFactStore struct {
	client     *notionClient
	profileID  string
	defaultUID string
}

// NewNotionUserFactStore creates a Notion-backed user fact store from the central config.
func NewNotionUserFactStore(cfg *config.Config) (*NotionUserFactStore, error) {
	if cfg.NotionToken == "" {
		return nil, fmt.Errorf("NOTION_TOKEN is required")
	}
	if cfg.NotionUserProfilePageID == "" {
		return nil, fmt.Errorf("NOTION_USER_PROFILE_PAGE_ID is required")
	}
	return &NotionUserFactStore{
		client:     newNotionClient(cfg.NotionToken),
		profileID:  cfg.NotionUserProfilePageID,
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

// NewNotionPageFetcher creates a fetcher from the central config.
func NewNotionPageFetcher(cfg *config.Config) (*NotionPageFetcherImpl, error) {
	if cfg.NotionToken == "" {
		return nil, fmt.Errorf("NOTION_TOKEN is required")
	}
	return &NotionPageFetcherImpl{client: newNotionClient(cfg.NotionToken)}, nil
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
