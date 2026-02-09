package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"time"

	"github.com/ziyixi/SecondBrain/services/cortex/internal/config"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/middleware"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/openaicompat"
	"github.com/ziyixi/SecondBrain/services/cortex/internal/server"
	agentv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/ingestion/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()

	// Create the Cortex server
	cortexServer := server.NewCortexServer(logger)
	defer cortexServer.Close()

	// Connect to downstream services (non-fatal if they're not available)
	if err := cortexServer.ConnectDownstream(cfg.FrontalLobeAddr, cfg.HippocampusAddr); err != nil {
		logger.Warn("failed to connect to some downstream services", "error", err)
	}

	// Configure gRPC server with interceptors and keepalive
	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Second,
			Time:                  5 * time.Minute,
			Timeout:               1 * time.Second,
		}),
		grpc.ChainUnaryInterceptor(
			middleware.UnaryRecovery(logger),
			middleware.UnaryLogging(logger),
			middleware.UnaryTimeout(cfg.DefaultTimeout),
		),
		grpc.ChainStreamInterceptor(
			middleware.StreamLogging(logger),
		),
	)

	// Register services
	agentv1.RegisterReasoningEngineServer(grpcServer, cortexServer)
	commonv1.RegisterHealthServiceServer(grpcServer, cortexServer)
	ingestionv1.RegisterIngestionServiceServer(grpcServer, cortexServer)
	reflection.Register(grpcServer)

	// Start listening
	addr := fmt.Sprintf(":%d", cfg.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "address", addr, "error", err)
		os.Exit(1)
	}

	// Set up OpenAI-compatible HTTP API
	availableModels := []string{"secondbrain", "mock"}
	openaiHandler := openaicompat.NewHandler(logger, availableModels)
	if err := openaiHandler.ConnectFrontalLobe(cfg.FrontalLobeAddr); err != nil {
		logger.Warn("failed to connect OpenAI handler to frontal lobe", "error", err)
	}
	defer openaiHandler.Close()

	httpMux := http.NewServeMux()
	openaiHandler.RegisterRoutes(httpMux)

	// Metrics endpoint
	metricsStore := cortexServer.MetricsStore()
	httpMux.HandleFunc("GET /v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metricsStore.Summary())
	})
	httpAddr := fmt.Sprintf(":%d", cfg.HTTPPort)
	httpServer := &http.Server{
		Addr:    httpAddr,
		Handler: httpMux,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("cortex service starting", "address", addr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		logger.Info("cortex HTTP server starting", "address", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down cortex service...")
	grpcServer.GracefulStop()
	httpServer.Shutdown(context.Background())
	logger.Info("cortex service stopped")
}
