package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
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

	// Create LLM provider
	var llm reasoning.LLMProvider
	switch cfg.LLMProvider {
	default:
		llm = reasoning.NewMockLLM()
	}

	// Create server
	frontalServer := server.NewFrontalLobeServer(logger, cfg, llm)

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
