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
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/vectorstore"
	commonv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/common/v1"
	memoryv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/memory/v1"
)

// HippocampusServer implements the MemoryService gRPC service.
type HippocampusServer struct {
	memoryv1.UnimplementedMemoryServiceServer
	commonv1.UnimplementedHealthServiceServer

	logger     *slog.Logger
	cfg        *config.Config
	store      vectorstore.Store
	embedder   embedder.Embedder
	kg         *graph.KnowledgeGraph
	docChunks  map[string][]string // document_id -> chunk_ids
	mu         sync.RWMutex
	lastIndexed time.Time
	version    string
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
		return &memoryv1.IndexResponse{
			DocumentId:    docID,
			ChunksCreated: 0,
			Success:       false,
			ErrorMessage:  "content is empty",
		}, nil
	}

	// Determine chunking strategy
	strategyMap := map[memoryv1.ChunkingStrategy]string{
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_UNSPECIFIED:  "fixed",
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_FIXED:        "fixed",
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_SEMANTIC:     "semantic",
		memoryv1.ChunkingStrategy_CHUNKING_STRATEGY_HIERARCHICAL: "hierarchical",
	}
	strategyName := strategyMap[req.GetChunkingStrategy()]
	strat := chunker.NewStrategy(strategyName, s.cfg.ChunkSize, s.cfg.ChunkOverlap)

	metadata := make(map[string]string)
	for k, v := range req.GetMetadata() {
		metadata[k] = v
	}
	metadata["document_id"] = docID

	chunks := strat.Chunk(docID, content, metadata)
	if len(chunks) == 0 {
		return &memoryv1.IndexResponse{
			DocumentId:    docID,
			ChunksCreated: 0,
			Success:       false,
			ErrorMessage:  "no chunks generated",
		}, nil
	}

	// Generate embeddings
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	embeddings, err := s.embedder.Embed(texts)
	if err != nil {
		return &memoryv1.IndexResponse{
			DocumentId:   docID,
			Success:      false,
			ErrorMessage: fmt.Sprintf("embedding error: %v", err),
		}, nil
	}

	// Store vectors
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
		return &memoryv1.IndexResponse{
			DocumentId:   docID,
			Success:      false,
			ErrorMessage: fmt.Sprintf("vector store error: %v", err),
		}, nil
	}

	s.mu.Lock()
	s.docChunks[docID] = chunkIDs
	s.lastIndexed = time.Now()
	s.mu.Unlock()

	s.logger.Info("indexed document", "document_id", docID, "chunks", len(chunks))

	return &memoryv1.IndexResponse{
		DocumentId:    docID,
		ChunksCreated: int32(len(chunks)),
		Success:       true,
	}, nil
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

	return &memoryv1.DeleteResponse{
		Success:       true,
		ChunksDeleted: int32(deleted),
	}, nil
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
