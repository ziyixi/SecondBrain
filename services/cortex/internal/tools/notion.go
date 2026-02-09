package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/ziyixi/SecondBrain/services/cortex/internal/mcp"
)

// NotionTools provides high-level Notion operations exposed as tools
// to the AI agent via MCP.
type NotionTools struct {
	mcpClient *mcp.Client
}

// NewNotionTools creates a new NotionTools instance.
func NewNotionTools(mcpClient *mcp.Client) *NotionTools {
	return &NotionTools{mcpClient: mcpClient}
}

// SmartAppendJournal appends text to the current day's journal page
// under the "Daily Log" section with a timestamp.
func (t *NotionTools) SmartAppendJournal(ctx context.Context, text string) error {
	timestamp := time.Now().Format("15:04")
	entry := fmt.Sprintf("• [%s] %s", timestamp, text)

	_, err := t.mcpClient.CallTool(ctx, "notion_append_block_children", map[string]interface{}{
		"blockId": "journal-daily-log",
		"children": fmt.Sprintf(`[{"object":"block","type":"bulleted_list_item","bulleted_list_item":{"rich_text":[{"type":"text","text":{"content":"%s"}}]}}]`, entry),
	})
	if err != nil {
		return fmt.Errorf("appending to journal: %w", err)
	}

	return nil
}

// QueryDatabaseSchema inspects the structure of a Notion database
// to prevent schema hallucinations.
func (t *NotionTools) QueryDatabaseSchema(ctx context.Context, databaseID string) (map[string]interface{}, error) {
	result, err := t.mcpClient.CallTool(ctx, "notion_retrieve_database", map[string]interface{}{
		"database_id": databaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("querying database schema: %w", err)
	}

	schema := make(map[string]interface{})
	if len(result.Content) > 0 {
		schema["raw"] = result.Content[0].Text
	}
	return schema, nil
}

// RetrieveAndVectorize fetches a page and triggers re-indexing
// in the Hippocampus service.
func (t *NotionTools) RetrieveAndVectorize(ctx context.Context, pageID string) (string, error) {
	result, err := t.mcpClient.CallTool(ctx, "notion_retrieve_page", map[string]interface{}{
		"page_id": pageID,
	})
	if err != nil {
		return "", fmt.Errorf("retrieving page: %w", err)
	}

	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", nil
}

// SearchDatabase searches a Notion database with a filter.
func (t *NotionTools) SearchDatabase(ctx context.Context, databaseID string, query string) (string, error) {
	result, err := t.mcpClient.CallTool(ctx, "notion_query_database", map[string]interface{}{
		"database_id": databaseID,
		"filter":      query,
	})
	if err != nil {
		return "", fmt.Errorf("searching database: %w", err)
	}

	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", nil
}
