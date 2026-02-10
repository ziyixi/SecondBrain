package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	agentv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/ingestion/v1"
	memoryv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/memory/v1"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/metrics"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/session"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CortexServer implements the orchestration logic for the Cognitive OS.
// It coordinates between the Frontal Lobe, Hippocampus, and Sensory Gateway.
type CortexServer struct {
	agentv1.UnimplementedReasoningEngineServer
	commonv1.UnimplementedHealthServiceServer
	ingestionv1.UnimplementedIngestionServiceServer

	logger         *slog.Logger
	sessionMgr     *session.Manager
	metricsStore   *metrics.Store
	frontalConn    *grpc.ClientConn
	hippocampusConn *grpc.ClientConn
	frontalClient  agentv1.ReasoningEngineClient
	memoryClient   memoryv1.MemoryServiceClient
	version        string
}

// NewCortexServer creates a new CortexServer instance.
func NewCortexServer(logger *slog.Logger) *CortexServer {
	return &CortexServer{
		logger:       logger,
		sessionMgr:   session.NewManager(),
		metricsStore: metrics.NewStore(),
		version:      "0.1.0",
	}
}

// MetricsStore returns the metrics store for external access (e.g., HTTP API).
func (s *CortexServer) MetricsStore() *metrics.Store {
	return s.metricsStore
}

// MemoryClient returns the memory service client for external access (e.g., MCP server).
func (s *CortexServer) MemoryClient() memoryv1.MemoryServiceClient {
	return s.memoryClient
}

// ConnectDownstream establishes connections to downstream services.
func (s *CortexServer) ConnectDownstream(frontalAddr, hippocampusAddr string) error {
	var err error

	s.frontalConn, err = grpc.NewClient(frontalAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connecting to frontal lobe: %w", err)
	}
	s.frontalClient = agentv1.NewReasoningEngineClient(s.frontalConn)

	s.hippocampusConn, err = grpc.NewClient(hippocampusAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connecting to hippocampus: %w", err)
	}
	s.memoryClient = memoryv1.NewMemoryServiceClient(s.hippocampusConn)

	s.logger.Info("connected to downstream services",
		"frontal_lobe", frontalAddr,
		"hippocampus", hippocampusAddr,
	)

	return nil
}

// Close cleanly shuts down connections.
func (s *CortexServer) Close() {
	if s.frontalConn != nil {
		s.frontalConn.Close()
	}
	if s.hippocampusConn != nil {
		s.hippocampusConn.Close()
	}
}

// Check implements the HealthService Check RPC.
func (s *CortexServer) Check(ctx context.Context, req *commonv1.HealthCheckRequest) (*commonv1.HealthCheckResponse, error) {
	return &commonv1.HealthCheckResponse{
		Status:    commonv1.HealthCheckResponse_SERVING,
		Version:   s.version,
		Timestamp: timestamppb.Now(),
	}, nil
}

// StreamThoughtProcess implements the bidirectional streaming RPC
// between the client and the Frontal Lobe reasoning engine.
func (s *CortexServer) StreamThoughtProcess(stream agentv1.ReasoningEngine_StreamThoughtProcessServer) error {
	// Receive the first message to get the session ID
	firstMsg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("receiving first message: %w", err)
	}

	sessionID := firstMsg.GetSessionId()
	s.logger.Info("starting thought process stream", "session_id", sessionID)

	// Ensure session exists
	sess, exists := s.sessionMgr.Get(sessionID)
	if !exists {
		sess = s.sessionMgr.Create(sessionID, "default-user")
	}

	// Process the first message
	if err := s.processAgentInput(stream, sess, firstMsg); err != nil {
		return err
	}

	// Continue receiving messages
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			s.logger.Info("stream ended", "session_id", sessionID)
			return nil
		}
		if err != nil {
			return fmt.Errorf("receiving message: %w", err)
		}

		if err := s.processAgentInput(stream, sess, msg); err != nil {
			return err
		}
	}
}

func (s *CortexServer) processAgentInput(
	stream agentv1.ReasoningEngine_StreamThoughtProcessServer,
	sess *session.Session,
	input *agentv1.AgentInput,
) error {
	sessionID := input.GetSessionId()

	if err := sendStatus(stream, sessionID, "Processing input...", 0.1); err != nil {
		return fmt.Errorf("sending status: %w", err)
	}

	if query := input.GetUserQuery(); query != "" {
		return s.handleUserQuery(stream, sess, input, sessionID, query)
	}

	if feedback := input.GetUserFeedback(); feedback != nil {
		s.handleFeedback(sessionID, feedback)
	}

	return nil
}

// handleUserQuery enriches the query with context from Hippocampus, records
// metrics, and forwards to the Frontal Lobe for reasoning.
func (s *CortexServer) handleUserQuery(
	stream agentv1.ReasoningEngine_StreamThoughtProcessServer,
	sess *session.Session,
	input *agentv1.AgentInput,
	sessionID, query string,
) error {
	sess.AddEpisodicMemory("User: " + query)

	ctx := input.GetContext()
	if ctx == nil {
		ctx = &agentv1.ContextSnapshot{}
	}

	contextRelevance := s.enrichContextFromMemory(stream.Context(), ctx, query)
	ctx.EpisodicMemory = sess.GetEpisodicMemory()
	input.Context = ctx

	s.metricsStore.Record(metrics.InteractionRecord{
		SessionID:        sessionID,
		Timestamp:        time.Now(),
		Query:            query,
		ContextRelevance: contextRelevance,
		ResponseQuality:  contextRelevance, // initial estimate from context quality
	})

	if s.frontalClient != nil {
		return s.forwardToFrontalLobe(stream, input)
	}

	return sendFinalResponse(stream, sessionID,
		fmt.Sprintf("Received query: %s (Frontal Lobe not connected)", query))
}

// enrichContextFromMemory searches Hippocampus for relevant content using
// hybrid search (BM25 + vector with RRF) and appends matches to the context
// snapshot. Falls back to semantic-only search when hybrid is unavailable.
// Returns the average relevance score across results (0 if no results).
func (s *CortexServer) enrichContextFromMemory(
	reqCtx context.Context,
	snapshot *agentv1.ContextSnapshot,
	query string,
) float64 {
	if s.memoryClient == nil {
		return 0
	}

	searchReq := &memoryv1.SearchRequest{
		Query: query,
		TopK:  5,
	}

	// Try hybrid search first, fall back to semantic-only
	searchResp, err := s.memoryClient.HybridSearch(reqCtx, searchReq)
	if err != nil {
		s.logger.Debug("hybrid search unavailable, falling back to semantic", "error", err)
		searchResp, err = s.memoryClient.SemanticSearch(reqCtx, searchReq)
		if err != nil {
			s.logger.Warn("failed to search memory", "error", err)
			return 0
		}
	}

	var totalScore float64
	for _, result := range searchResp.GetResults() {
		snapshot.SemanticMemory = append(snapshot.SemanticMemory, &agentv1.SemanticChunk{
			ChunkId:        result.GetChunkId(),
			Content:        result.GetContent(),
			RelevanceScore: result.GetScore(),
			Metadata:       result.GetMetadata(),
		})
		totalScore += float64(result.GetScore())
	}

	if n := len(searchResp.GetResults()); n > 0 {
		return totalScore / float64(n)
	}
	return 0
}

// handleFeedback records a user feedback signal in the metrics store.
func (s *CortexServer) handleFeedback(sessionID string, feedback *agentv1.FeedbackSignal) {
	var feedbackType metrics.FeedbackType
	switch feedback.GetSentiment() {
	case agentv1.FeedbackSignal_POSITIVE:
		feedbackType = metrics.FeedbackPositive
	case agentv1.FeedbackSignal_NEGATIVE:
		feedbackType = metrics.FeedbackNegative
	case agentv1.FeedbackSignal_CORRECTION:
		feedbackType = metrics.FeedbackCorrection
	}
	s.metricsStore.Record(metrics.InteractionRecord{
		SessionID: sessionID,
		Timestamp: time.Now(),
		Feedback:  feedbackType,
	})
}

// --- Stream output helpers ---

// sendStatus sends a progress status update to the client stream.
func sendStatus(stream agentv1.ReasoningEngine_StreamThoughtProcessServer, sessionID, message string, progress float32) error {
	return stream.Send(&agentv1.AgentOutput{
		SessionId: sessionID,
		Timestamp: timestamppb.Now(),
		OutputType: &agentv1.AgentOutput_Status{
			Status: &agentv1.StatusUpdate{
				StatusMessage: message,
				Progress:      progress,
			},
		},
	})
}

// sendFinalResponse sends a final response to the client stream.
func sendFinalResponse(stream agentv1.ReasoningEngine_StreamThoughtProcessServer, sessionID, response string) error {
	return stream.Send(&agentv1.AgentOutput{
		SessionId: sessionID,
		Timestamp: timestamppb.Now(),
		OutputType: &agentv1.AgentOutput_FinalResponse{
			FinalResponse: response,
		},
	})
}

func (s *CortexServer) forwardToFrontalLobe(
	clientStream agentv1.ReasoningEngine_StreamThoughtProcessServer,
	input *agentv1.AgentInput,
) error {
	ctx, cancel := context.WithTimeout(clientStream.Context(), 5*time.Minute)
	defer cancel()

	frontalStream, err := s.frontalClient.StreamThoughtProcess(ctx)
	if err != nil {
		return fmt.Errorf("connecting to frontal lobe stream: %w", err)
	}

	// Send input to frontal lobe
	if err := frontalStream.Send(input); err != nil {
		return fmt.Errorf("sending to frontal lobe: %w", err)
	}
	frontalStream.CloseSend()

	// Relay responses back to client
	for {
		output, err := frontalStream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("receiving from frontal lobe: %w", err)
		}

		if err := clientStream.Send(output); err != nil {
			return fmt.Errorf("relaying to client: %w", err)
		}
	}
}

// ClassifyItem implements the unary classification RPC.
func (s *CortexServer) ClassifyItem(ctx context.Context, req *agentv1.ClassifyRequest) (*agentv1.ClassifyResponse, error) {
	if s.frontalClient != nil {
		return s.frontalClient.ClassifyItem(ctx, req)
	}
	return &agentv1.ClassifyResponse{
		Classification: agentv1.ClassifyResponse_REFERENCE,
		Confidence:     0.0,
	}, nil
}

// GenerateWeeklyReview implements the weekly review generation RPC.
func (s *CortexServer) GenerateWeeklyReview(ctx context.Context, req *agentv1.WeeklyReviewRequest) (*agentv1.WeeklyReviewResponse, error) {
	if s.frontalClient != nil {
		return s.frontalClient.GenerateWeeklyReview(ctx, req)
	}
	return &agentv1.WeeklyReviewResponse{
		ReportMarkdown: "Weekly review generation requires the Frontal Lobe service.",
	}, nil
}

// IngestItem implements the IngestionService IngestItem RPC (proxy).
func (s *CortexServer) IngestItem(ctx context.Context, req *ingestionv1.IngestRequest) (*ingestionv1.IngestResponse, error) {
	item := req.GetItem()
	s.logger.Info("ingesting item", "id", item.GetId(), "source", item.GetSource())

	// Index in Hippocampus for semantic search
	if s.memoryClient != nil && item.GetContent() != "" {
		_, err := s.memoryClient.IndexDocument(ctx, &memoryv1.IndexRequest{
			DocumentId: item.GetId(),
			Content:    item.GetContent(),
			Metadata: map[string]string{
				"source":     item.GetSource(),
				"source_id":  item.GetSourceId(),
				"content_type": item.GetContentType(),
			},
		})
		if err != nil {
			s.logger.Warn("failed to index document", "error", err)
		}
	}

	return &ingestionv1.IngestResponse{
		ItemId:   item.GetId(),
		Accepted: true,
		Message:  "Item accepted for processing",
		Status:   commonv1.ProcessingStatus_PROCESSING_STATUS_ANALYZING,
	}, nil
}
