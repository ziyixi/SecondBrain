package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/chunker"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/config"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/embedder"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/graph"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/hybrid"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/textindex"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/vectorstore"
	commonv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/common/v1"
	memoryv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/memory/v1"
)

// HippocampusServer implements the MemoryService gRPC service.
type HippocampusServer struct {
	memoryv1.UnimplementedMemoryServiceServer
	commonv1.UnimplementedHealthServiceServer

	logger      *slog.Logger
	cfg         *config.Config
	store       vectorstore.Store
	embedder    embedder.Embedder
	kg          *graph.KnowledgeGraph
	textIdx     *textindex.Index
	docChunks   map[string][]string // document_id -> chunk_ids
	mu          sync.RWMutex
	lastIndexed time.Time
	version     string
}

// NewHippocampusServer creates a new HippocampusServer.
func NewHippocampusServer(
	logger *slog.Logger,
	cfg *config.Config,
	store vectorstore.Store,
	emb embedder.Embedder,
) *HippocampusServer {
	return &HippocampusServer{
		logger:    logger,
		cfg:       cfg,
		store:     store,
		embedder:  emb,
		kg:        graph.New(),
		textIdx:   textindex.New(),
		docChunks: make(map[string][]string),
		version:   "0.1.0",
	}
}

// Check implements the HealthService Check RPC.
func (s *HippocampusServer) Check(ctx context.Context, req *commonv1.HealthCheckRequest) (*commonv1.HealthCheckResponse, error) {
	return &commonv1.HealthCheckResponse{
		Status:    commonv1.HealthCheckResponse_SERVING,
		Version:   s.version,
		Timestamp: timestamppb.Now(),
	}, nil
}

// IndexDocument indexes a document into the vector store.
func (s *HippocampusServer) IndexDocument(ctx context.Context, req *memoryv1.IndexRequest) (*memoryv1.IndexResponse, error) {
	docID := req.GetDocumentId()
	if docID == "" {
		docID = uuid.New().String()
	}

	content := req.GetContent()
	if content == "" {
		return indexError(docID, "content is empty"), nil
	}

	// Chunk the document
	chunks := s.chunkDocument(docID, content, req.GetChunkingStrategy(), req.GetMetadata())
	if len(chunks) == 0 {
		return indexError(docID, "no chunks generated"), nil
	}

	// Generate embeddings
	embeddings, err := s.embedChunks(chunks)
	if err != nil {
		return indexError(docID, fmt.Sprintf("embedding error: %v", err)), nil
	}

	// Store vectors
	chunkIDs, err := s.storeChunkVectors(docID, chunks, embeddings)
	if err != nil {
		return indexError(docID, fmt.Sprintf("vector store error: %v", err)), nil
	}

	s.mu.Lock()
	s.docChunks[docID] = chunkIDs
	s.lastIndexed = time.Now()
	s.mu.Unlock()

	// Also index for full-text search
	s.textIdx.Add(s.cfg.CollectionName, textindex.Document{
		ID:       docID,
		Content:  content,
		Metadata: req.GetMetadata(),
	})

	s.logger.Info("indexed document", "document_id", docID, "chunks", len(chunks))

	return &memoryv1.IndexResponse{
		DocumentId:    docID,
		ChunksCreated: int32(len(chunks)),
		Success:       true,
	}, nil
}

// chunkDocument splits document content using the requested chunking strategy.
func (s *HippocampusServer) chunkDocument(docID, content string, strategy memoryv1.ChunkingStrategy, reqMetadata map[string]string) []chunker.Chunk {
	strategyMap := map[memoryv1.ChunkingStrategy]string{
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_UNSPECIFIED:  "fixed",
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_FIXED:        "fixed",
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_SEMANTIC:     "semantic",
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_HIERARCHICAL: "hierarchical",
	}
	strat := chunker.NewStrategy(strategyMap[strategy], s.cfg.ChunkSize, s.cfg.ChunkOverlap)

	metadata := make(map[string]string)
	for k, v := range reqMetadata {
		metadata[k] = v
	}
	metadata["document_id"] = docID

	return strat.Chunk(docID, content, metadata)
}

// embedChunks generates embeddings for a list of chunks.
func (s *HippocampusServer) embedChunks(chunks []chunker.Chunk) ([][]float32, error) {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	return s.embedder.Embed(texts)
}

// storeChunkVectors writes chunk embeddings into the vector store and returns chunk IDs.
func (s *HippocampusServer) storeChunkVectors(docID string, chunks []chunker.Chunk, embeddings [][]float32) ([]string, error) {
	records := make([]vectorstore.Record, len(chunks))
	chunkIDs := make([]string, len(chunks))

	for i, c := range chunks {
		payload := make(map[string]string)
		for k, v := range c.Metadata {
			payload[k] = v
		}
		payload["content"] = c.Content
		payload["document_id"] = docID

		records[i] = vectorstore.Record{
			ID:      c.ID,
			Vector:  embeddings[i],
			Payload: payload,
		}
		chunkIDs[i] = c.ID
	}

	if err := s.store.Upsert(s.cfg.CollectionName, records); err != nil {
		return nil, err
	}
	return chunkIDs, nil
}

// indexError builds a failed IndexResponse with the given error message.
func indexError(docID, message string) *memoryv1.IndexResponse {
	return &memoryv1.IndexResponse{
		DocumentId:   docID,
		Success:      false,
		ErrorMessage: message,
	}
}

// SemanticSearch searches for semantically similar content.
func (s *HippocampusServer) SemanticSearch(ctx context.Context, req *memoryv1.SearchRequest) (*memoryv1.SearchResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	embeddings, err := s.embedder.Embed([]string{req.GetQuery()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "embedding error: %v", err)
	}

	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = 5
	}

	var filters map[string]string
	if len(req.GetFilters()) > 0 {
		filters = make(map[string]string)
		for k, v := range req.GetFilters() {
			filters[k] = v
		}
	}

	hits, err := s.store.Search(s.cfg.CollectionName, embeddings[0], topK, filters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search error: %v", err)
	}

	// Filter by min score
	var results []*memoryv1.SearchResult
	for _, hit := range hits {
		if req.GetMinScore() > 0 && hit.Score < req.GetMinScore() {
			continue
		}
		results = append(results, &memoryv1.SearchResult{
			ChunkId:    hit.ID,
			DocumentId: hit.Payload["document_id"],
			Content:    hit.Payload["content"],
			Score:      hit.Score,
			Metadata:   hit.Payload,
		})
	}

	return &memoryv1.SearchResponse{Results: results}, nil
}

// AddGraphTriple adds a triple to the knowledge graph.
func (s *HippocampusServer) AddGraphTriple(ctx context.Context, req *memoryv1.GraphTripleRequest) (*memoryv1.GraphTripleResponse, error) {
	if req.GetSubject() == "" || req.GetPredicate() == "" || req.GetObject() == "" {
		return nil, status.Error(codes.InvalidArgument, "subject, predicate, and object are required")
	}

	meta := make(map[string]string)
	for k, v := range req.GetMetadata() {
		meta[k] = v
	}

	tripleID := s.kg.AddTriple(graph.Triple{
		Subject:   req.GetSubject(),
		Predicate: req.GetPredicate(),
		Object:    req.GetObject(),
		Metadata:  meta,
	})

	return &memoryv1.GraphTripleResponse{
		Success:  true,
		TripleId: tripleID,
	}, nil
}

// QueryGraph queries the knowledge graph.
func (s *HippocampusServer) QueryGraph(ctx context.Context, req *memoryv1.GraphQueryRequest) (*memoryv1.GraphQueryResponse, error) {
	if req.GetEntity() == "" {
		return nil, status.Error(codes.InvalidArgument, "entity is required")
	}

	maxHops := int(req.GetMaxHops())
	if maxHops <= 0 {
		maxHops = 2
	}

	nodes, edges := s.kg.Query(req.GetEntity(), maxHops, req.GetRelationshipFilter())

	pbNodes := make([]*memoryv1.GraphNode, len(nodes))
	for i, n := range nodes {
		pbNodes[i] = &memoryv1.GraphNode{
			Id:         n.ID,
			Label:      n.Label,
			Properties: n.Properties,
		}
	}

	pbEdges := make([]*memoryv1.GraphEdge, len(edges))
	for i, e := range edges {
		pbEdges[i] = &memoryv1.GraphEdge{
			Source:       e.Source,
			Target:       e.Target,
			Relationship: e.Relationship,
			Properties:   e.Properties,
		}
	}

	return &memoryv1.GraphQueryResponse{
		Nodes: pbNodes,
		Edges: pbEdges,
	}, nil
}

// DeleteDocument removes a document from the vector store.
func (s *HippocampusServer) DeleteDocument(ctx context.Context, req *memoryv1.DeleteRequest) (*memoryv1.DeleteResponse, error) {
	s.mu.Lock()
	chunkIDs := s.docChunks[req.GetDocumentId()]
	delete(s.docChunks, req.GetDocumentId())
	s.mu.Unlock()

	deleted := 0
	if len(chunkIDs) > 0 {
		n, err := s.store.Delete(s.cfg.CollectionName, chunkIDs)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "delete error: %v", err)
		}
		deleted = n
	}

	// Also remove from text index
	s.textIdx.Delete(s.cfg.CollectionName, req.GetDocumentId())

	return &memoryv1.DeleteResponse{
		Success:       true,
		ChunksDeleted: int32(deleted),
	}, nil
}

// FullTextSearch performs BM25-ranked full-text search.
// Inspired by qmd's BM25 search via FTS5.
func (s *HippocampusServer) FullTextSearch(ctx context.Context, req *memoryv1.SearchRequest) (*memoryv1.SearchResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = 5
	}

	var filters map[string]string
	if len(req.GetFilters()) > 0 {
		filters = make(map[string]string)
		for k, v := range req.GetFilters() {
			filters[k] = v
		}
	}

	hits := s.textIdx.Search(s.cfg.CollectionName, req.GetQuery(), topK, filters)

	var results []*memoryv1.SearchResult
	for _, hit := range hits {
		if req.GetMinScore() > 0 && float32(hit.Score) < req.GetMinScore() {
			continue
		}
		results = append(results, &memoryv1.SearchResult{
			DocumentId: hit.ID,
			Content:    hit.Content,
			Score:      float32(hit.Score),
			Metadata:   hit.Metadata,
		})
	}

	return &memoryv1.SearchResponse{Results: results}, nil
}

// HybridSearch combines BM25 full-text and vector semantic search
// using Reciprocal Rank Fusion, inspired by qmd's hybrid query pipeline.
func (s *HippocampusServer) HybridSearch(ctx context.Context, req *memoryv1.SearchRequest) (*memoryv1.SearchResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = 5
	}

	var filters map[string]string
	if len(req.GetFilters()) > 0 {
		filters = make(map[string]string)
		for k, v := range req.GetFilters() {
			filters[k] = v
		}
	}

	// BM25 full-text search
	ftsHits := s.textIdx.Search(s.cfg.CollectionName, req.GetQuery(), topK*2, filters)
	var ftsList []hybrid.RankedResult
	for _, h := range ftsHits {
		ftsList = append(ftsList, hybrid.RankedResult{
			ID: h.ID, Score: h.Score, Content: h.Content, Metadata: h.Metadata,
		})
	}

	// Vector semantic search
	embeddings, err := s.embedder.Embed([]string{req.GetQuery()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "embedding error: %v", err)
	}

	vecHits, err := s.store.Search(s.cfg.CollectionName, embeddings[0], topK*2, filters)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "vector search error: %v", err)
	}

	var vecList []hybrid.RankedResult
	for _, h := range vecHits {
		vecList = append(vecList, hybrid.RankedResult{
			ID:       h.Payload["document_id"],
			Score:    float64(h.Score),
			Content:  h.Payload["content"],
			Metadata: h.Payload,
		})
	}

	// Reciprocal Rank Fusion with BM25 weighted 2x (original query emphasis)
	rankedLists := [][]hybrid.RankedResult{ftsList, vecList}
	weights := []float64{2.0, 1.0}
	fused := hybrid.ReciprocalRankFusion(rankedLists, weights, 60)

	// Normalize and truncate
	fused = hybrid.NormalizeScores(fused)
	if len(fused) > topK {
		fused = fused[:topK]
	}

	var results []*memoryv1.SearchResult
	for _, r := range fused {
		if req.GetMinScore() > 0 && float32(r.Score) < req.GetMinScore() {
			continue
		}
		results = append(results, &memoryv1.SearchResult{
			DocumentId: r.ID,
			Content:    r.Content,
			Score:      float32(r.Score),
			Metadata:   r.Metadata,
		})
	}

	return &memoryv1.SearchResponse{Results: results}, nil
}

// GetStats returns indexing statistics.
func (s *HippocampusServer) GetStats(ctx context.Context, req *memoryv1.StatsRequest) (*memoryv1.StatsResponse, error) {
	s.mu.RLock()
	docCount := len(s.docChunks)
	lastIndexed := s.lastIndexed
	s.mu.RUnlock()

	chunkCount := s.store.Count(s.cfg.CollectionName)
	tripleCount := s.kg.TriplesCount()

	resp := &memoryv1.StatsResponse{
		TotalDocuments:   int64(docCount),
		TotalChunks:      int64(chunkCount),
		TotalGraphTriples: int64(tripleCount),
	}

	if !lastIndexed.IsZero() {
		resp.LastIndexedAt = timestamppb.New(lastIndexed)
	}

	return resp, nil
}
