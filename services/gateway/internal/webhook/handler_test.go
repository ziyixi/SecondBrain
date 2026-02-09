package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"log/slog"
	"os"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestHandleEmail(t *testing.T) {
	h := NewHandler(newTestLogger(), "")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"subject": "Test Subject",
		"body":    "<p>Email body</p>",
		"from":    "test@example.com",
		"is_html": true,
	})

	req := httptest.NewRequest("POST", "/webhooks/email", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "accepted" {
		t.Errorf("expected accepted status, got %q", resp["status"])
	}
	if resp["item_id"] == "" {
		t.Error("expected non-empty item_id")
	}

	// Check that item was enqueued
	select {
	case item := <-h.Items():
		if item.Source != "email" {
			t.Errorf("expected source 'email', got %q", item.Source)
		}
	default:
		t.Error("expected item to be enqueued")
	}
}

func TestHandleSlack(t *testing.T) {
	h := NewHandler(newTestLogger(), "")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"text":    "Hello from Slack",
		"channel": "#general",
		"user":    "U123",
	})

	req := httptest.NewRequest("POST", "/webhooks/slack", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestHandleGeneric(t *testing.T) {
	h := NewHandler(newTestLogger(), "")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{
		"content":  "Generic webhook data",
		"source":   "custom-app",
		"metadata": map[string]string{"key": "value"},
	})

	req := httptest.NewRequest("POST", "/webhooks/generic", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestHandleGitHubMissingHeader(t *testing.T) {
	h := NewHandler(newTestLogger(), "")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleInvalidJSON(t *testing.T) {
	h := NewHandler(newTestLogger(), "")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/webhooks/email", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	h := NewHandler(newTestLogger(), "")
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
