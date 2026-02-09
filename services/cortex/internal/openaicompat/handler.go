package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	agentv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Handler serves the OpenAI-compatible HTTP API.
type Handler struct {
	logger       *slog.Logger
	models       []string
	frontalAddr  string
	frontalConn  *grpc.ClientConn
	frontalClient agentv1.ReasoningEngineClient
}

// NewHandler creates a new OpenAI-compatible API handler.
func NewHandler(logger *slog.Logger, models []string) *Handler {
	return &Handler{
		logger: logger,
		models: models,
	}
}

// ConnectFrontalLobe sets up the gRPC connection to the frontal lobe.
func (h *Handler) ConnectFrontalLobe(addr string) error {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connecting to frontal lobe: %w", err)
	}
	h.frontalAddr = addr
	h.frontalConn = conn
	h.frontalClient = agentv1.NewReasoningEngineClient(conn)
	return nil
}

// Close cleans up resources.
func (h *Handler) Close() {
	if h.frontalConn != nil {
		h.frontalConn.Close()
	}
}

// RegisterRoutes registers the OpenAI-compatible API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/chat/completions", h.handleChatCompletions)
	mux.HandleFunc("GET /v1/models", h.handleListModels)
}

func (h *Handler) handleListModels(w http.ResponseWriter, r *http.Request) {
	models := make([]Model, 0, len(h.models))
	for _, m := range h.models {
		models = append(models, Model{
			ID:      m,
			Object:  "model",
			Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
			OwnedBy: "secondbrain",
		})
	}

	resp := ModelList{
		Object: "list",
		Data:   models,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		h.writeError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	if req.Stream {
		h.handleStreamingCompletion(w, r, &req)
		return
	}

	h.handleNonStreamingCompletion(w, r, &req)
}

func (h *Handler) handleNonStreamingCompletion(w http.ResponseWriter, r *http.Request, req *ChatCompletionRequest) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Build session and query from messages
	sessionID := req.User
	if sessionID == "" {
		sessionID = fmt.Sprintf("openai-compat-%d", time.Now().UnixNano())
	}

	query, systemPrompt := extractQueryAndSystem(req.Messages)

	// Call the reasoning engine via gRPC streaming
	response, err := h.callReasoningEngine(ctx, sessionID, query, systemPrompt, req.Model)
	if err != nil {
		h.logger.Error("reasoning engine call failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "server_error", "Internal server error")
		return
	}

	chatResp := NewChatCompletionResponse(
		fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		req.Model,
		response,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResp)
}

func (h *Handler) handleStreamingCompletion(w http.ResponseWriter, r *http.Request, req *ChatCompletionRequest) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	sessionID := req.User
	if sessionID == "" {
		sessionID = fmt.Sprintf("openai-compat-%d", time.Now().UnixNano())
	}

	query, systemPrompt := extractQueryAndSystem(req.Messages)
	completionID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "server_error", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send role chunk first
	roleChunk := &ChatCompletionChunk{
		ID:      completionID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []ChatChunkChoice{
			{Index: 0, Delta: ChatDelta{Role: "assistant"}},
		},
	}
	h.writeSSE(w, roleChunk)
	flusher.Flush()

	// Stream from reasoning engine
	chunks, err := h.streamReasoningEngine(ctx, sessionID, query, systemPrompt, req.Model)
	if err != nil {
		h.logger.Error("streaming reasoning engine failed", "error", err)
		return
	}

	for content := range chunks {
		chunk := NewStreamChunk(completionID, req.Model, content, false)
		h.writeSSE(w, chunk)
		flusher.Flush()
	}

	// Send final chunk
	finishChunk := NewStreamChunk(completionID, req.Model, "", true)
	h.writeSSE(w, finishChunk)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (h *Handler) callReasoningEngine(ctx context.Context, sessionID, query, systemPrompt, model string) (string, error) {
	if h.frontalClient == nil {
		// Fallback: echo response
		return fmt.Sprintf("Echo: %s (model: %s, no reasoning engine connected)", query, model), nil
	}

	stream, err := h.frontalClient.StreamThoughtProcess(ctx)
	if err != nil {
		return "", fmt.Errorf("opening stream: %w", err)
	}

	input := &agentv1.AgentInput{
		SessionId: sessionID,
		InputType: &agentv1.AgentInput_UserQuery{UserQuery: query},
		Context: &agentv1.ContextSnapshot{
			SystemPrompt: systemPrompt,
		},
	}

	if err := stream.Send(input); err != nil {
		return "", fmt.Errorf("sending input: %w", err)
	}
	stream.CloseSend()

	var finalResponse string
	for {
		output, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("receiving output: %w", err)
		}

		if resp := output.GetFinalResponse(); resp != "" {
			finalResponse = resp
		}
	}

	if finalResponse == "" {
		finalResponse = "No response generated."
	}
	return finalResponse, nil
}

func (h *Handler) streamReasoningEngine(ctx context.Context, sessionID, query, systemPrompt, model string) (<-chan string, error) {
	ch := make(chan string, 10)

	if h.frontalClient == nil {
		go func() {
			defer close(ch)
			ch <- fmt.Sprintf("Echo: %s (model: %s, no reasoning engine connected)", query, model)
		}()
		return ch, nil
	}

	stream, err := h.frontalClient.StreamThoughtProcess(ctx)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("opening stream: %w", err)
	}

	input := &agentv1.AgentInput{
		SessionId: sessionID,
		InputType: &agentv1.AgentInput_UserQuery{UserQuery: query},
		Context: &agentv1.ContextSnapshot{
			SystemPrompt: systemPrompt,
		},
	}

	if err := stream.Send(input); err != nil {
		close(ch)
		return nil, fmt.Errorf("sending input: %w", err)
	}
	stream.CloseSend()

	go func() {
		defer close(ch)
		for {
			output, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				h.logger.Error("stream recv error", "error", err)
				return
			}

			if thought := output.GetThoughtChain(); thought != "" {
				ch <- thought + "\n"
			}
			if resp := output.GetFinalResponse(); resp != "" {
				ch <- resp
			}
		}
	}()

	return ch, nil
}

func (h *Handler) writeSSE(w http.ResponseWriter, data interface{}) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Message: message,
			Type:    errType,
			Code:    fmt.Sprintf("%d", status),
		},
	})
}

// extractQueryAndSystem separates the user query and system prompt from messages.
func extractQueryAndSystem(messages []ChatMessage) (query, systemPrompt string) {
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemPrompt = msg.Content
		case "user":
			query = msg.Content
		}
	}
	// Use the last user message as the query
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			query = messages[i].Content
			break
		}
	}
	return query, systemPrompt
}
