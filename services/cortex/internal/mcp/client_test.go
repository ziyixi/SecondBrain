package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["method"] != "tools/list" {
			t.Errorf("expected method tools/list, got %v", body["method"])
		}

		resp := map[string]interface{}{
			"result": map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name":        "notion_search",
						"description": "Search Notion pages",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "notion_search" {
		t.Errorf("expected tool name 'notion_search', got %q", tools[0].Name)
	}
}

func TestCallTool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["method"] != "tools/call" {
			t.Errorf("expected method tools/call, got %v", body["method"])
		}

		resp := map[string]interface{}{
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "Page found: My Notes"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	result, err := client.CallTool(context.Background(), "notion_search", map[string]interface{}{
		"query": "notes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Text != "Page found: My Notes" {
		t.Errorf("unexpected content: %q", result.Content[0].Text)
	}
}

func TestCallToolServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.CallTool(context.Background(), "bad_tool", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetToken(t *testing.T) {
	client := NewClient("http://localhost", "old-token")
	client.SetToken("new-token")

	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.token != "new-token" {
		t.Errorf("expected new-token, got %q", client.token)
	}
}

func TestAuthorizationHeader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{"tools": []interface{}{}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "my-secret-token")
	client.ListTools(context.Background())

	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("expected 'Bearer my-secret-token', got %q", gotAuth)
	}
}
