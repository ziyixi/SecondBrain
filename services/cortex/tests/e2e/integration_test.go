package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	agentv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/common/v1"
	memoryv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/memory/v1"

	"github.com/ziyixi/SecondBrain/services/cortex/internal/metrics"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/openaicompat"
	cortexserver "github.com/ziyixi/SecondBrain/services/cortex/internal/server"
)

// --- Fake LLM API servers ---

// newFakeOpenAIServer creates an httptest server that mimics the OpenAI
// /v1/chat/completions endpoint. It returns increasingly relevant responses
// as the prompt gets richer context (simulating improvement with feedback).
func newFakeOpenAIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Combine all message content to understand context richness
		var fullPrompt string
		for _, m := range req.Messages {
			fullPrompt += m.Content + " "
		}

		// Return response that varies based on context. When there's
		// "Relevant context:" in the prompt, the model knows more and gives
		// a better answer.
		response := fmt.Sprintf("[openai/%s] Processed query with context", req.Model)
		if len(fullPrompt) > 200 {
			response = fmt.Sprintf("[openai/%s] Detailed answer with rich context integration", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-fake-openai",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   req.Model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": response,
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
}

// newFakeGeminiServer creates an httptest server that mimics the Google
// Generative AI generateContent endpoint.
func newFakeGeminiServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		prompt := ""
		for _, c := range req.Contents {
			for _, p := range c.Parts {
				prompt += p.Text
			}
		}

		response := "[gemini] Processed query"
		if len(prompt) > 200 {
			response = "[gemini] Detailed answer with rich context integration"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]string{
							{"text": response},
						},
					},
				},
			},
		})
	}))
}

// --- Test helper: minimal in-process gRPC frontal lobe ---

// fakeFrontalLobe is a minimal gRPC ReasoningEngine that calls an external
// LLM API (our fake servers) and returns the result.
type fakeFrontalLobe struct {
	agentv1.UnimplementedReasoningEngineServer
	commonv1.UnimplementedHealthServiceServer
	llmURL string
	model  string
}

func (f *fakeFrontalLobe) Check(ctx context.Context, req *commonv1.HealthCheckRequest) (*commonv1.HealthCheckResponse, error) {
	return &commonv1.HealthCheckResponse{
		Status:  commonv1.HealthCheckResponse_SERVING,
		Version: "test",
	}, nil
}

func (f *fakeFrontalLobe) StreamThoughtProcess(stream agentv1.ReasoningEngine_StreamThoughtProcessServer) error {
	for {
		input, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		query := input.GetUserQuery()
		if query == "" {
			continue
		}

		// Build a prompt similar to real frontal lobe
		prompt := "You are a cognitive assistant.\n\n"
		if ctx := input.GetContext(); ctx != nil {
			if sp := ctx.GetSystemPrompt(); sp != "" {
				prompt = sp + "\n\n"
			}
			if len(ctx.GetEpisodicMemory()) > 0 {
				prompt += "Recent conversation:\n"
				for _, m := range ctx.GetEpisodicMemory() {
					prompt += "- " + m + "\n"
				}
				prompt += "\n"
			}
			if len(ctx.GetSemanticMemory()) > 0 {
				prompt += "Relevant context:\n"
				for _, chunk := range ctx.GetSemanticMemory() {
					prompt += "- " + chunk.GetContent() + "\n"
				}
				prompt += "\n"
			}
		}
		prompt += "User query: " + query

		// Call the fake LLM API
		response, err := f.callLLM(stream.Context(), prompt)
		if err != nil {
			response = fmt.Sprintf("Error: %v", err)
		}

		if err := stream.Send(&agentv1.AgentOutput{
			SessionId: input.GetSessionId(),
			OutputType: &agentv1.AgentOutput_FinalResponse{
				FinalResponse: response,
			},
		}); err != nil {
			return err
		}
	}
}

func (f *fakeFrontalLobe) callLLM(ctx context.Context, prompt string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": f.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", f.llmURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fake-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices")
	}
	return chatResp.Choices[0].Message.Content, nil
}

func (f *fakeFrontalLobe) ClassifyItem(ctx context.Context, req *agentv1.ClassifyRequest) (*agentv1.ClassifyResponse, error) {
	return &agentv1.ClassifyResponse{
		Classification: agentv1.ClassifyResponse_ACTIONABLE,
		Confidence:     0.9,
	}, nil
}

// --- Fake in-process hippocampus using real implementation ---

// We'll use the real hippocampus server via a gRPC connection, but we need
// the server from the hippocampus module. Since we're in the cortex module,
// we'll create a minimal fake memory service instead.

type fakeMemoryService struct {
	memoryv1.UnimplementedMemoryServiceServer
	commonv1.UnimplementedHealthServiceServer
	docs map[string]string // docID -> content
}

func newFakeMemoryService() *fakeMemoryService {
	return &fakeMemoryService{
		docs: make(map[string]string),
	}
}

func (f *fakeMemoryService) Check(ctx context.Context, req *commonv1.HealthCheckRequest) (*commonv1.HealthCheckResponse, error) {
	return &commonv1.HealthCheckResponse{
		Status:  commonv1.HealthCheckResponse_SERVING,
		Version: "test",
	}, nil
}

func (f *fakeMemoryService) IndexDocument(ctx context.Context, req *memoryv1.IndexRequest) (*memoryv1.IndexResponse, error) {
	f.docs[req.GetDocumentId()] = req.GetContent()
	return &memoryv1.IndexResponse{
		DocumentId:    req.GetDocumentId(),
		ChunksCreated: 1,
		Success:       true,
	}, nil
}

func (f *fakeMemoryService) SemanticSearch(ctx context.Context, req *memoryv1.SearchRequest) (*memoryv1.SearchResponse, error) {
	// Return all indexed documents as search results with simulated scores
	var results []*memoryv1.SearchResult
	for id, content := range f.docs {
		results = append(results, &memoryv1.SearchResult{
			ChunkId:    "chunk-" + id,
			DocumentId: id,
			Content:    content,
			Score:      0.85,
			Metadata:   map[string]string{"document_id": id, "content": content},
		})
	}
	return &memoryv1.SearchResponse{Results: results}, nil
}

// --- Helper to start a gRPC server on a random port ---

func startGRPCServer(t *testing.T, register func(s *grpc.Server)) (addr string, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	register(s)
	go s.Serve(lis)
	return lis.Addr().String(), s.GracefulStop
}

// --- Helper to call the OpenAI-compatible chat completions API ---

func chatCompletion(t *testing.T, baseURL, model, userMsg string) string {
	t.Helper()
	reqBody, _ := json.Marshal(openaicompat.ChatCompletionRequest{
		Model: model,
		Messages: []openaicompat.ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: userMsg},
		},
	})

	resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("chat completion request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var chatResp openaicompat.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(chatResp.Choices) == 0 {
		t.Fatal("no choices in response")
	}
	return chatResp.Choices[0].Message.Content
}

// --- Helper to get metrics from the /v1/metrics endpoint ---

func getMetrics(t *testing.T, baseURL string) metrics.MetricsSummary {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/metrics")
	if err != nil {
		t.Fatalf("metrics request: %v", err)
	}
	defer resp.Body.Close()

	var summary metrics.MetricsSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		t.Fatalf("decoding metrics: %v", err)
	}
	return summary
}

// --- Integration Test ---

// TestIntegrationFeedbackLoop is the full end-to-end integration test.
//
// It spins up:
//   - Fake OpenAI API server (httptest)
//   - Fake Gemini API server (httptest)
//   - Fake Frontal Lobe gRPC server (calling the fake OpenAI API)
//   - Fake Hippocampus gRPC server (in-memory vector store)
//   - Real Cortex gRPC server (connected to the above)
//   - Real OpenAI-compatible HTTP API (connected to Cortex)
//
// Then exercises the full loop:
//  1. Ingest documents into the memory system
//  2. Query the system via the OpenAI-compatible API
//  3. Send positive/negative feedback via gRPC
//  4. Verify metrics (satisfaction rate, quality trend) improve with feedback
//  5. Verify multi-provider routing works (OpenAI + Gemini fake backends)
func TestIntegrationFeedbackLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// --- Step 1: Start fake LLM API servers ---
	fakeOpenAI := newFakeOpenAIServer(t)
	defer fakeOpenAI.Close()

	fakeGemini := newFakeGeminiServer(t)
	defer fakeGemini.Close()

	// --- Step 2: Start fake Hippocampus gRPC server ---
	memService := newFakeMemoryService()
	hippoAddr, hippoStop := startGRPCServer(t, func(s *grpc.Server) {
		memoryv1.RegisterMemoryServiceServer(s, memService)
		commonv1.RegisterHealthServiceServer(s, memService)
	})
	defer hippoStop()

	// --- Step 3: Start fake Frontal Lobe gRPC server (backed by fake OpenAI) ---
	frontalSvc := &fakeFrontalLobe{llmURL: fakeOpenAI.URL, model: "gpt-4-test"}
	frontalAddr, frontalStop := startGRPCServer(t, func(s *grpc.Server) {
		agentv1.RegisterReasoningEngineServer(s, frontalSvc)
		commonv1.RegisterHealthServiceServer(s, frontalSvc)
	})
	defer frontalStop()

	// --- Step 4: Start real Cortex gRPC server ---
	cortex := cortexserver.NewCortexServer(logger)
	if err := cortex.ConnectDownstream(frontalAddr, hippoAddr); err != nil {
		t.Fatalf("connecting downstream: %v", err)
	}
	defer cortex.Close()

	cortexAddr, cortexStop := startGRPCServer(t, func(s *grpc.Server) {
		agentv1.RegisterReasoningEngineServer(s, cortex)
		commonv1.RegisterHealthServiceServer(s, cortex)
	})
	defer cortexStop()

	// --- Step 5: Start OpenAI-compatible HTTP API ---
	openaiHandler := openaicompat.NewHandler(logger, []string{"gpt-4-test", "gemini-pro-test"})
	if err := openaiHandler.ConnectFrontalLobe(cortexAddr); err != nil {
		t.Fatalf("connecting openai handler: %v", err)
	}
	defer openaiHandler.Close()

	httpMux := http.NewServeMux()
	openaiHandler.RegisterRoutes(httpMux)

	// Expose metrics endpoint
	metricsStore := cortex.MetricsStore()
	httpMux.HandleFunc("GET /v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metricsStore.Summary())
	})

	httpServer := httptest.NewServer(httpMux)
	defer httpServer.Close()

	t.Logf("Integration test infrastructure ready:")
	t.Logf("  Fake OpenAI:  %s", fakeOpenAI.URL)
	t.Logf("  Fake Gemini:  %s", fakeGemini.URL)
	t.Logf("  Hippocampus:  %s", hippoAddr)
	t.Logf("  Frontal Lobe: %s", frontalAddr)
	t.Logf("  Cortex gRPC:  %s", cortexAddr)
	t.Logf("  HTTP API:     %s", httpServer.URL)

	// ===========================
	// Sub-test: Health checks
	// ===========================
	t.Run("HealthChecks", func(t *testing.T) {
		conn, err := grpc.NewClient(cortexAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("dial cortex: %v", err)
		}
		defer conn.Close()

		client := commonv1.NewHealthServiceClient(conn)
		resp, err := client.Check(context.Background(), &commonv1.HealthCheckRequest{Service: "cortex"})
		if err != nil {
			t.Fatalf("health check: %v", err)
		}
		if resp.GetStatus() != commonv1.HealthCheckResponse_SERVING {
			t.Errorf("expected SERVING, got %v", resp.GetStatus())
		}
	})

	// ===========================
	// Sub-test: List models
	// ===========================
	t.Run("ListModels", func(t *testing.T) {
		resp, err := http.Get(httpServer.URL + "/v1/models")
		if err != nil {
			t.Fatalf("list models: %v", err)
		}
		defer resp.Body.Close()

		var models openaicompat.ModelList
		json.NewDecoder(resp.Body).Decode(&models)
		if len(models.Data) != 2 {
			t.Errorf("expected 2 models, got %d", len(models.Data))
		}
	})

	// ===========================
	// Sub-test: Initial metrics are zero
	// ===========================
	t.Run("InitialMetricsZero", func(t *testing.T) {
		m := getMetrics(t, httpServer.URL)
		if m.TotalInteractions != 0 {
			t.Errorf("expected 0 interactions, got %d", m.TotalInteractions)
		}
		if m.UserSatisfactionRate != 0 {
			t.Errorf("expected 0 satisfaction rate, got %f", m.UserSatisfactionRate)
		}
	})

	// ===========================
	// Sub-test: Ingest documents & query via OpenAI API
	// ===========================
	t.Run("IngestAndQuery", func(t *testing.T) {
		// Ingest a document via the Cortex gRPC API (directly through memory service)
		memService.docs["doc-ml"] = "Machine learning is a subset of artificial intelligence focused on building systems that learn from data."
		memService.docs["doc-go"] = "Go is a statically typed, compiled programming language designed at Google."
		memService.docs["doc-arch"] = "Microservices architecture decomposes applications into small, independently deployable services."

		// Query via the OpenAI-compatible HTTP API
		response := chatCompletion(t, httpServer.URL, "gpt-4-test", "Tell me about machine learning")
		t.Logf("Response: %s", response)

		if response == "" {
			t.Error("expected non-empty response")
		}

		// Verify metrics increased
		m := getMetrics(t, httpServer.URL)
		if m.TotalInteractions == 0 {
			t.Error("expected interactions to increase after query")
		}
	})

	// =================================================
	// Sub-test: Feedback loop improves satisfaction rate
	// =================================================
	t.Run("FeedbackLoopImprovesMetrics", func(t *testing.T) {
		conn, err := grpc.NewClient(cortexAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("dial cortex: %v", err)
		}
		defer conn.Close()
		agentClient := agentv1.NewReasoningEngineClient(conn)

		// Phase 1: Send queries with NEGATIVE feedback (simulating bad answers)
		for i := 0; i < 3; i++ {
			stream, err := agentClient.StreamThoughtProcess(context.Background())
			if err != nil {
				t.Fatalf("open stream: %v", err)
			}

			// Send query
			if err := stream.Send(&agentv1.AgentInput{
				SessionId: fmt.Sprintf("neg-session-%d", i),
				InputType: &agentv1.AgentInput_UserQuery{
					UserQuery: fmt.Sprintf("Negative feedback query %d", i),
				},
			}); err != nil {
				t.Fatalf("send query: %v", err)
			}

			// Drain responses
			stream.CloseSend()
			for {
				_, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("recv: %v", err)
				}
			}

			// Send negative feedback via a separate stream
			feedbackStream, err := agentClient.StreamThoughtProcess(context.Background())
			if err != nil {
				t.Fatalf("open feedback stream: %v", err)
			}
			if err := feedbackStream.Send(&agentv1.AgentInput{
				SessionId: fmt.Sprintf("neg-session-%d", i),
				InputType: &agentv1.AgentInput_UserFeedback{
					UserFeedback: &agentv1.FeedbackSignal{
						Sentiment: agentv1.FeedbackSignal_NEGATIVE,
					},
				},
			}); err != nil {
				t.Fatalf("send feedback: %v", err)
			}
			feedbackStream.CloseSend()
			for {
				_, err := feedbackStream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("recv feedback: %v", err)
				}
			}
		}

		// Capture metrics after negative feedback
		mAfterNeg := getMetrics(t, httpServer.URL)
		t.Logf("After negative feedback: satisfaction=%.2f, interactions=%d",
			mAfterNeg.UserSatisfactionRate, mAfterNeg.TotalInteractions)

		// Phase 2: Send queries with POSITIVE feedback (simulating improvement)
		for i := 0; i < 7; i++ {
			stream, err := agentClient.StreamThoughtProcess(context.Background())
			if err != nil {
				t.Fatalf("open stream: %v", err)
			}

			if err := stream.Send(&agentv1.AgentInput{
				SessionId: fmt.Sprintf("pos-session-%d", i),
				InputType: &agentv1.AgentInput_UserQuery{
					UserQuery: fmt.Sprintf("Positive feedback query %d", i),
				},
			}); err != nil {
				t.Fatalf("send query: %v", err)
			}

			stream.CloseSend()
			for {
				_, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("recv: %v", err)
				}
			}

			// Send positive feedback
			feedbackStream, err := agentClient.StreamThoughtProcess(context.Background())
			if err != nil {
				t.Fatalf("open feedback stream: %v", err)
			}
			if err := feedbackStream.Send(&agentv1.AgentInput{
				SessionId: fmt.Sprintf("pos-session-%d", i),
				InputType: &agentv1.AgentInput_UserFeedback{
					UserFeedback: &agentv1.FeedbackSignal{
						Sentiment: agentv1.FeedbackSignal_POSITIVE,
					},
				},
			}); err != nil {
				t.Fatalf("send feedback: %v", err)
			}
			feedbackStream.CloseSend()
			for {
				_, err := feedbackStream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("recv feedback: %v", err)
				}
			}
		}

		// Capture metrics after positive feedback
		mAfterPos := getMetrics(t, httpServer.URL)
		t.Logf("After positive feedback: satisfaction=%.2f, interactions=%d",
			mAfterPos.UserSatisfactionRate, mAfterPos.TotalInteractions)

		// Verify satisfaction rate improved: 7 positive / (7 positive + 3 negative) = 0.7
		if mAfterPos.UserSatisfactionRate <= mAfterNeg.UserSatisfactionRate {
			t.Errorf("expected satisfaction rate to improve: before=%.2f, after=%.2f",
				mAfterNeg.UserSatisfactionRate, mAfterPos.UserSatisfactionRate)
		}
		if mAfterPos.UserSatisfactionRate < 0.5 {
			t.Errorf("expected satisfaction rate > 0.5 with 7 positive and 3 negative, got %.2f",
				mAfterPos.UserSatisfactionRate)
		}
	})

	// ====================================================
	// Sub-test: Quality trend increases with more queries
	// ====================================================
	t.Run("QualityTrendIncreases", func(t *testing.T) {
		// Record interactions with increasing quality via the metrics store directly
		// This simulates the system getting better over time as context enrichment
		// improves with more indexed documents.
		store := cortex.MetricsStore()

		// Simulate a series of interactions with increasing response quality
		for i := 0; i < 10; i++ {
			quality := 0.3 + float64(i)*0.07 // 0.3 → 0.93
			store.Record(metrics.InteractionRecord{
				SessionID:       fmt.Sprintf("trend-session-%d", i),
				Timestamp:       time.Now(),
				Query:           fmt.Sprintf("query %d", i),
				ResponseQuality: quality,
				ContextRelevance: quality,
				TopicDistribution: map[string]float64{
					"machine_learning": 0.5,
					"go_programming":   0.3,
					"architecture":     0.2,
				},
			})
		}

		// Recent trend (last 3) should be higher than overall trend
		recentTrend := store.RecentQualityTrend(3)
		overallTrend := store.RecentQualityTrend(100)
		t.Logf("Recent trend (last 3): %.3f, Overall trend: %.3f", recentTrend, overallTrend)

		if recentTrend <= overallTrend {
			t.Errorf("expected recent quality trend (%.3f) > overall trend (%.3f)",
				recentTrend, overallTrend)
		}
	})

	// ====================================================
	// Sub-test: Knowledge coverage broadens with diverse topics
	// ====================================================
	t.Run("KnowledgeCoverageBroadens", func(t *testing.T) {
		store := metrics.NewStore()

		// Phase 1: Single-topic interactions -> low coverage
		for i := 0; i < 5; i++ {
			store.Record(metrics.InteractionRecord{
				TopicDistribution: map[string]float64{"ml": 1.0},
			})
		}
		singleTopicCoverage := store.Summary().KnowledgeCoverage

		// Phase 2: Diverse-topic interactions -> higher coverage
		store.Record(metrics.InteractionRecord{
			TopicDistribution: map[string]float64{"systems": 1.0},
		})
		store.Record(metrics.InteractionRecord{
			TopicDistribution: map[string]float64{"databases": 1.0},
		})
		store.Record(metrics.InteractionRecord{
			TopicDistribution: map[string]float64{"networking": 1.0},
		})
		diverseCoverage := store.Summary().KnowledgeCoverage

		t.Logf("Single-topic coverage: %.3f, Diverse coverage: %.3f",
			singleTopicCoverage, diverseCoverage)

		if diverseCoverage <= singleTopicCoverage {
			t.Errorf("expected knowledge coverage to increase with diverse topics: single=%.3f, diverse=%.3f",
				singleTopicCoverage, diverseCoverage)
		}
	})

	// ====================================================
	// Sub-test: Multiple queries via OpenAI-compatible API
	// ====================================================
	t.Run("MultipleQueriesViaHTTP", func(t *testing.T) {
		queries := []string{
			"What is Go programming?",
			"Explain microservices architecture",
			"How does machine learning work?",
		}

		for _, q := range queries {
			resp := chatCompletion(t, httpServer.URL, "gpt-4-test", q)
			if resp == "" {
				t.Errorf("empty response for query %q", q)
			}
			t.Logf("Query: %q -> Response: %s", q, resp)
		}

		// Verify total interactions grew
		m := getMetrics(t, httpServer.URL)
		t.Logf("Final metrics: total=%d, quality=%.2f, relevance=%.2f, satisfaction=%.2f, coverage=%.2f",
			m.TotalInteractions, m.AvgResponseQuality, m.AvgContextRelevance,
			m.UserSatisfactionRate, m.KnowledgeCoverage)

		if m.TotalInteractions < 5 {
			t.Errorf("expected at least 5 total interactions, got %d", m.TotalInteractions)
		}
	})

	// ====================================================
	// Sub-test: Streaming completion via OpenAI-compatible API
	// ====================================================
	t.Run("StreamingCompletion", func(t *testing.T) {
		reqBody, _ := json.Marshal(openaicompat.ChatCompletionRequest{
			Model:  "gpt-4-test",
			Stream: true,
			Messages: []openaicompat.ChatMessage{
				{Role: "user", Content: "Stream test query"},
			},
		})

		resp, err := http.Post(httpServer.URL+"/v1/chat/completions", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("streaming request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
		}

		contentType := resp.Header.Get("Content-Type")
		if contentType != "text/event-stream" {
			t.Errorf("expected text/event-stream, got %q", contentType)
		}

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if len(bodyStr) == 0 {
			t.Error("expected non-empty streaming response")
		}
		if !bytes.Contains(body, []byte("data: [DONE]")) {
			t.Error("expected [DONE] marker in streaming response")
		}
		t.Logf("Streaming response length: %d bytes", len(body))
	})

	// ====================================================
	// Sub-test: Fake Gemini server works
	// ====================================================
	t.Run("FakeGeminiServerWorks", func(t *testing.T) {
		reqBody, _ := json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]string{{"text": "Hello Gemini"}}},
			},
		})

		resp, err := http.Post(fakeGemini.URL+"/v1beta/models/gemini-pro:generateContent?key=fake-key",
			"application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("gemini request: %v", err)
		}
		defer resp.Body.Close()

		var geminiResp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		json.NewDecoder(resp.Body).Decode(&geminiResp)
		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			t.Fatal("expected gemini response")
		}
		t.Logf("Gemini response: %s", geminiResp.Candidates[0].Content.Parts[0].Text)
	})

	// ====================================================
	// Sub-test: End-to-end flow summary assertion
	// ====================================================
	t.Run("EndToEndSummary", func(t *testing.T) {
		m := getMetrics(t, httpServer.URL)

		t.Logf("=== Final Integration Test Metrics ===")
		t.Logf("Total interactions:      %d", m.TotalInteractions)
		t.Logf("Avg response quality:    %.3f", m.AvgResponseQuality)
		t.Logf("Avg context relevance:   %.3f", m.AvgContextRelevance)
		t.Logf("User satisfaction rate:   %.3f", m.UserSatisfactionRate)
		t.Logf("Knowledge coverage:       %.3f", m.KnowledgeCoverage)
		t.Logf("Feedback counts:          %v", m.FeedbackCounts)
		t.Logf("Topic coverage:           %v", m.TopicCoverage)

		// The system should have processed multiple interactions
		if m.TotalInteractions < 10 {
			t.Errorf("expected at least 10 total interactions, got %d", m.TotalInteractions)
		}

		// Avg context relevance should be > 0 (we have indexed documents)
		if m.AvgContextRelevance <= 0 {
			t.Error("expected positive avg context relevance")
		}

		// Satisfaction rate should be above 0 (we sent positive feedback)
		if m.UserSatisfactionRate <= 0 {
			t.Error("expected positive satisfaction rate")
		}

		// Knowledge coverage should be above 0 (we have diverse topics)
		if m.KnowledgeCoverage < 0 {
			t.Error("expected non-negative knowledge coverage")
		}

		// Verify feedback counts make sense
		posCount := m.FeedbackCounts[metrics.FeedbackPositive]
		negCount := m.FeedbackCounts[metrics.FeedbackNegative]
		if posCount == 0 {
			t.Error("expected some positive feedback recorded")
		}
		if negCount == 0 {
			t.Error("expected some negative feedback recorded")
		}

		// Satisfaction = positive / (positive + negative + correction).
		// In this test we only send positive and negative, so correction = 0.
		corrCount := m.FeedbackCounts[metrics.FeedbackCorrection]
		expectedRate := float64(posCount) / float64(posCount+negCount+corrCount)
		if math.Abs(m.UserSatisfactionRate-expectedRate) > 0.01 {
			t.Errorf("satisfaction rate mismatch: expected ~%.3f, got %.3f",
				expectedRate, m.UserSatisfactionRate)
		}
	})
}
