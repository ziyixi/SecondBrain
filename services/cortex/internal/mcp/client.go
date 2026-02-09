package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Client implements the MCP (Model Context Protocol) client
// for communicating with the Notion MCP server.
type Client struct {
	serverURL  string
	token      string
	httpClient *http.Client
	mu         sync.RWMutex
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolCallResult is the result of executing an MCP tool.
type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// ContentBlock represents a piece of content in an MCP response.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// NewClient creates a new MCP client.
func NewClient(serverURL, token string) *Client {
	return &Client{
		serverURL: strings.TrimRight(serverURL, "/"),
		token:     token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListTools retrieves available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	req, err := c.newRequest(ctx, "POST", "/", map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var result struct {
		Result struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := c.doJSON(req, &result); err != nil {
		return nil, fmt.Errorf("listing tools: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", result.Error.Message)
	}

	return result.Result.Tools, nil
}

// CallTool executes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*ToolCallResult, error) {
	req, err := c.newRequest(ctx, "POST", "/", map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var result struct {
		Result *ToolCallResult `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := c.doJSON(req, &result); err != nil {
		return nil, fmt.Errorf("calling tool %s: %w", toolName, err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("MCP tool error: %s", result.Error.Message)
	}

	return result.Result, nil
}

// SetToken updates the authentication token.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

func (c *Client) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling body: %w", err)
	}

	url := c.serverURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.mu.RLock()
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	c.mu.RUnlock()

	return req, nil
}

func (c *Client) doJSON(req *http.Request, result interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("unmarshaling response: %w", err)
	}

	return nil
}
