package reasoning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"
)

func TestRouterFallback(t *testing.T) {
	mock := NewMockLLM()
	router := NewRouter(mock)

	resp, err := router.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response from fallback")
	}
}

func TestRouterRegisterAndForModel(t *testing.T) {
	mock := NewMockLLM()
	router := NewRouter(mock)

	custom := NewMockLLM()
	router.Register("gpt-4", custom)
	router.Register("gemini-pro", custom)

	// Registered model returns the custom provider
	p := router.ForModel("gpt-4")
	if p == nil {
		t.Fatal("expected non-nil provider for gpt-4")
	}

	// Unknown model returns fallback
	p = router.ForModel("unknown-model")
	if p != mock {
		t.Error("expected fallback provider for unknown model")
	}
}

func TestRouterListModels(t *testing.T) {
	mock := NewMockLLM()
	router := NewRouter(mock)
	router.Register("gpt-4", mock)
	router.Register("gemini-pro", mock)

	models := router.ListModels()
	sort.Strings(models)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0] != "gemini-pro" || models[1] != "gpt-4" {
		t.Errorf("unexpected models: %v", models)
	}
}

func TestRouterGenerateWithModel(t *testing.T) {
	mock := NewMockLLM()
	router := NewRouter(mock)
	router.Register("gpt-4", mock)

	resp, err := router.GenerateWithModel(context.Background(), "gpt-4", "weekly review")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response")
	}
}

func TestRouterClassify(t *testing.T) {
	mock := NewMockLLM()
	router := NewRouter(mock)

	cat, conf, err := router.Classify(context.Background(), "urgent action needed", []string{"ACTIONABLE", "REFERENCE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != "ACTIONABLE" {
		t.Errorf("expected ACTIONABLE, got %s", cat)
	}
	if conf <= 0 {
		t.Error("expected positive confidence")
	}
}

func TestOpenAIProviderGenerate(t *testing.T) {
	// Mock OpenAI API server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or wrong authorization header")
		}

		resp := openAIChatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "Hello from OpenAI mock"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider := NewOpenAIProvider("test-key", srv.URL, "gpt-4", 10*time.Second)
	resp, err := provider.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "Hello from OpenAI mock" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestOpenAIProviderClassify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIChatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "ACTIONABLE"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider := NewOpenAIProvider("test-key", srv.URL, "gpt-4", 10*time.Second)
	cat, conf, err := provider.Classify(context.Background(), "urgent task", []string{"ACTIONABLE", "REFERENCE", "TRASH"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != "ACTIONABLE" {
		t.Errorf("expected ACTIONABLE, got %s", cat)
	}
	if conf <= 0 {
		t.Error("expected positive confidence")
	}
}

func TestOpenAIProviderAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIChatResponse{
			Error: &struct {
				Message string `json:"message"`
			}{Message: "rate limit exceeded"},
		}
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider := NewOpenAIProvider("test-key", srv.URL, "gpt-4", 10*time.Second)
	_, err := provider.Generate(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestGoogleProviderGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			t.Error("missing API key in query")
		}

		resp := googleGenResponse{
			Candidates: []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			}{
				{Content: struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				}{
					Parts: []struct {
						Text string `json:"text"`
					}{{Text: "Hello from Google mock"}},
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider := NewGoogleProvider("test-key", "gemini-pro", 10*time.Second)
	// Override the base URL to point to our test server
	provider.baseURL = srv.URL

	resp, err := provider.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "Hello from Google mock" {
		t.Errorf("unexpected response: %s", resp)
	}
}

func TestGoogleProviderClassify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := googleGenResponse{
			Candidates: []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			}{
				{Content: struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				}{
					Parts: []struct {
						Text string `json:"text"`
					}{{Text: "REFERENCE"}},
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider := NewGoogleProvider("test-key", "gemini-pro", 10*time.Second)
	provider.baseURL = srv.URL

	cat, _, err := provider.Classify(context.Background(), "article about Go", []string{"ACTIONABLE", "REFERENCE", "TRASH"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != "REFERENCE" {
		t.Errorf("expected REFERENCE, got %s", cat)
	}
}
