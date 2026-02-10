package server

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/config"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/embedder"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/vectorstore"
	commonv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/common/v1"
	memoryv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/memory/v1"
)

func newTestServer() *HippocampusServer {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := &config.Config{
		CollectionName:     "test",
		EmbeddingDimension: 32,
		ChunkSize:          50,
		ChunkOverlap:       5,
	}
	store := vectorstore.NewInMemoryStore()
	emb := embedder.NewMockEmbedder(32)
	return NewHippocampusServer(logger, cfg, store, emb)
}

func TestHippocampusHealthCheck(t *testing.T) {
	s := newTestServer()
	resp, err := s.Check(context.Background(), &commonv1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != commonv1.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING, got %v", resp.Status)
	}
}

func TestIndexAndSearch(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	// Index a document
	indexResp, err := s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId:       "doc-1",
		Content:          "The PhaseNet-TF model extends the original PhaseNet architecture for seismic signal detection using transfer learning techniques.",
		ChunkingStrategy: memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_SEMANTIC,
		Metadata:         map[string]string{"type": "research"},
	})
	if err != nil {
		t.Fatalf("index error: %v", err)
	}
	if !indexResp.Success {
		t.Fatalf("indexing failed: %s", indexResp.ErrorMessage)
	}
	if indexResp.ChunksCreated == 0 {
		t.Error("expected chunks to be created")
	}

	// Search
	searchResp, err := s.SemanticSearch(ctx, &memoryv1.SearchRequest{
		Query: "seismic detection",
		TopK:  3,
	})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(searchResp.Results) == 0 {
		t.Error("expected search results")
	}
}

func TestIndexEmptyContent(t *testing.T) {
	s := newTestServer()
	resp, err := s.IndexDocument(context.Background(), &memoryv1.IndexRequest{
		DocumentId: "doc-empty",
		Content:    "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Error("expected failure for empty content")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	s := newTestServer()
	_, err := s.SemanticSearch(context.Background(), &memoryv1.SearchRequest{
		Query: "",
	})
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestAddAndQueryGraphTriple(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	// Add triple
	tripleResp, err := s.AddGraphTriple(ctx, &memoryv1.GraphTripleRequest{
		Subject:   "PhaseNet-TF",
		Predicate: "extends",
		Object:    "PhaseNet",
	})
	if err != nil {
		t.Fatalf("add triple error: %v", err)
	}
	if !tripleResp.Success {
		t.Error("expected success")
	}

	// Query graph
	queryResp, err := s.QueryGraph(ctx, &memoryv1.GraphQueryRequest{
		Entity:  "PhaseNet-TF",
		MaxHops: 2,
	})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(queryResp.Nodes) == 0 {
		t.Error("expected nodes")
	}
	if len(queryResp.Edges) == 0 {
		t.Error("expected edges")
	}
}

func TestAddGraphTripleMissingFields(t *testing.T) {
	s := newTestServer()
	_, err := s.AddGraphTriple(context.Background(), &memoryv1.GraphTripleRequest{
		Subject: "A",
		// Missing predicate and object
	})
	if err == nil {
		t.Error("expected error for missing fields")
	}
}

func TestDeleteDocument(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	// Index first
	s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId: "doc-del",
		Content:    "Some test content to be deleted later from the system",
	})

	// Delete
	delResp, err := s.DeleteDocument(ctx, &memoryv1.DeleteRequest{
		DocumentId: "doc-del",
	})
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}
	if !delResp.Success {
		t.Error("expected success")
	}
	if delResp.ChunksDeleted == 0 {
		t.Error("expected chunks to be deleted")
	}
}

func TestGetStats(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	// Index a doc
	s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId: "doc-stats",
		Content:    "Content for stats testing in the hippocampus service",
	})

	// Add a triple
	s.AddGraphTriple(ctx, &memoryv1.GraphTripleRequest{
		Subject:   "A",
		Predicate: "links",
		Object:    "B",
	})

	stats, err := s.GetStats(ctx, &memoryv1.StatsRequest{})
	if err != nil {
		t.Fatalf("stats error: %v", err)
	}

	if stats.TotalDocuments != 1 {
		t.Errorf("expected 1 document, got %d", stats.TotalDocuments)
	}
	if stats.TotalChunks == 0 {
		t.Error("expected chunks > 0")
	}
	if stats.TotalGraphTriples != 1 {
		t.Errorf("expected 1 triple, got %d", stats.TotalGraphTriples)
	}
}

func TestFullTextSearch(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	// Index documents
	s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId: "doc-1",
		Content:    "The PhaseNet-TF model extends the original PhaseNet architecture for seismic signal detection using transfer learning techniques.",
		Metadata:   map[string]string{"type": "research"},
	})
	s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId: "doc-2",
		Content:    "Kubernetes deployment patterns for microservices and container orchestration in production environments.",
		Metadata:   map[string]string{"type": "devops"},
	})

	// Full-text search
	resp, err := s.FullTextSearch(ctx, &memoryv1.SearchRequest{
		Query: "seismic detection",
		TopK:  5,
	})
	if err != nil {
		t.Fatalf("full-text search error: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected full-text search results")
	}
	if resp.Results[0].DocumentId != "doc-1" {
		t.Errorf("expected doc-1 first, got %q", resp.Results[0].DocumentId)
	}
}

func TestFullTextSearchEmptyQuery(t *testing.T) {
	s := newTestServer()
	_, err := s.FullTextSearch(context.Background(), &memoryv1.SearchRequest{Query: ""})
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestHybridSearch(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	// Index documents
	s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId: "doc-1",
		Content:    "The PhaseNet-TF model extends the original PhaseNet architecture for seismic signal detection using transfer learning techniques.",
		Metadata:   map[string]string{"type": "research"},
	})
	s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId: "doc-2",
		Content:    "Deep learning for earthquake analysis and seismic wave detection using neural networks.",
		Metadata:   map[string]string{"type": "research"},
	})

	// Hybrid search
	resp, err := s.HybridSearch(ctx, &memoryv1.SearchRequest{
		Query: "seismic detection neural",
		TopK:  5,
	})
	if err != nil {
		t.Fatalf("hybrid search error: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected hybrid search results")
	}
}

func TestHybridSearchEmptyQuery(t *testing.T) {
	s := newTestServer()
	_, err := s.HybridSearch(context.Background(), &memoryv1.SearchRequest{Query: ""})
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestFullTextSearchWithMinScore(t *testing.T) {
	s := newTestServer()
	ctx := context.Background()

	s.IndexDocument(ctx, &memoryv1.IndexRequest{
		DocumentId: "doc-1",
		Content:    "Machine learning algorithms for data processing",
	})

	resp, err := s.FullTextSearch(ctx, &memoryv1.SearchRequest{
		Query:    "machine learning",
		TopK:     5,
		MinScore: 0.99, // Very high threshold
	})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	// With only one doc, top score is 1.0 so it should pass
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result above threshold, got %d", len(resp.Results))
	}
}
