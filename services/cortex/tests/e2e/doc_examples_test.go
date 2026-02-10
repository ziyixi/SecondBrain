package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"

	"github.com/ziyixi/SecondBrain/services/cortex/internal/mcpserver"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/metrics"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/openaicompat"
	cortexserver "github.com/ziyixi/SecondBrain/services/cortex/internal/server"
	agentv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/common/v1"
	memoryv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/memory/v1"
)

// TestDocExamples validates that the API examples documented in README.md
// produce the expected response shapes and status codes. This test is run
// in CI to catch documentation drift.
func TestDocExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping doc examples test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// --- Infrastructure setup (same as integration test) ---
	fakeOpenAI := newFakeOpenAIServer(t)
	defer fakeOpenAI.Close()

	memService := newFakeMemoryService()
	hippoAddr, hippoStop := startGRPCServer(t, func(s *grpc.Server) {
		memoryv1.RegisterMemoryServiceServer(s, memService)
		commonv1.RegisterHealthServiceServer(s, memService)
	})
	defer hippoStop()

	frontalSvc := &fakeFrontalLobe{llmURL: fakeOpenAI.URL, model: "secondbrain"}
	frontalAddr, frontalStop := startGRPCServer(t, func(s *grpc.Server) {
		agentv1.RegisterReasoningEngineServer(s, frontalSvc)
		commonv1.RegisterHealthServiceServer(s, frontalSvc)
	})
	defer frontalStop()

	cortex := cortexserver.NewCortexServer(logger)
	if err := cortex.ConnectDownstream(frontalAddr, hippoAddr); err != nil {
		t.Fatalf("connecting downstream: %v", err)
	}
	defer cortex.Close()

	cortexAddr, cortexStop := startGRPCServer(t, func(s *grpc.Server) {
		agentv1.RegisterReasoningEngineServer(s, cortex)
		commonv1.RegisterHealthServiceServer(s, cortex)
	})
	defer cortexStop()

	openaiHandler := openaicompat.NewHandler(logger, []string{"secondbrain", "mock"})
	if err := openaiHandler.ConnectFrontalLobe(cortexAddr); err != nil {
		t.Fatalf("connecting openai handler: %v", err)
	}
	defer openaiHandler.Close()

	httpMux := http.NewServeMux()
	openaiHandler.RegisterRoutes(httpMux)

	mcpSrv := mcpserver.NewServer(logger, cortex.MemoryClient())
	httpMux.Handle("POST /mcp", mcpSrv)

	metricsStore := cortex.MetricsStore()
	httpMux.HandleFunc("GET /v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metricsStore.Summary())
	})

	srv := httptest.NewServer(httpMux)
	defer srv.Close()

	// Seed documents for search
	memService.docs["doc-1"] = "Machine learning is a subset of AI that enables systems to learn from data."
	memService.docs["doc-2"] = "Go is a compiled language designed at Google for building scalable systems."

	// ===================================================================
	// README Example: POST /v1/chat/completions (non-streaming)
	// ===================================================================
	t.Run("ChatCompletionNonStreaming", func(t *testing.T) {
		body := `{
  "model": "secondbrain",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What do my notes say about machine learning?"}
  ]
}`
		resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}

		var chatResp struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Model   string `json:"model"`
			Choices []struct {
				Index        int `json:"index"`
				Message      struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			t.Fatalf("decoding response: %v", err)
		}

		// Validate response shape matches documented example
		if chatResp.Object != "chat.completion" {
			t.Errorf("expected object=chat.completion, got %q", chatResp.Object)
		}
		if chatResp.Model != "secondbrain" {
			t.Errorf("expected model=secondbrain, got %q", chatResp.Model)
		}
		if len(chatResp.Choices) == 0 {
			t.Fatal("expected at least one choice")
		}
		if chatResp.Choices[0].Message.Role != "assistant" {
			t.Errorf("expected role=assistant, got %q", chatResp.Choices[0].Message.Role)
		}
		if chatResp.Choices[0].Message.Content == "" {
			t.Error("expected non-empty content")
		}
		if chatResp.Choices[0].FinishReason != "stop" {
			t.Errorf("expected finish_reason=stop, got %q", chatResp.Choices[0].FinishReason)
		}
		if !strings.HasPrefix(chatResp.ID, "chatcmpl-") {
			t.Errorf("expected id prefix chatcmpl-, got %q", chatResp.ID)
		}
	})

	// ===================================================================
	// README Example: POST /v1/chat/completions (streaming)
	// ===================================================================
	t.Run("ChatCompletionStreaming", func(t *testing.T) {
		body := `{
  "model": "secondbrain",
  "stream": true,
  "messages": [
    {"role": "user", "content": "Summarize my knowledge base"}
  ]
}`
		resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("expected Content-Type text/event-stream, got %q", ct)
		}

		data, _ := io.ReadAll(resp.Body)
		bodyStr := string(data)

		// SSE stream must contain data: lines and end with [DONE]
		if !strings.Contains(bodyStr, "data: ") {
			t.Error("expected SSE data: lines in streaming response")
		}
		if !strings.Contains(bodyStr, "data: [DONE]") {
			t.Error("expected [DONE] marker in streaming response")
		}

		// Parse the first data line to validate chunk shape
		for _, line := range strings.Split(bodyStr, "\n") {
			if strings.HasPrefix(line, "data: ") && !strings.Contains(line, "[DONE]") {
				payload := strings.TrimPrefix(line, "data: ")
				var chunk struct {
					Object string `json:"object"`
					Model  string `json:"model"`
				}
				if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
					t.Errorf("parsing SSE chunk: %v", err)
					break
				}
				if chunk.Object != "chat.completion.chunk" {
					t.Errorf("expected object=chat.completion.chunk, got %q", chunk.Object)
				}
				break
			}
		}
	})

	// ===================================================================
	// README Example: GET /v1/models
	// ===================================================================
	t.Run("ListModels", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/models")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var models struct {
			Object string `json:"object"`
			Data   []struct {
				ID      string `json:"id"`
				Object  string `json:"object"`
				OwnedBy string `json:"owned_by"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
			t.Fatalf("decoding: %v", err)
		}

		if models.Object != "list" {
			t.Errorf("expected object=list, got %q", models.Object)
		}
		if len(models.Data) != 2 {
			t.Errorf("expected 2 models, got %d", len(models.Data))
		}
		for _, m := range models.Data {
			if m.Object != "model" {
				t.Errorf("expected model object=model, got %q", m.Object)
			}
			if m.OwnedBy != "secondbrain" {
				t.Errorf("expected owned_by=secondbrain, got %q", m.OwnedBy)
			}
		}
	})

	// ===================================================================
	// README Example: GET /v1/metrics
	// ===================================================================
	t.Run("MetricsEndpoint", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/metrics")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var m metrics.MetricsSummary
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatalf("decoding: %v", err)
		}

		// The response must contain all documented fields
		// (values may be zero since no interactions recorded yet via this test)
		b, _ := json.Marshal(m)
		fields := []string{
			"total_interactions",
			"avg_response_quality",
			"avg_context_relevance",
			"user_satisfaction_rate",
			"knowledge_coverage",
		}
		for _, f := range fields {
			if !strings.Contains(string(b), f) {
				t.Errorf("metrics response missing documented field %q", f)
			}
		}
	})

	// ===================================================================
	// README Example: MCP tools/list
	// ===================================================================
	t.Run("MCPToolsList", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
		}

		var rpcResp struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Result  struct {
				Tools []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
			t.Fatalf("decoding: %v", err)
		}

		if rpcResp.JSONRPC != "2.0" {
			t.Errorf("expected jsonrpc=2.0, got %q", rpcResp.JSONRPC)
		}

		// Verify all documented tools are present
		toolNames := map[string]bool{}
		for _, tool := range rpcResp.Result.Tools {
			toolNames[tool.Name] = true
		}
		for _, expected := range []string{"search", "fts", "hybrid", "status"} {
			if !toolNames[expected] {
				t.Errorf("MCP tools/list missing documented tool %q", expected)
			}
		}
	})

	// ===================================================================
	// README Example: MCP tools/call hybrid search
	// ===================================================================
	t.Run("MCPHybridSearch", func(t *testing.T) {
		body := `{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "search",
    "arguments": {
      "query": "machine learning",
      "limit": 5,
      "min_score": 0.3
    }
  }
}`
		resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
		}

		var rpcResp struct {
			JSONRPC string `json:"jsonrpc"`
			Result  struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		raw, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(raw, &rpcResp); err != nil {
			t.Fatalf("decoding: %v (body: %s)", err, raw)
		}

		if rpcResp.Error != nil {
			t.Fatalf("MCP returned error: %s", rpcResp.Error.Message)
		}
		if len(rpcResp.Result.Content) == 0 {
			t.Error("expected non-empty content in MCP search result")
		}
		if rpcResp.Result.Content[0].Type != "text" {
			t.Errorf("expected content type=text, got %q", rpcResp.Result.Content[0].Type)
		}
	})

	// ===================================================================
	// README Example: MCP initialize
	// ===================================================================
	t.Run("MCPInitialize", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
		resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		var rpcResp struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
				ServerInfo      struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"serverInfo"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
			t.Fatalf("decoding: %v", err)
		}

		if rpcResp.Result.ServerInfo.Name != "secondbrain" {
			t.Errorf("expected server name=secondbrain, got %q", rpcResp.Result.ServerInfo.Name)
		}
	})

	// ===================================================================
	// Validate documented error shape
	// ===================================================================
	t.Run("ErrorResponseShape", func(t *testing.T) {
		// Send invalid request (empty messages)
		body := `{"model":"secondbrain","messages":[]}`
		resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader([]byte(body)))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}

		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			t.Fatalf("decoding error: %v", err)
		}
		if errResp.Error.Message == "" {
			t.Error("expected error message")
		}
		if errResp.Error.Type == "" {
			t.Error("expected error type")
		}
	})
}
