package main

import (
	"context"
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

	"github.com/ziyixi/SecondBrain/services/gateway/internal/config"
	"github.com/ziyixi/SecondBrain/services/gateway/internal/poller"
	"github.com/ziyixi/SecondBrain/services/gateway/internal/server"
	"github.com/ziyixi/SecondBrain/services/gateway/internal/webhook"
	commonv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/ingestion/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()

	// Create servers
	gatewayServer := server.NewGatewayServer(logger)
	webhookHandler := webhook.NewHandler(logger, cfg.WebhookSecret)
	pollerService := poller.New(logger, cfg.PollInterval)

	// Set up gRPC server
	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Minute,
			MaxConnectionAgeGrace: 5 * time.Second,
			Time:                  5 * time.Minute,
			Timeout:               1 * time.Second,
		}),
	)

	ingestionv1.RegisterIngestionServiceServer(grpcServer, gatewayServer)
	commonv1.RegisterHealthServiceServer(grpcServer, gatewayServer)
	reflection.Register(grpcServer)

	// Set up HTTP server for webhooks
	mux := http.NewServeMux()
	webhookHandler.RegisterRoutes(mux)
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Forward webhook items to gateway server
	go func() {
		for item := range webhookHandler.Items() {
			gatewayServer.AddItem(item)
		}
	}()

	// Forward poller items to gateway server
	go func() {
		for item := range pollerService.Items() {
			gatewayServer.AddItem(item)
		}
	}()

	// Start pollers
	go pollerService.Start(ctx)

	// Start gRPC server
	grpcAddr := fmt.Sprintf(":%d", cfg.GRPCPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Error("failed to listen", "address", grpcAddr, "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("gateway gRPC server starting", "address", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Start HTTP server
	go func() {
		logger.Info("gateway HTTP server starting", "address", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gateway service...")
	grpcServer.GracefulStop()
	httpServer.Shutdown(context.Background()) //nolint:errcheck
	logger.Info("gateway service stopped")
}
