package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds configuration for the Frontal Lobe service.
type Config struct {
	GRPCPort    int
	ServiceName string

	// LLM settings
	LLMProvider string // "mock", "openai", etc.
	LLMModel    string
	LLMAPIKey   string

	// Timeouts
	ReasoningTimeout time.Duration

	// Observability
	OTelEndpoint string
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		GRPCPort:         getEnvInt("FRONTAL_LOBE_GRPC_PORT", 50052),
		ServiceName:      getEnv("FRONTAL_LOBE_SERVICE_NAME", "frontal-lobe"),
		LLMProvider:      getEnv("LLM_PROVIDER", "mock"),
		LLMModel:         getEnv("LLM_MODEL", "gpt-4"),
		LLMAPIKey:        getEnv("LLM_API_KEY", ""),
		ReasoningTimeout: getDurationEnv("REASONING_TIMEOUT", 2*time.Minute),
		OTelEndpoint:     getEnv("OTEL_ENDPOINT", ""),
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
