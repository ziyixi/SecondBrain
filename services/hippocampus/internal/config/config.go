package config

import (
	"os"
	"strconv"
)

// Config holds all configuration for the Hippocampus service.
type Config struct {
	GRPCPort    int
	ServiceName string

	// Vector store
	CollectionName    string
	EmbeddingDimension int

	// Chunking
	ChunkSize    int
	ChunkOverlap int

	// Observability
	OTelEndpoint string
}

// Load reads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		GRPCPort:           getEnvInt("HIPPOCAMPUS_GRPC_PORT", 50053),
		ServiceName:        getEnv("HIPPOCAMPUS_SERVICE_NAME", "hippocampus"),
		CollectionName:     getEnv("COLLECTION_NAME", "second_brain"),
		EmbeddingDimension: getEnvInt("EMBEDDING_DIMENSION", 384),
		ChunkSize:          getEnvInt("CHUNK_SIZE", 512),
		ChunkOverlap:       getEnvInt("CHUNK_OVERLAP", 50),
		OTelEndpoint:       getEnv("OTEL_ENDPOINT", ""),
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
