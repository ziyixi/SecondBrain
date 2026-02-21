// Integration test server: real HTTP server with faked LLM/Notion and real Qdrant.
// Used by integration tests that start this binary and send HTTP requests to it.
// No GOOGLE_API_KEY required. Set QDRANT_HOST and QDRANT_PORT to point at Qdrant.

package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/yourusername/secondbrain/internal/api"
	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
	"github.com/yourusername/secondbrain/test/fakes"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Scripted LLM: first response = SearchKnowledgeBase("golang"), second = final answer (matches TestIntegration_MemorySearchAndGoal).
	scriptedLLM := fakes.NewScriptedLLM(
		&llm.GenerateOutput{
			Content: "",
			ToolCalls: []llm.ToolCall{
				{ID: "1", Name: "SearchKnowledgeBase", Args: map[string]any{"query": "golang"}},
			},
			FinishReason: "stop",
		},
		&llm.GenerateOutput{
			Content:      "Based on the knowledge base, Golang is a programming language used for backend services and cloud tools.",
			FinishReason: "stop",
		},
	)

	working := memory.NewWorkingMemory(20)
	facts := fakes.NewInMemoryFactStore()

	var kb memory.KnowledgeBase
	if os.Getenv("QDRANT_HOST") != "" || os.Getenv("QDRANT_PORT") != "" {
		embedder := func(ctx context.Context, text string) ([]float32, error) {
			return []float32{0.1, 0.2, 0.3, 0.4}, nil
		}
		qdrantKB, err := memory.NewQdrantKnowledgeBase(embedder, nil)
		if err != nil {
			log.Fatalf("NewQdrantKnowledgeBase: %v", err)
		}
		defer qdrantKB.Close()
		kb = qdrantKB
	}

	chat := &api.ChatHandler{
		LLM:     scriptedLLM,
		Working: working,
		Facts:   facts,
		KB:      kb,
	}
	r := api.Router(chat)

	srv := &http.Server{Addr: ":" + port, Handler: r}
	log.Printf("integration-server listening on :%s (fakes + Qdrant)", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe: %v", err)
	}
	_ = srv.Shutdown(context.Background())
}
