package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourusername/secondbrain/internal/api"
	"github.com/yourusername/secondbrain/internal/config"
	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	gemini, err := llm.NewGeminiClient(ctx, cfg)
	if err != nil {
		log.Fatalf("NewGeminiClient: %v", err)
	}
	defer gemini.Close()

	working := memory.NewWorkingMemory(cfg.WorkingMemorySize)

	var facts memory.UserFactStore
	if cfg.NotionToken != "" && cfg.NotionUserProfilePageID != "" {
		facts, err = memory.NewNotionUserFactStore(cfg)
		if err != nil {
			log.Printf("Notion fact store disabled: %v", err)
		}
	}

	var kb memory.KnowledgeBase
	var qdrantKB *memory.QdrantKnowledgeBase
	if cfg.QdrantHost != "" || cfg.QdrantPort > 0 {
		embedder := func(ctx context.Context, text string) ([]float32, error) {
			return gemini.Embed(ctx, text)
		}
		var fetcher memory.NotionPageFetcher
		if cfg.NotionToken != "" {
			fetcher, _ = memory.NewNotionPageFetcher(cfg)
		}
		qdrantKB, err = memory.NewQdrantKnowledgeBase(embedder, fetcher, cfg)
		if err != nil {
			log.Printf("Qdrant KB disabled: %v", err)
		} else {
			kb = qdrantKB
			defer qdrantKB.Close()
		}
	}

	var memorizer api.Memorizer
	if notionKnowledge, err := memory.NewNotionKnowledgeStore(cfg); err == nil && qdrantKB != nil {
		memorizer = memory.NewMemorizerService(qdrantKB, notionKnowledge, cfg.TopicSimilarityThreshold, cfg)
	}

	chat := &api.ChatHandler{
		Config:    cfg,
		LLM:       gemini,
		Working:   working,
		Facts:     facts,
		KB:        kb,
		Memorizer: memorizer,
	}
	r := api.Router(chat)

	srv := &http.Server{Addr: ":" + cfg.Port, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	log.Printf("Listening on :%s", cfg.Port)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("Shutdown: %v", err)
	}
}
