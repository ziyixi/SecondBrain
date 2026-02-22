// Integration test server: real HTTP server with faked LLM/Notion and real Qdrant.
// Used by integration tests that start this binary and send HTTP requests to it.
// No GOOGLE_API_KEY required. Set QDRANT_HOST and QDRANT_PORT to point at Qdrant.

package main

import (
	"context"
	"log"
	"net/http"

	"github.com/yourusername/secondbrain/internal/api"
	"github.com/yourusername/secondbrain/internal/config"
	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
	"github.com/yourusername/secondbrain/test/fakes"
)

func main() {
	cfg := config.Load()

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

	working := memory.NewWorkingMemory(cfg.WorkingMemorySize)
	facts := fakes.NewInMemoryFactStore()

	var kb memory.KnowledgeBase
	if cfg.QdrantHost != "" || cfg.QdrantPort > 0 {
		embedder := func(ctx context.Context, text string) ([]float32, error) {
			return []float32{0.1, 0.2, 0.3, 0.4}, nil
		}
		qdrantKB, err := memory.NewQdrantKnowledgeBase(embedder, nil, cfg)
		if err != nil {
			log.Fatalf("NewQdrantKnowledgeBase: %v", err)
		}
		defer qdrantKB.Close()
		kb = qdrantKB
	}

	chat := &api.ChatHandler{
		Config:  cfg,
		LLM:     scriptedLLM,
		Working: working,
		Facts:   facts,
		KB:      kb,
	}
	r := api.Router(chat)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	log.Printf("integration-server listening on :%s (fakes + Qdrant)", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServe: %v", err)
	}
	_ = srv.Shutdown(context.Background())
}
