package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/secondbrain/internal/config"
	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
)

// testConfig returns a config for handler tests (uses Load() so env can override).
func testConfig() *config.Config {
	return config.Load()
}

// Mock LLM client
type mockLLM struct {
	generateFunc func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error)
	embedFunc    func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockLLM) Generate(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, in)
	}
	return &llm.GenerateOutput{Content: "Hello", FinishReason: "stop"}, nil
}

func (m *mockLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	return []float32{0.1}, nil
}

// Mock working memory
type mockWorking struct {
	messages []struct{ Role, Content string }
}

func (m *mockWorking) Append(role, content string) {
	m.messages = append(m.messages, struct{ Role, Content string }{role, content})
}
func (m *mockWorking) Messages() []struct{ Role, Content string } {
	return m.messages
}
func (m *mockWorking) Clear() {
	m.messages = nil
}

type mockMemorizer struct {
	result string
	err    error
}

func (m *mockMemorizer) MemorizeInformation(ctx context.Context, topic string, content string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func TestHandleChatCompletions_Unit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		body         ChatCompletionRequest
		mockGenerate func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error)
		wantStatus   int
		wantContent  string
		memorizer    *mockMemorizer
	}{
		{
			name: "simple message",
			body: ChatCompletionRequest{
				Model: "gemini-2.5-flash",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hi"},
				},
			},
			mockGenerate: func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
				return &llm.GenerateOutput{Content: "Hi there!", FinishReason: "stop"}, nil
			},
			wantStatus:  http.StatusOK,
			wantContent: "Hi there!",
		},
		{
			name:      "empty messages rejected",
			memorizer: nil,
			body: ChatCompletionRequest{
				Model:    "gemini-2.5-flash",
				Messages: nil,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "tool call then reply",
			body: ChatCompletionRequest{
				Model: "gemini-2.5-flash",
				Messages: []ChatMessage{
					{Role: "user", Content: "Remember I like coffee"},
				},
			},
			mockGenerate: func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
				// First call: return tool call
				if len(in.Messages) == 1 {
					return &llm.GenerateOutput{
						Content: "",
						ToolCalls: []llm.ToolCall{
							{ID: "1", Name: "UpsertUserFact", Args: map[string]any{"fact": "User likes coffee"}},
						},
						FinishReason: "stop",
					}, nil
				}
				// Second call: return final text
				return &llm.GenerateOutput{Content: "I'll remember that.", FinishReason: "stop"}, nil
			},
			wantStatus:  http.StatusOK,
			wantContent: "I'll remember that.",
		},
		{
			name: "MemorizeInformation tool",
			body: ChatCompletionRequest{
				Model: "gemini-2.5-flash",
				Messages: []ChatMessage{
					{Role: "user", Content: "Store this: Go is a programming language."},
				},
			},
			mockGenerate: func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
				if len(in.Messages) == 1 {
					return &llm.GenerateOutput{
						Content: "",
						ToolCalls: []llm.ToolCall{
							{ID: "1", Name: "MemorizeInformation", Args: map[string]any{"topic": "Programming", "content": "Go is a programming language."}},
						},
						FinishReason: "stop",
					}, nil
				}
				return &llm.GenerateOutput{Content: "Stored under Programming.", FinishReason: "stop"}, nil
			},
			wantStatus:  http.StatusOK,
			wantContent: "Stored under Programming.",
			memorizer:   &mockMemorizer{result: "Created new category page: page-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmMock := &mockLLM{generateFunc: tt.mockGenerate}
			var mem Memorizer
			if tt.memorizer != nil {
				mem = tt.memorizer
			}
			chat := &ChatHandler{
				Config:    testConfig(),
				LLM:       llmMock,
				Working:   &mockWorking{},
				Facts:     nil,
				KB:        nil,
				Memorizer: mem,
			}
			r := Router(chat)

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var resp ChatCompletionResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(resp.Choices) == 0 {
				t.Fatal("expected at least one choice")
			}
			if resp.Choices[0].Message.Content != tt.wantContent {
				t.Errorf("content = %q, want %q", resp.Choices[0].Message.Content, tt.wantContent)
			}
		})
	}
}

// mockKB is a knowledge base that returns a configurable error for tests.
type mockKB struct {
	searchErr error
}

func (m *mockKB) Search(ctx context.Context, query string, limit int) ([]memory.DocumentHit, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return []memory.DocumentHit{}, nil
}

func (m *mockKB) Upsert(ctx context.Context, notionPageID string, text string) error {
	return nil
}

func TestHandleChatCompletions_SearchKnowledgeBaseError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	kbErr := errors.New("search failed")
	callCount := 0
	llmMock := &mockLLM{
		generateFunc: func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
			callCount++
			if callCount == 1 {
				return &llm.GenerateOutput{
					Content: "",
					ToolCalls: []llm.ToolCall{
						{ID: "1", Name: "SearchKnowledgeBase", Args: map[string]any{"query": "test"}},
					},
					FinishReason: "stop",
				}, nil
			}
			return &llm.GenerateOutput{Content: "Search failed.", FinishReason: "stop"}, nil
		},
	}
	chat := &ChatHandler{
		Config:  testConfig(),
		LLM:     llmMock,
		Working: &mockWorking{},
		KB:      &mockKB{searchErr: kbErr},
	}
	r := Router(chat)
	body := ChatCompletionRequest{
		Model:    "default",
		Messages: []ChatMessage{{Role: "user", Content: "Search for X"}},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Second turn: model received tool result "error: search failed" and replied
	if resp.Choices[0].Message.Content == "" {
		t.Error("expected non-empty assistant content after tool error")
	}
}

func TestHandleChatCompletions_UnknownTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	llmMock := &mockLLM{
		generateFunc: func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
			calls++
			if calls == 1 {
				return &llm.GenerateOutput{
					Content: "",
					ToolCalls: []llm.ToolCall{
						{ID: "1", Name: "NonExistentTool", Args: map[string]any{}},
					},
					FinishReason: "stop",
				}, nil
			}
			return &llm.GenerateOutput{Content: "I don't have that tool.", FinishReason: "stop"}, nil
		},
	}
	chat := &ChatHandler{
		Config:  testConfig(),
		LLM:     llmMock,
		Working: &mockWorking{},
	}
	r := Router(chat)
	body := ChatCompletionRequest{
		Model:    "default",
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Choices[0].Message.Content == "" {
		t.Error("expected non-empty reply after unknown tool result")
	}
}

func TestHandleChatCompletions_UpsertUserFactEmptyFact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	llmMock := &mockLLM{
		generateFunc: func(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
			calls++
			if calls == 1 {
				return &llm.GenerateOutput{
					Content: "",
					ToolCalls: []llm.ToolCall{
						{ID: "1", Name: "UpsertUserFact", Args: map[string]any{"fact": ""}},
					},
					FinishReason: "stop",
				}, nil
			}
			return &llm.GenerateOutput{Content: "I need a fact to save.", FinishReason: "stop"}, nil
		},
	}
	chat := &ChatHandler{
		Config:  testConfig(),
		LLM:     llmMock,
		Working: &mockWorking{},
		Facts:   &mockFacts{},
	}
	r := Router(chat)
	body := ChatCompletionRequest{
		Model:    "default",
		Messages: []ChatMessage{{Role: "user", Content: "Remember this"}},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Choices[0].Message.Content == "" {
		t.Error("expected non-empty reply after empty fact tool result")
	}
}

// mockFacts implements memory.UserFactStore for tests.
type mockFacts struct{}

func (m *mockFacts) GetProfile(ctx context.Context, userID string) (string, error) {
	return "", nil
}

func (m *mockFacts) AppendFact(ctx context.Context, userID string, fact string) error {
	return nil
}

func TestOpenAIMessagesToLLM(t *testing.T) {
	h := &ChatHandler{Config: testConfig()}
	msgs := h.openAIMessagesToLLM([]ChatMessage{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello!"},
	})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system skipped), got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hi" {
		t.Errorf("first message = %+v", msgs[0])
	}
	if msgs[1].Role != "model" || msgs[1].Content != "Hello!" {
		t.Errorf("assistant should become model role: %+v", msgs[1])
	}
}

func TestRouter_Health(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := Router(&ChatHandler{Config: testConfig()})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /health status = %d, want %d", rec.Code, http.StatusOK)
	}
}
