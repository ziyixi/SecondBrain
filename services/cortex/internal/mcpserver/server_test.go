package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"
	"os"

	memoryv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/memory/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockMemoryClient implements memoryv1.MemoryServiceClient for testing.
type mockMemoryClient struct {
	memoryv1.MemoryServiceClient
	searchResults   *memoryv1.SearchResponse
	ftsResults      *memoryv1.SearchResponse
	hybridResults   *memoryv1.SearchResponse
	statsResp       *memoryv1.StatsResponse
}

func (m *mockMemoryClient) SemanticSearch(ctx context.Context, in *memoryv1.SearchRequest, opts ...grpc.CallOption) (*memoryv1.SearchResponse, error) {
	if m.searchResults != nil {
		return m.searchResults, nil
	}
	return &memoryv1.SearchResponse{}, nil
}

func (m *mockMemoryClient) FullTextSearch(ctx context.Context, in *memoryv1.SearchRequest, opts ...grpc.CallOption) (*memoryv1.SearchResponse, error) {
	if m.ftsResults != nil {
		return m.ftsResults, nil
	}
	return &memoryv1.SearchResponse{}, nil
}

func (m *mockMemoryClient) HybridSearch(ctx context.Context, in *memoryv1.SearchRequest, opts ...grpc.CallOption) (*memoryv1.SearchResponse, error) {
	if m.hybridResults != nil {
		return m.hybridResults, nil
	}
	return &memoryv1.SearchResponse{}, nil
}

func (m *mockMemoryClient) GetStats(ctx context.Context, in *memoryv1.StatsRequest, opts ...grpc.CallOption) (*memoryv1.StatsResponse, error) {
	if m.statsResp != nil {
		return m.statsResp, nil
	}
	return &memoryv1.StatsResponse{}, nil
}

func newTestServer() *Server {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	mock := &mockMemoryClient{
		searchResults: &memoryv1.SearchResponse{
			Results: []*memoryv1.SearchResult{
				{DocumentId: "doc-1", Content: "Seismic detection research", Score: 0.95},
			},
		},
		ftsResults: &memoryv1.SearchResponse{
			Results: []*memoryv1.SearchResult{
				{DocumentId: "doc-2", Content: "Full text search result", Score: 0.8},
			},
		},
		hybridResults: &memoryv1.SearchResponse{
			Results: []*memoryv1.SearchResult{
				{DocumentId: "doc-3", Content: "Hybrid search result", Score: 0.9},
			},
		},
		statsResp: &memoryv1.StatsResponse{
			TotalDocuments:    10,
			TotalChunks:       42,
			TotalGraphTriples: 5,
			LastIndexedAt:     timestamppb.Now(),
		},
	}
	return NewServer(logger, mock)
}

func doRPC(t *testing.T, srv *Server, method string, params map[string]interface{}) jsonRPCResponse {
	t.Helper()
	body := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var resp jsonRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func TestInitialize(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "initialize", nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}
	info, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("expected serverInfo")
	}
	if info["name"] != "secondbrain" {
		t.Errorf("expected name 'secondbrain', got %v", info["name"])
	}
}

func TestToolsList(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "tools/list", nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("expected tools array")
	}
	if len(tools) != 4 {
		t.Errorf("expected 4 tools, got %d", len(tools))
	}
}

func TestToolSearch(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "tools/call", map[string]interface{}{
		"name":      "search",
		"arguments": map[string]interface{}{"query": "seismic"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}
	content, ok := result["content"].([]interface{})
	if !ok {
		t.Fatal("expected content array")
	}
	if len(content) == 0 {
		t.Fatal("expected content")
	}
}

func TestToolFTS(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "tools/call", map[string]interface{}{
		"name":      "fts",
		"arguments": map[string]interface{}{"query": "keyword search"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
}

func TestToolHybrid(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "tools/call", map[string]interface{}{
		"name":      "hybrid",
		"arguments": map[string]interface{}{"query": "hybrid query"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
}

func TestToolStatus(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "tools/call", map[string]interface{}{
		"name": "status",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}
	content, ok := result["content"].([]interface{})
	if !ok {
		t.Fatal("expected content array")
	}
	if len(content) == 0 {
		t.Fatal("expected status content")
	}
}

func TestUnknownTool(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "tools/call", map[string]interface{}{
		"name": "nonexistent",
	})

	if resp.Error == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "unknown/method", nil)

	if resp.Error == nil {
		t.Error("expected error for unknown method")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	srv := newTestServer()
	resp := doRPC(t, srv, "tools/call", map[string]interface{}{
		"name":      "search",
		"arguments": map[string]interface{}{"query": ""},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %s", resp.Error.Message)
	}

	// Should return isError in the tool result
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result map")
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected isError=true for empty query")
	}
}

func TestGetOnly(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
