package openaicompat

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleListModels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, []string{"gpt-4", "gemini-pro", "mock"})

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ModelList
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %q", resp.Object)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 models, got %d", len(resp.Data))
	}

	// Check model IDs
	ids := map[string]bool{}
	for _, m := range resp.Data {
		ids[m.ID] = true
		if m.Object != "model" {
			t.Errorf("expected model object 'model', got %q", m.Object)
		}
		if m.OwnedBy != "secondbrain" {
			t.Errorf("expected owned_by 'secondbrain', got %q", m.OwnedBy)
		}
	}
	for _, id := range []string{"gpt-4", "gemini-pro", "mock"} {
		if !ids[id] {
			t.Errorf("expected model %q in list", id)
		}
	}
}

func TestHandleChatCompletionsNonStreaming(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, []string{"mock"})
	// No frontal lobe connected - will use fallback echo

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	chatReq := ChatCompletionRequest{
		Model: "mock",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello, world!"},
		},
	}
	body, _ := json.Marshal(chatReq)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", resp.Object)
	}
	if resp.Model != "mock" {
		t.Errorf("expected model 'mock', got %q", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", resp.Choices[0].Message.Role)
	}
	if !strings.Contains(resp.Choices[0].Message.Content, "Hello, world!") {
		t.Errorf("expected content to contain query, got %q", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", resp.Choices[0].FinishReason)
	}
}

func TestHandleChatCompletionsStreaming(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, []string{"mock"})

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	chatReq := ChatCompletionRequest{
		Model:  "mock",
		Stream: true,
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello!"},
		},
	}
	body, _ := json.Marshal(chatReq)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected content-type text/event-stream, got %q", contentType)
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, "data: ") {
		t.Error("expected SSE data in response")
	}
	if !strings.Contains(respBody, "data: [DONE]") {
		t.Error("expected [DONE] marker in response")
	}
}

func TestHandleChatCompletionsEmptyMessages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, []string{"mock"})

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	chatReq := ChatCompletionRequest{
		Model:    "mock",
		Messages: []ChatMessage{},
	}
	body, _ := json.Marshal(chatReq)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("decoding error response: %v", err)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("expected error type 'invalid_request_error', got %q", errResp.Error.Type)
	}
}

func TestHandleChatCompletionsInvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, []string{"mock"})

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestExtractQueryAndSystem(t *testing.T) {
	messages := []ChatMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "Second question"},
	}

	query, system := extractQueryAndSystem(messages)
	if system != "You are a helpful assistant." {
		t.Errorf("expected system prompt, got %q", system)
	}
	if query != "Second question" {
		t.Errorf("expected last user message, got %q", query)
	}
}

func TestNewChatCompletionResponse(t *testing.T) {
	resp := NewChatCompletionResponse("test-id", "gpt-4", "Hello!")
	if resp.ID != "test-id" {
		t.Errorf("expected id 'test-id', got %q", resp.ID)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", resp.Object)
	}
	if resp.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", resp.Model)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "Hello!" {
		t.Error("unexpected choices")
	}
}

func TestNewStreamChunk(t *testing.T) {
	// Content chunk
	chunk := NewStreamChunk("test-id", "gpt-4", "partial", false)
	if chunk.Choices[0].Delta.Content != "partial" {
		t.Errorf("expected content 'partial', got %q", chunk.Choices[0].Delta.Content)
	}
	if chunk.Choices[0].FinishReason != nil {
		t.Error("expected nil finish_reason for non-final chunk")
	}

	// Final chunk
	final := NewStreamChunk("test-id", "gpt-4", "", true)
	if final.Choices[0].FinishReason == nil || *final.Choices[0].FinishReason != "stop" {
		t.Error("expected finish_reason 'stop' for final chunk")
	}
}
