package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Sensory Gateway service.
type Config struct {
	GRPCPort    int
	HTTPPort    int
	ServiceName string
	CortexAddr  string

	// Webhook settings
	WebhookSecret string

	// Poller settings
	PollInterval time.Duration

	// Observability
	OTelEndpoint string
}

// Load reads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		GRPCPort:      getEnvInt("GATEWAY_GRPC_PORT", 50054),
		HTTPPort:      getEnvInt("GATEWAY_HTTP_PORT", 8081),
		ServiceName:   getEnv("GATEWAY_SERVICE_NAME", "sensory-gateway"),
		CortexAddr:    getEnv("CORTEX_ADDR", "localhost:50051"),
		WebhookSecret: getEnv("WEBHOOK_SECRET", ""),
		PollInterval:  getDurationEnv("POLL_INTERVAL", 5*time.Minute),
		OTelEndpoint:  getEnv("OTEL_ENDPOINT", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
