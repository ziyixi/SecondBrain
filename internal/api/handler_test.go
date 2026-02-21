package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/secondbrain/internal/llm"
)

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
