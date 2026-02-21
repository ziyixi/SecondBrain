package memory

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// fakeNotionWriter is an in-memory NotionKnowledgeWriter + NotionPageFetcher for integration tests (avoids import cycle with test/fakes).
type fakeNotionWriter struct {
	mu     sync.Mutex
	pages  map[string]string
	nextID int
}

func newFakeNotionWriter() *fakeNotionWriter {
	return &fakeNotionWriter{pages: make(map[string]string), nextID: 1}
}

func (f *fakeNotionWriter) CreatePage(ctx context.Context, title string, content string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("page-%d", f.nextID)
	f.nextID++
	if content != "" {
		f.pages[id] = title + "\n" + content
	} else {
		f.pages[id] = title
	}
	return id, nil
}

func (f *fakeNotionWriter) AppendToPage(ctx context.Context, pageID string, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[pageID] = f.pages[pageID] + "\n" + content
	return nil
}

func (f *fakeNotionWriter) GetPageText(ctx context.Context, pageID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pages[pageID], nil
}

func (f *fakeNotionWriter) pageIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.pages))
	for id := range f.pages {
		ids = append(ids, id)
	}
	return ids
}

func (f *fakeNotionWriter) getPage(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pages[id]
}

// TestMemorizer_Integration uses Docker Compose (Qdrant) + in-memory fake Notion to test
// MemorizeInformation: create new category, then Search returns the stored content.
func TestMemorizer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	composeFile := composePath(t)
	projectName := "secondbrain-memorizer-integration"
	grpcPort := getFreePort(t)
	httpPort := getFreePort(t)
	startCompose(t, composeFile, projectName, grpcPort, httpPort)
	defer stopCompose(composeFile, projectName)

	os.Setenv("QDRANT_HOST", "127.0.0.1")
	os.Setenv("QDRANT_PORT", grpcPort)
	defer os.Unsetenv("QDRANT_HOST")
	defer os.Unsetenv("QDRANT_PORT")

	ctx := context.Background()
	fakeNotion := newFakeNotionWriter()
	embedder := func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1, 0.2, 0.3, 0.4}, nil
	}
	kb, err := NewQdrantKnowledgeBase(embedder, fakeNotion)
	if err != nil {
		t.Fatalf("NewQdrantKnowledgeBase: %v", err)
	}
	defer kb.Close()

	mem := NewMemorizerService(kb, fakeNotion, 0.85)
	msg, err := mem.MemorizeInformation(ctx, "Go", "Go is a programming language.")
	if err != nil {
		t.Fatalf("MemorizeInformation: %v", err)
	}
	if !strings.Contains(msg, "Created new category") {
		t.Errorf("expected message to contain 'Created new category', got %q", msg)
	}
	ids := fakeNotion.pageIDs()
	if len(ids) != 1 {
		t.Fatalf("expected 1 page, got %d", len(ids))
	}
	pageText := fakeNotion.getPage(ids[0])
	if !strings.Contains(pageText, "Go") || !strings.Contains(pageText, "programming") {
		t.Errorf("expected page to contain topic and content, got %q", pageText)
	}

	hits, err := kb.Search(ctx, "programming language", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit from Search")
	}
	if hits[0].Text == "" {
		t.Error("expected hit text from fetcher (fake Notion)")
	}
	if !strings.Contains(hits[0].Text, "Go") {
		t.Errorf("expected hit to contain 'Go', got %q", hits[0].Text)
	}
}
