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

	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/config"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/embedder"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/server"
	"github.com/ziyixi/SecondBrain/services/hippocampus/internal/vectorstore"
	commonv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/common/v1"
	memoryv1 "github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/memory/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()

	// Create dependencies
	store := vectorstore.NewInMemoryStore()
	emb := embedder.NewMockEmbedder(cfg.EmbeddingDimension)

	// Create server
	hippocampusServer := server.NewHippocampusServer(logger, cfg, store, emb)

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

	memoryv1.RegisterMemoryServiceServer(grpcServer, hippocampusServer)
	commonv1.RegisterHealthServiceServer(grpcServer, hippocampusServer)
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
		logger.Info("hippocampus service starting", "address", addr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down hippocampus service...")
	grpcServer.GracefulStop()
	logger.Info("hippocampus service stopped")
}
