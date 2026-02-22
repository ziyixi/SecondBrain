// Notion API client via HTTP (no third-party SDK).
//
// Implementation follows the official Notion API docs:
//   - Authorization: https://developers.notion.com/docs/authorization (Bearer token, internal integration)
//   - API versioning: https://developers.notion.com/reference/versioning (Notion-Version header)
//   - Retrieve block children: https://developers.notion.com/reference/get-block-children (GET /v1/blocks/{block_id}/children)
//   - Append block children: https://developers.notion.com/reference/patch-block-children (PATCH /v1/blocks/{block_id}/children)
//   - Create a page: https://developers.notion.com/reference/post-page (POST /v1/pages)
//   - Block object (response shape): https://developers.notion.com/reference/block
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	notionAPIBase    = "https://api.notion.com" // Notion API base URL, see https://developers.notion.com/reference
	notionAPIVersion = "2025-09-03"             // From https://developers.notion.com/reference/versioning
)

// notionClient calls the Notion REST API with Bearer token and Notion-Version header.
type notionClient struct {
	token      string
	httpClient *http.Client
}

func newNotionClient(token string) *notionClient {
	return &notionClient{
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{},
	}
}

func (c *notionClient) do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, notionAPIBase+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", notionAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

// blockChildrenResponse is the response from GET /v1/blocks/{id}/children.
// See https://developers.notion.com/reference/get-block-children
type blockChildrenResponse struct {
	Results []notionBlock `json:"results"`
	HasMore bool          `json:"has_more"`
}

// notionBlock is a block in the API response; we only need type and rich_text content.
// Block object reference: https://developers.notion.com/reference/block
type notionBlock struct {
	Type             string          `json:"type"`
	Paragraph        *richTextHolder `json:"paragraph,omitempty"`
	Heading1         *richTextHolder `json:"heading_1,omitempty"`
	Heading2         *richTextHolder `json:"heading_2,omitempty"`
	Heading3         *richTextHolder `json:"heading_3,omitempty"`
	BulletedListItem *richTextHolder `json:"bulleted_list_item,omitempty"`
	NumberedListItem *richTextHolder `json:"numbered_list_item,omitempty"`
	ToDo             *richTextHolder `json:"to_do,omitempty"`
	Quote            *richTextHolder `json:"quote,omitempty"`
	Callout          *richTextHolder `json:"callout,omitempty"`
}

type richTextHolder struct {
	RichText []notionRichText `json:"rich_text"`
}

type notionRichText struct {
	PlainText string `json:"plain_text"`
	Type      string `json:"type"`
	Text      *struct {
		Content string `json:"content"`
	} `json:"text,omitempty"`
}

// paragraphBlockRequest is the request body for appending a paragraph block.
// See https://developers.notion.com/reference/patch-block-children (children array, block object request).
type paragraphBlockRequest struct {
	Object    string `json:"object"`
	Type      string `json:"type"`
	Paragraph struct {
		RichText []notionRichTextRequest `json:"rich_text"`
	} `json:"paragraph"`
}

type notionRichTextRequest struct {
	Type string `json:"type"`
	Text struct {
		Content string `json:"content"`
	} `json:"text"`
}

func richTextRequest(content string) notionRichTextRequest {
	return notionRichTextRequest{
		Type: "text",
		Text: struct {
			Content string `json:"content"`
		}{Content: content},
	}
}

// appendBlockChildrenRequest is the body for PATCH /v1/blocks/{id}/children.
type appendBlockChildrenRequest struct {
	Children []paragraphBlockRequest `json:"children"`
}

// createPageRequest is the body for POST /v1/pages (database parent).
// See https://developers.notion.com/reference/post-page
type createPageRequest struct {
	Parent     notionParent                   `json:"parent"`
	Properties map[string]notionTitleProperty `json:"properties"`
	Children   []paragraphBlockRequest        `json:"children,omitempty"`
}

type notionParent struct {
	Type       string `json:"type"`
	DatabaseID string `json:"database_id"`
}

type notionTitleProperty struct {
	Title []notionRichTextRequest `json:"title"`
}

// formatBlockID trims whitespace; Notion accepts IDs with or without hyphens.
func formatBlockID(id string) string {
	return strings.TrimSpace(id)
}

// GetBlockChildren fetches the first page of child blocks (page_size=100).
// See https://developers.notion.com/reference/get-block-children
func (c *notionClient) GetBlockChildren(ctx context.Context, blockID string) ([]notionBlock, error) {
	id := formatBlockID(blockID)
	path := fmt.Sprintf("/v1/blocks/%s/children?page_size=100", id)
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("notion API %s %s: %d %s", http.MethodGet, path, resp.StatusCode, string(body))
	}
	var out blockChildrenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// AppendBlockChildren appends paragraph blocks to the given block (page or block).
// See https://developers.notion.com/reference/patch-block-children
func (c *notionClient) AppendBlockChildren(ctx context.Context, blockID string, paragraphContents []string) error {
	id := formatBlockID(blockID)
	path := fmt.Sprintf("/v1/blocks/%s/children", id)
	children := make([]paragraphBlockRequest, 0, len(paragraphContents))
	for _, content := range paragraphContents {
		children = append(children, paragraphBlockRequest{
			Object: "block",
			Type:   "paragraph",
			Paragraph: struct {
				RichText []notionRichTextRequest `json:"rich_text"`
			}{
				RichText: []notionRichTextRequest{richTextRequest(content)},
			},
		})
	}
	body := appendBlockChildrenRequest{Children: children}
	resp, err := c.do(ctx, http.MethodPatch, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notion API PATCH %s: %d %s", path, resp.StatusCode, string(respBody))
	}
	return nil
}

// CreatePage creates a page in a database with the given title and optional body paragraph. Returns the new page ID.
// See https://developers.notion.com/reference/post-page
func (c *notionClient) CreatePage(ctx context.Context, databaseID, titleProperty, title, bodyContent string) (string, error) {
	path := "/v1/pages"
	dbID := formatBlockID(databaseID)
	req := createPageRequest{
		Parent: notionParent{Type: "database_id", DatabaseID: dbID},
		Properties: map[string]notionTitleProperty{
			titleProperty: {Title: []notionRichTextRequest{richTextRequest(title)}},
		},
	}
	if bodyContent != "" {
		req.Children = []paragraphBlockRequest{{
			Object: "block",
			Type:   "paragraph",
			Paragraph: struct {
				RichText []notionRichTextRequest `json:"rich_text"`
			}{
				RichText: []notionRichTextRequest{richTextRequest(bodyContent)},
			},
		}}
	}
	resp, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("notion API POST %s: %d %s", path, resp.StatusCode, string(respBody))
	}
	var page struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return "", err
	}
	return page.ID, nil
}

// blockToPlainText extracts plain text from a Notion block for known types.
func blockToPlainText(b notionBlock) string {
	var rts []notionRichText
	switch b.Type {
	case "paragraph":
		if b.Paragraph != nil {
			rts = b.Paragraph.RichText
		}
	case "heading_1":
		if b.Heading1 != nil {
			rts = b.Heading1.RichText
		}
	case "heading_2":
		if b.Heading2 != nil {
			rts = b.Heading2.RichText
		}
	case "heading_3":
		if b.Heading3 != nil {
			rts = b.Heading3.RichText
		}
	case "bulleted_list_item":
		if b.BulletedListItem != nil {
			rts = b.BulletedListItem.RichText
		}
	case "numbered_list_item":
		if b.NumberedListItem != nil {
			rts = b.NumberedListItem.RichText
		}
	case "to_do":
		if b.ToDo != nil {
			rts = b.ToDo.RichText
		}
	case "quote":
		if b.Quote != nil {
			rts = b.Quote.RichText
		}
	case "callout":
		if b.Callout != nil {
			rts = b.Callout.RichText
		}
	}
	return richTextToPlain(rts)
}

func richTextToPlain(rts []notionRichText) string {
	var s strings.Builder
	for _, rt := range rts {
		if rt.PlainText != "" {
			s.WriteString(rt.PlainText)
		} else if rt.Text != nil {
			s.WriteString(rt.Text.Content)
		}
	}
	return s.String()
}
