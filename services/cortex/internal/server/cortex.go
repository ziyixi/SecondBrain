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
	frontalConn    *grpc.ClientConn
	hippocampusConn *grpc.ClientConn
	frontalClient  agentv1.ReasoningEngineClient
	memoryClient   memoryv1.MemoryServiceClient
	version        string
}

// NewCortexServer creates a new CortexServer instance.
func NewCortexServer(logger *slog.Logger) *CortexServer {
	return &CortexServer{
		logger:     logger,
		sessionMgr: session.NewManager(),
		version:    "0.1.0",
	}
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

	// Send status update
	if err := stream.Send(&agentv1.AgentOutput{
		SessionId: sessionID,
		Timestamp: timestamppb.Now(),
		OutputType: &agentv1.AgentOutput_Status{
			Status: &agentv1.StatusUpdate{
				StatusMessage: "Processing input...",
				Progress:      0.1,
			},
		},
	}); err != nil {
		return fmt.Errorf("sending status: %w", err)
	}

	// If there's a user query, enrich with context from Hippocampus
	if query := input.GetUserQuery(); query != "" {
		sess.AddEpisodicMemory("User: " + query)

		// Search for relevant context in the vector store
		context := input.GetContext()
		if context == nil {
			context = &agentv1.ContextSnapshot{}
		}

		if s.memoryClient != nil {
			searchResp, err := s.memoryClient.SemanticSearch(
				stream.Context(),
				&memoryv1.SearchRequest{
					Query: query,
					TopK:  5,
				},
			)
			if err != nil {
				s.logger.Warn("failed to search memory", "error", err)
			} else {
				for _, result := range searchResp.GetResults() {
					context.SemanticMemory = append(context.SemanticMemory, &agentv1.SemanticChunk{
						ChunkId:        result.GetChunkId(),
						Content:        result.GetContent(),
						RelevanceScore: result.GetScore(),
						Metadata:       result.GetMetadata(),
					})
				}
			}
		}

		// Add episodic memory
		context.EpisodicMemory = sess.GetEpisodicMemory()
		input.Context = context

		// Forward to Frontal Lobe if connected
		if s.frontalClient != nil {
			return s.forwardToFrontalLobe(stream, input)
		}

		// Fallback: echo response if frontal lobe is not connected
		return stream.Send(&agentv1.AgentOutput{
			SessionId: sessionID,
			Timestamp: timestamppb.Now(),
			OutputType: &agentv1.AgentOutput_FinalResponse{
				FinalResponse: fmt.Sprintf("Received query: %s (Frontal Lobe not connected)", query),
			},
		})
	}

	return nil
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
