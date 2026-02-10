package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	memoryv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/memory/v1"
)

// Server implements an MCP (Model Context Protocol) server that exposes
// search and retrieval tools for the Second Brain knowledge base.
// Inspired by qmd's MCP server pattern for agentic workflows.
type Server struct {
	logger       *slog.Logger
	memoryClient memoryv1.MemoryServiceClient
}

// NewServer creates a new MCP server.
func NewServer(logger *slog.Logger, memoryClient memoryv1.MemoryServiceClient) *Server {
	return &Server{
		logger:       logger,
		memoryClient: memoryClient,
	}
}

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolDef defines an MCP tool.
type toolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ServeHTTP handles MCP JSON-RPC requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nil, -32700, "parse error")
		return
	}

	var resp jsonRPCResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "initialize":
		resp.Result = s.handleInitialize()
	case "tools/list":
		resp.Result = s.handleToolsList()
	case "tools/call":
		result, err := s.handleToolsCall(r.Context(), req.Params)
		if err != nil {
			resp.Error = &jsonRPCError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &jsonRPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleInitialize() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "secondbrain",
			"version": "0.1.0",
		},
	}
}

func (s *Server) handleToolsList() map[string]interface{} {
	tools := []toolDef{
		{
			Name:        "search",
			Description: "Semantic vector search using embeddings. Finds conceptually related content even without exact keyword matches.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":     map[string]interface{}{"type": "string", "description": "Natural language search query"},
					"limit":     map[string]interface{}{"type": "number", "description": "Maximum results (default: 5)"},
					"min_score": map[string]interface{}{"type": "number", "description": "Minimum relevance score 0-1"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "fts",
			Description: "Fast BM25 keyword-based full-text search. Best for finding documents with specific words or phrases.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":     map[string]interface{}{"type": "string", "description": "Keyword search query"},
					"limit":     map[string]interface{}{"type": "number", "description": "Maximum results (default: 5)"},
					"min_score": map[string]interface{}{"type": "number", "description": "Minimum relevance score 0-1"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "hybrid",
			Description: "Highest quality search combining BM25 + vector + Reciprocal Rank Fusion. Slower but most accurate.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":     map[string]interface{}{"type": "string", "description": "Natural language search query"},
					"limit":     map[string]interface{}{"type": "number", "description": "Maximum results (default: 5)"},
					"min_score": map[string]interface{}{"type": "number", "description": "Minimum relevance score 0-1"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "status",
			Description: "Show index health: document counts, chunk counts, and graph triple counts.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}
	return map[string]interface{}{"tools": tools}
}

func (s *Server) handleToolsCall(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})
	if args == nil {
		args = make(map[string]interface{})
	}

	switch name {
	case "search":
		return s.toolSearch(ctx, args)
	case "fts":
		return s.toolFullTextSearch(ctx, args)
	case "hybrid":
		return s.toolHybridSearch(ctx, args)
	case "status":
		return s.toolStatus(ctx)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *Server) toolSearch(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return errorContent("query is required"), nil
	}

	topK := getInt(args, "limit", 5)
	minScore := getFloat(args, "min_score", 0)

	if s.memoryClient == nil {
		return errorContent("memory service not connected"), nil
	}

	resp, err := s.memoryClient.SemanticSearch(ctx, &memoryv1.SearchRequest{
		Query:    query,
		TopK:     int32(topK),
		MinScore: float32(minScore),
	})
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}

	return formatSearchResults(resp.GetResults(), query), nil
}

func (s *Server) toolFullTextSearch(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return errorContent("query is required"), nil
	}

	topK := getInt(args, "limit", 5)
	minScore := getFloat(args, "min_score", 0)

	if s.memoryClient == nil {
		return errorContent("memory service not connected"), nil
	}

	resp, err := s.memoryClient.FullTextSearch(ctx, &memoryv1.SearchRequest{
		Query:    query,
		TopK:     int32(topK),
		MinScore: float32(minScore),
	})
	if err != nil {
		return nil, fmt.Errorf("full-text search: %w", err)
	}

	return formatSearchResults(resp.GetResults(), query), nil
}

func (s *Server) toolHybridSearch(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return errorContent("query is required"), nil
	}

	topK := getInt(args, "limit", 5)
	minScore := getFloat(args, "min_score", 0)

	if s.memoryClient == nil {
		return errorContent("memory service not connected"), nil
	}

	resp, err := s.memoryClient.HybridSearch(ctx, &memoryv1.SearchRequest{
		Query:    query,
		TopK:     int32(topK),
		MinScore: float32(minScore),
	})
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}

	return formatSearchResults(resp.GetResults(), query), nil
}

func (s *Server) toolStatus(ctx context.Context) (interface{}, error) {
	if s.memoryClient == nil {
		return errorContent("memory service not connected"), nil
	}

	resp, err := s.memoryClient.GetStats(ctx, &memoryv1.StatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	text := fmt.Sprintf(
		"Second Brain Index Status:\n  Documents: %d\n  Chunks: %d\n  Graph Triples: %d",
		resp.GetTotalDocuments(),
		resp.GetTotalChunks(),
		resp.GetTotalGraphTriples(),
	)
	if resp.GetLastIndexedAt() != nil {
		text += fmt.Sprintf("\n  Last Indexed: %s", resp.GetLastIndexedAt().AsTime().Format("2006-01-02 15:04:05"))
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	}, nil
}

// --- helpers ---

func formatSearchResults(results []*memoryv1.SearchResult, query string) map[string]interface{} {
	if len(results) == 0 {
		return map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("No results found for %q", query)},
			},
		}
	}

	text := fmt.Sprintf("Found %d result(s) for %q:\n\n", len(results), query)
	for _, r := range results {
		text += fmt.Sprintf("  [%.0f%%] %s\n", r.GetScore()*100, r.GetDocumentId())
		content := r.GetContent()
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		text += fmt.Sprintf("  %s\n\n", content)
	}

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	}
}

func errorContent(msg string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": msg},
		},
		"isError": true,
	}
}

func writeError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: message},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func getInt(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

func getFloat(args map[string]interface{}, key string, defaultVal float32) float32 {
	if v, ok := args[key]; ok {
		if n, ok := v.(float64); ok {
			return float32(n)
		}
	}
	return defaultVal
}
