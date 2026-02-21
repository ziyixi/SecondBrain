package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourusername/secondbrain/internal/api"
	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx := context.Background()
	gemini, err := llm.NewGeminiClient(ctx)
	if err != nil {
		log.Fatalf("NewGeminiClient: %v", err)
	}
	defer gemini.Close()

	working := memory.NewWorkingMemory(20)

	var facts memory.UserFactStore
	if os.Getenv("NOTION_TOKEN") != "" && os.Getenv("NOTION_USER_PROFILE_PAGE_ID") != "" {
		facts, err = memory.NewNotionUserFactStore()
		if err != nil {
			log.Printf("Notion fact store disabled: %v", err)
		}
	}

	var kb memory.KnowledgeBase
	if os.Getenv("QDRANT_HOST") != "" || os.Getenv("QDRANT_PORT") != "" {
		embedder := func(ctx context.Context, text string) ([]float32, error) {
			return gemini.Embed(ctx, text)
		}
		var fetcher memory.NotionPageFetcher
		if os.Getenv("NOTION_TOKEN") != "" {
			fetcher, _ = memory.NewNotionPageFetcher()
		}
		kb, err = memory.NewQdrantKnowledgeBase(embedder, fetcher)
		if err != nil {
			log.Printf("Qdrant KB disabled: %v", err)
		}
	}

	chat := &api.ChatHandler{
		LLM:     gemini,
		Working: working,
		Facts:   facts,
		KB:      kb,
	}
	r := api.Router(chat)

	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	log.Printf("Listening on :%s", port)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("Shutdown: %v", err)
	}
}
