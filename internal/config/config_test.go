package config

import (
	"os"
	"testing"
)

// keys that Load() reads; unset for default tests
var loadEnvKeys = []string{
	"PORT", "GOOGLE_API_KEY",
	"GEMINI_CHAT_MODEL", "GEMINI_EMBEDDING_MODEL", "GEMINI_MAX_OUTPUT_TOKENS", "GEMINI_TEMPERATURE",
	"NOTION_TOKEN", "NOTION_USER_PROFILE_PAGE_ID", "NOTION_KNOWLEDGE_DATABASE_ID", "NOTION_KNOWLEDGE_TITLE_PROPERTY",
	"QDRANT_HOST", "QDRANT_PORT", "QDRANT_COLLECTION",
	"MEMORIZE_TOPIC_SIMILARITY_THRESHOLD", "WORKING_MEMORY_SIZE", "CHAT_TOOL_LOOP_MAX_ITER",
	"CHAT_DEFAULT_MAX_TOKENS", "CHAT_DEFAULT_TEMPERATURE",
	"KB_MAX_TEXT_CHARS", "KB_RRF_K", "KB_SEARCH_PREFETCH_MIN",
}

func unsetLoadKeys() {
	for _, k := range loadEnvKeys {
		os.Unsetenv(k)
	}
}

func TestLoad_Defaults(t *testing.T) {
	unsetLoadKeys()
	defer unsetLoadKeys()

	cfg := Load()

	if cfg.Port != DefaultPort {
		t.Errorf("Port = %q, want %q", cfg.Port, DefaultPort)
	}
	if cfg.TopicSimilarityThreshold != DefaultTopicSimilarityThreshold {
		t.Errorf("TopicSimilarityThreshold = %v, want %v", cfg.TopicSimilarityThreshold, DefaultTopicSimilarityThreshold)
	}
	if cfg.WorkingMemorySize != DefaultWorkingMemorySize {
		t.Errorf("WorkingMemorySize = %v, want %v", cfg.WorkingMemorySize, DefaultWorkingMemorySize)
	}
	if cfg.ChatToolLoopMaxIter != DefaultChatToolLoopMaxIter {
		t.Errorf("ChatToolLoopMaxIter = %v, want %v", cfg.ChatToolLoopMaxIter, DefaultChatToolLoopMaxIter)
	}
	if cfg.DefaultMaxTokens != DefaultMaxTokensVal {
		t.Errorf("DefaultMaxTokens = %v, want %v", cfg.DefaultMaxTokens, DefaultMaxTokensVal)
	}
	if cfg.DefaultTemperature != DefaultTemperatureVal {
		t.Errorf("DefaultTemperature = %v, want %v", cfg.DefaultTemperature, DefaultTemperatureVal)
	}
	if cfg.KBMaxTextChars != DefaultKBMaxTextChars {
		t.Errorf("KBMaxTextChars = %v, want %v", cfg.KBMaxTextChars, DefaultKBMaxTextChars)
	}
	if cfg.KBRRFK != DefaultKBRRFK {
		t.Errorf("KBRRFK = %v, want %v", cfg.KBRRFK, DefaultKBRRFK)
	}
	if cfg.KBSearchPrefetchMin != DefaultKBSearchPrefetchMin {
		t.Errorf("KBSearchPrefetchMin = %v, want %v", cfg.KBSearchPrefetchMin, DefaultKBSearchPrefetchMin)
	}
	if cfg.QdrantHost != "" || cfg.QdrantPort != 0 {
		t.Errorf("Qdrant (unset env) should be empty/0, got host=%q port=%d", cfg.QdrantHost, cfg.QdrantPort)
	}
	if cfg.QdrantCollection != DefaultQdrantCollection {
		t.Errorf("QdrantCollection = %q, want %q", cfg.QdrantCollection, DefaultQdrantCollection)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	os.Setenv("MEMORIZE_TOPIC_SIMILARITY_THRESHOLD", "0.9")
	os.Setenv("WORKING_MEMORY_SIZE", "50")
	os.Setenv("PORT", "9000")
	defer unsetLoadKeys()

	cfg := Load()

	if cfg.TopicSimilarityThreshold != 0.9 {
		t.Errorf("TopicSimilarityThreshold = %v, want 0.9", cfg.TopicSimilarityThreshold)
	}
	if cfg.WorkingMemorySize != 50 {
		t.Errorf("WorkingMemorySize = %v, want 50", cfg.WorkingMemorySize)
	}
	if cfg.Port != "9000" {
		t.Errorf("Port = %q, want 9000", cfg.Port)
	}
}

func TestLoad_DefaultTemperatureClamping(t *testing.T) {
	unsetLoadKeys()
	defer unsetLoadKeys()

	os.Setenv("GEMINI_TEMPERATURE", "3")
	cfg := Load()
	if cfg.DefaultTemperature != 2 {
		t.Errorf("GEMINI_TEMPERATURE=3 should clamp to 2, got %v", cfg.DefaultTemperature)
	}

	os.Unsetenv("GEMINI_TEMPERATURE")
	os.Setenv("GEMINI_TEMPERATURE", "-0.5")
	cfg = Load()
	if cfg.DefaultTemperature != 0 {
		t.Errorf("GEMINI_TEMPERATURE=-0.5 should clamp to 0, got %v", cfg.DefaultTemperature)
	}
}

func TestLoad_InvalidValues(t *testing.T) {
	os.Setenv("WORKING_MEMORY_SIZE", "invalid")
	defer os.Unsetenv("WORKING_MEMORY_SIZE")

	cfg := Load()
	if cfg.WorkingMemorySize != DefaultWorkingMemorySize {
		t.Errorf("invalid WORKING_MEMORY_SIZE should use default %d, got %d", DefaultWorkingMemorySize, cfg.WorkingMemorySize)
	}
}
