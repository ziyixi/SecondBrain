package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/config"
	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/reasoning"
	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/server"
	agentv1 "github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/common/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()

	// Create LLM provider router
	var defaultLLM reasoning.LLMProvider
	switch cfg.LLMProvider {
	case "openai":
		defaultLLM = reasoning.NewOpenAIProvider(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel, cfg.ReasoningTimeout)
	case "google":
		defaultLLM = reasoning.NewGoogleProvider(cfg.LLMAPIKey, cfg.LLMModel, cfg.ReasoningTimeout)
	default:
		defaultLLM = reasoning.NewMockLLM()
	}

	router := reasoning.NewRouter(defaultLLM)

	// Register additional OpenAI models
	if cfg.OpenAIAPIKey != "" && cfg.OpenAIModels != "" {
		for _, model := range strings.Split(cfg.OpenAIModels, ",") {
			model = strings.TrimSpace(model)
			if model != "" {
				router.Register(model, reasoning.NewOpenAIProvider(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, model, cfg.ReasoningTimeout))
			}
		}
	}

	// Register additional Google models
	if cfg.GoogleAPIKey != "" && cfg.GoogleModels != "" {
		for _, model := range strings.Split(cfg.GoogleModels, ",") {
			model = strings.TrimSpace(model)
			if model != "" {
				router.Register(model, reasoning.NewGoogleProvider(cfg.GoogleAPIKey, model, cfg.ReasoningTimeout))
			}
		}
	}

	// Create server (router implements LLMProvider)
	frontalServer := server.NewFrontalLobeServer(logger, cfg, router)

	// Configure gRPC server
	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Second,
			Time:                  5 * time.Minute,
			Timeout:               1 * time.Second,
		}),
	)

	agentv1.RegisterReasoningEngineServer(grpcServer, frontalServer)
	commonv1.RegisterHealthServiceServer(grpcServer, frontalServer)
	reflection.Register(grpcServer)

	// Start listening
	addr := fmt.Sprintf(":%d", cfg.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "address", addr, "error", err)
		os.Exit(1)
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("frontal lobe service starting", "address", addr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down frontal lobe service...")
	grpcServer.GracefulStop()
	logger.Info("frontal lobe service stopped")
}
