package server

import (
	"context"
	"io"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/agents"
	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/config"
	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/reasoning"
	agentv1 "github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/common/v1"
)

// FrontalLobeServer implements the ReasoningEngine gRPC service.
type FrontalLobeServer struct {
	agentv1.UnimplementedReasoningEngineServer
	commonv1.UnimplementedHealthServiceServer

	logger       *slog.Logger
	cfg          *config.Config
	llm          reasoning.LLMProvider
	clarifyAgent *agents.ClarifyAgent
	reflectAgent *agents.ReflectAgent
	version      string
}

// NewFrontalLobeServer creates a new FrontalLobeServer.
func NewFrontalLobeServer(
	logger *slog.Logger,
	cfg *config.Config,
	llm reasoning.LLMProvider,
) *FrontalLobeServer {
	return &FrontalLobeServer{
		logger:       logger,
		cfg:          cfg,
		llm:          llm,
		clarifyAgent: agents.NewClarifyAgent(llm),
		reflectAgent: agents.NewReflectAgent(llm),
		version:      "0.1.0",
	}
}

// Check implements the HealthService Check RPC.
func (s *FrontalLobeServer) Check(ctx context.Context, req *commonv1.HealthCheckRequest) (*commonv1.HealthCheckResponse, error) {
	return &commonv1.HealthCheckResponse{
		Status:    commonv1.HealthCheckResponse_SERVING,
		Version:   s.version,
		Timestamp: timestamppb.Now(),
	}, nil
}

// StreamThoughtProcess implements the bidirectional streaming reasoning RPC.
func (s *FrontalLobeServer) StreamThoughtProcess(stream agentv1.ReasoningEngine_StreamThoughtProcessServer) error {
	for {
		input, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		sessionID := input.GetSessionId()
		s.logger.Info("processing thought", "session_id", sessionID)

		// Send thinking status
		if err := stream.Send(&agentv1.AgentOutput{
			SessionId: sessionID,
			Timestamp: timestamppb.Now(),
			OutputType: &agentv1.AgentOutput_Status{
				Status: &agentv1.StatusUpdate{
					StatusMessage: "Thinking...",
					Progress:      0.3,
				},
			},
		}); err != nil {
			return err
		}

		// Process based on input type
		if query := input.GetUserQuery(); query != "" {
			// Send thought chain
			if err := stream.Send(&agentv1.AgentOutput{
				SessionId: sessionID,
				Timestamp: timestamppb.Now(),
				OutputType: &agentv1.AgentOutput_ThoughtChain{
					ThoughtChain: "Analyzing the query and retrieving relevant context...",
				},
			}); err != nil {
				return err
			}

			// Build prompt from context
			prompt := s.buildPrompt(query, input.GetContext())

			// Generate response
			response, err := s.llm.Generate(stream.Context(), prompt)
			if err != nil {
				return stream.Send(&agentv1.AgentOutput{
					SessionId: sessionID,
					Timestamp: timestamppb.Now(),
					OutputType: &agentv1.AgentOutput_FinalResponse{
						FinalResponse: "I encountered an error while processing your request.",
					},
				})
			}

			// Send final response
			if err := stream.Send(&agentv1.AgentOutput{
				SessionId: sessionID,
				Timestamp: timestamppb.Now(),
				OutputType: &agentv1.AgentOutput_FinalResponse{
					FinalResponse: response,
				},
			}); err != nil {
				return err
			}
		}

		// Handle tool results
		if toolResult := input.GetToolResult(); toolResult != nil {
			s.logger.Info("received tool result",
				"call_id", toolResult.GetCallId(),
				"is_error", toolResult.GetIsError(),
			)

			if err := stream.Send(&agentv1.AgentOutput{
				SessionId: sessionID,
				Timestamp: timestamppb.Now(),
				OutputType: &agentv1.AgentOutput_ThoughtChain{
					ThoughtChain: "Processing tool result...",
				},
			}); err != nil {
				return err
			}
		}
	}
}

// ClassifyItem classifies an inbox item.
func (s *FrontalLobeServer) ClassifyItem(ctx context.Context, req *agentv1.ClassifyRequest) (*agentv1.ClassifyResponse, error) {
	result, err := s.clarifyAgent.Process(ctx, req.GetContent(), req.GetSource(), req.GetMetadata())
	if err != nil {
		return nil, err
	}

	classMap := map[string]agentv1.ClassifyResponse_Classification{
		"ACTIONABLE": agentv1.ClassifyResponse_ACTIONABLE,
		"REFERENCE":  agentv1.ClassifyResponse_REFERENCE,
		"TRASH":      agentv1.ClassifyResponse_TRASH,
	}
	classification := classMap[result.Classification]

	return &agentv1.ClassifyResponse{
		Classification:    classification,
		SuggestedProject:  result.SuggestedProject,
		SuggestedArea:     result.SuggestedArea,
		Priority:          result.Priority,
		ExtractedMetadata: result.ExtractedMetadata,
		Confidence:        float32(result.Confidence),
	}, nil
}

// GenerateWeeklyReview generates a weekly review report.
func (s *FrontalLobeServer) GenerateWeeklyReview(ctx context.Context, req *agentv1.WeeklyReviewRequest) (*agentv1.WeeklyReviewResponse, error) {
	startDate := time.Now().AddDate(0, 0, -7)
	endDate := time.Now()

	if req.GetStartDate() != nil {
		startDate = req.GetStartDate().AsTime()
	}
	if req.GetEndDate() != nil {
		endDate = req.GetEndDate().AsTime()
	}

	result, err := s.reflectAgent.GenerateWeeklyReview(
		ctx, startDate, endDate,
		req.GetCompletedTasks(), req.GetActiveTasks(), req.GetBlockedTasks(),
	)
	if err != nil {
		return nil, err
	}

	return &agentv1.WeeklyReviewResponse{
		ReportMarkdown:       result.ReportMarkdown,
		StalledProjects:      result.StalledProjects,
		SuggestedNextActions: result.SuggestedNextActions,
		DormantIdeas:         result.DormantIdeas,
	}, nil
}

func (s *FrontalLobeServer) buildPrompt(query string, ctx *agentv1.ContextSnapshot) string {
	var prompt string

	if ctx != nil && ctx.GetSystemPrompt() != "" {
		prompt = ctx.GetSystemPrompt() + "\n\n"
	} else {
		prompt = "You are an expert cognitive assistant helping manage a Second Brain knowledge system.\n\n"
	}

	// Add episodic memory
	if ctx != nil && len(ctx.GetEpisodicMemory()) > 0 {
		prompt += "Recent conversation:\n"
		for _, mem := range ctx.GetEpisodicMemory() {
			prompt += "- " + mem + "\n"
		}
		prompt += "\n"
	}

	// Add semantic memory
	if ctx != nil && len(ctx.GetSemanticMemory()) > 0 {
		prompt += "Relevant context:\n"
		for _, chunk := range ctx.GetSemanticMemory() {
			prompt += "- " + chunk.GetContent() + "\n"
		}
		prompt += "\n"
	}

	// Add graph context
	if ctx != nil && len(ctx.GetGraphContext()) > 0 {
		prompt += "Knowledge graph context:\n"
		for _, triple := range ctx.GetGraphContext() {
			prompt += "- " + triple.GetSubject() + " → " + triple.GetPredicate() + " → " + triple.GetObject() + "\n"
		}
		prompt += "\n"
	}

	prompt += "User query: " + query

	return prompt
}
