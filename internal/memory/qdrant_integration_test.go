package memory

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// composePath returns the path to test/integration/docker-compose.yml (run from repo root or package dir).
func composePath(t *testing.T) string {
	for _, base := range []string{".", "..", "../..", "../../.."} {
		p := filepath.Join(base, "test", "integration", "docker-compose.yml")
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	t.Skip("docker-compose.yml not found (run tests from repo root)")
	return ""
}

func getFreePort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

// startCompose runs docker compose up and waits for Qdrant gRPC TCP then REST /readyz (so gRPC is ready in CI).
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

func stopCompose(composePath string, projectName string) {
	dir := filepath.Dir(composePath)
	cmd := exec.Command("docker", "compose", "-p", projectName, "-f", composePath, "down", "-v")
	cmd.Dir = dir
	_ = cmd.Run()
}

// TestQdrantKnowledgeBase_Integration uses Docker Compose (test/integration/docker-compose.yml)
// to start Qdrant, then tests upsert and hybrid search.
func TestQdrantKnowledgeBase_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	composeFile := composePath(t)
	projectName := "secondbrain-memory-integration"
	grpcPort := getFreePort(t)
	httpPort := getFreePort(t)
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
	kb, err := NewQdrantKnowledgeBase(embedder, nil)
	if err != nil {
		t.Fatalf("NewQdrantKnowledgeBase: %v", err)
	}
	defer kb.Close()

	err = kb.Upsert(ctx, "page-1", "The quick brown fox jumps over the lazy dog. Golang is a programming language.")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	hits, err := kb.Search(ctx, "golang", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if hits[0].NotionPageID == "" {
		t.Error("expected non-empty NotionPageID")
	}
	if hits[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", hits[0].Score)
	}
}
