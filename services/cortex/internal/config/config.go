package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Cortex service.
type Config struct {
	// Server settings
	GRPCPort    int
	HTTPPort    int
	ServiceName string

	// Downstream services
	FrontalLobeAddr  string
	HippocampusAddr  string
	GatewayAddr      string

	// MCP settings
	MCPServerURL  string
	NotionToken   string

	// Timeouts
	DefaultTimeout time.Duration
	StreamTimeout  time.Duration

	// Auth
	OAuthClientID     string
	OAuthClientSecret string

	// Observability
	OTelEndpoint string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		GRPCPort:          getEnvInt("CORTEX_GRPC_PORT", 50051),
		HTTPPort:          getEnvInt("CORTEX_HTTP_PORT", 8080),
		ServiceName:       getEnv("CORTEX_SERVICE_NAME", "cortex"),
		FrontalLobeAddr:   getEnv("FRONTAL_LOBE_ADDR", "localhost:50052"),
		HippocampusAddr:   getEnv("HIPPOCAMPUS_ADDR", "localhost:50053"),
		GatewayAddr:       getEnv("GATEWAY_ADDR", "localhost:50054"),
		MCPServerURL:      getEnv("MCP_SERVER_URL", "http://localhost:3000"),
		NotionToken:       getEnv("NOTION_TOKEN", ""),
		DefaultTimeout:    getDurationEnv("DEFAULT_TIMEOUT", 30*time.Second),
		StreamTimeout:     getDurationEnv("STREAM_TIMEOUT", 5*time.Minute),
		OAuthClientID:     getEnv("OAUTH_CLIENT_ID", ""),
		OAuthClientSecret: getEnv("OAUTH_CLIENT_SECRET", ""),
		OTelEndpoint:      getEnv("OTEL_ENDPOINT", ""),
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
