package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
	"github.com/yourusername/secondbrain/test/fakes"
)

func getFreePort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

// repoRoot returns the repo root (directory containing go.mod) for building the integration server.
func repoRoot(t *testing.T) string {
	for _, base := range []string{".", "..", "../.."} {
		p := filepath.Join(base, "go.mod")
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(base)
			return abs
		}
	}
	t.Skip("go.mod not found (run tests from repo root or internal/api)")
	return ""
}

// composePath returns the path to the integration docker-compose file (relative to repo root).
func composePath(t *testing.T) string {
	for _, base := range []string{".", "..", "../.."} {
		p := filepath.Join(base, "test", "integration", "docker-compose.yml")
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	t.Skip("docker-compose.yml not found (run tests from repo root or internal/api)")
	return ""
}

// startCompose runs `docker compose -p <project> -f <path> up -d` and waits for Qdrant.
// grpcPort and httpPort are used for QDRANT_GRPC_PORT and QDRANT_HTTP_PORT (gRPC for client, REST for /readyz).
// Waits for TCP on gRPC port then for HTTP GET /readyz on REST port so Qdrant is ready (avoids "connection reset by peer" in CI).
func startCompose(t *testing.T, composePath string, projectName string, grpcPort string, httpPort string) {
	dir := filepath.Dir(composePath)
	cmd := exec.Command("docker", "compose", "-p", projectName, "-f", composePath, "up", "-d")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "QDRANT_GRPC_PORT="+grpcPort, "QDRANT_HTTP_PORT="+httpPort)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker compose up: %v\n%s", err, out)
	}
	deadline := time.Now().Add(60 * time.Second)
	var tcpOK bool
	for time.Now().Before(deadline) {
		c, err := net.Dial("tcp", "127.0.0.1:"+grpcPort)
		if err == nil {
			c.Close()
			tcpOK = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !tcpOK {
		t.Fatalf("timeout waiting for Qdrant gRPC TCP on %s", grpcPort)
	}
	readyURL := "http://127.0.0.1:" + httpPort + "/readyz"
	for time.Now().Before(deadline) {
		resp, err := http.Get(readyURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for Qdrant readyz on %s", httpPort)
}

// stopCompose runs `docker compose -p <project> -f <path> down`.
func stopCompose(composePath string, projectName string) {
	dir := filepath.Dir(composePath)
	cmd := exec.Command("docker", "compose", "-p", projectName, "-f", composePath, "down", "-v")
	cmd.Dir = dir
	_ = cmd.Run()
}

// waitForServer polls baseURL/health until 200 or timeout.
func waitForServer(t *testing.T, baseURL string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for server at %s", baseURL)
}

// TestIntegration_MemorySearchAndGoal starts the fake Go server (cmd/integration-server) with
// Docker Compose Qdrant, seeds Qdrant, then sends HTTP requests to the real server and asserts response.
func TestIntegration_MemorySearchAndGoal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	composeFile := composePath(t)
	root := repoRoot(t)
	projectName := "secondbrain-api-integration"
	grpcPort := getFreePort(t)
	httpPort := getFreePort(t)
	serverPort := getFreePort(t)
	startCompose(t, composeFile, projectName, grpcPort, httpPort)
	defer stopCompose(composeFile, projectName)

	os.Setenv("QDRANT_HOST", "127.0.0.1")
	os.Setenv("QDRANT_PORT", grpcPort)
	defer os.Unsetenv("QDRANT_HOST")
	defer os.Unsetenv("QDRANT_PORT")

	ctx := context.Background()
	embedder := func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1, 0.2, 0.3, 0.4}, nil
	}
	kb, err := memory.NewQdrantKnowledgeBase(embedder, nil)
	if err != nil {
		t.Fatalf("NewQdrantKnowledgeBase: %v", err)
	}
	defer kb.Close()
	docText := "Golang is a programming language. It is used for backend services and cloud tools."
	if err := kb.Upsert(ctx, "page-golang", docText); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Build and start the fake Go server (real HTTP server with faked LLM/Notion, real Qdrant).
	binPath := filepath.Join(t.TempDir(), "integration-server")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/integration-server")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build integration-server: %v\n%s", err, out)
	}
	serverEnv := append(os.Environ(), "PORT="+serverPort, "QDRANT_HOST=127.0.0.1", "QDRANT_PORT="+grpcPort)
	serverCmd := exec.Command(binPath)
	serverCmd.Dir = root
	serverCmd.Env = serverEnv
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("start integration-server: %v", err)
	}
	defer func() { _ = serverCmd.Process.Kill() }()

	baseURL := "http://127.0.0.1:" + serverPort
	waitForServer(t, baseURL, 15*time.Second)

	body := ChatCompletionRequest{
		Model: "test",
		Messages: []ChatMessage{
			{Role: "user", Content: "What do you know about Golang?"},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		t.Fatalf("status = %d, body: %s", resp.StatusCode, buf.String())
	}

	var chatResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(chatResp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	content := chatResp.Choices[0].Message.Content

	if !strings.Contains(strings.ToLower(content), "golang") {
		t.Errorf("response should mention golang (from memory search), got: %q", content)
	}
	if !strings.Contains(strings.ToLower(content), "programming") {
		t.Errorf("response should reflect KB content (programming), got: %q", content)
	}
}

// TestIntegration_UpsertFactAndRecall uses only fakes (no compose); scripted LLM calls UpsertUserFact.
func TestIntegration_UpsertFactAndRecall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	factStore := fakes.NewInMemoryFactStore()
	scriptedLLM := fakes.NewScriptedLLM(
		&llm.GenerateOutput{
			Content: "",
			ToolCalls: []llm.ToolCall{
				{ID: "1", Name: "UpsertUserFact", Args: map[string]any{"fact": "User's favorite color is blue."}},
			},
			FinishReason: "stop",
		},
		&llm.GenerateOutput{
			Content:      "I've saved that you like the color blue.",
			FinishReason: "stop",
		},
	)

	chat := &ChatHandler{
		LLM:     scriptedLLM,
		Working: memory.NewWorkingMemory(20),
		Facts:   factStore,
		KB:      nil,
	}
	router := Router(chat)

	body := ChatCompletionRequest{
		Model: "test",
		Messages: []ChatMessage{
			{Role: "user", Content: "Remember that my favorite color is blue."},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	facts := factStore.Facts("default")
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact stored, got %d: %v", len(facts), facts)
	}
	if facts[0] != "User's favorite color is blue." {
		t.Errorf("fact = %q, want %q", facts[0], "User's favorite color is blue.")
	}
}
