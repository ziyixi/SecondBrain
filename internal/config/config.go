package config

import (
	"os"
	"strconv"
)

// All configurable values are read from env with these defaults.
// Set in .env or export; see .env.example and TECH.md.

const (
	DefaultTopicSimilarityThreshold = 0.85
	DefaultWorkingMemorySize        = 20
	DefaultChatToolLoopMaxIter      = 10
	DefaultMaxTokensVal             = 2048
	DefaultTemperatureVal           = 0.7
	DefaultKBMaxTextChars           = 6000
	DefaultKBRRFK                   = 60
	DefaultKBSearchPrefetchMin      = 20
)

// TopicSimilarityThreshold returns MEMORIZE_TOPIC_SIMILARITY_THRESHOLD (cosine, 0–1). Default 0.85.
func TopicSimilarityThreshold() float32 {
	return floatEnv("MEMORIZE_TOPIC_SIMILARITY_THRESHOLD", DefaultTopicSimilarityThreshold)
}

// WorkingMemorySize returns WORKING_MEMORY_SIZE (last N messages). Default 20.
func WorkingMemorySize() int {
	return intEnv("WORKING_MEMORY_SIZE", DefaultWorkingMemorySize)
}

// ChatToolLoopMaxIter returns CHAT_TOOL_LOOP_MAX_ITER (max tool-call rounds). Default 10.
func ChatToolLoopMaxIter() int {
	return intEnv("CHAT_TOOL_LOOP_MAX_ITER", DefaultChatToolLoopMaxIter)
}

// DefaultMaxTokens returns default max tokens when request omits it. Uses GEMINI_MAX_OUTPUT_TOKENS if set, else 2048.
func DefaultMaxTokens() int {
	if v := os.Getenv("GEMINI_MAX_OUTPUT_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return intEnv("CHAT_DEFAULT_MAX_TOKENS", DefaultMaxTokensVal)
}

// DefaultTemperature returns default temperature when request omits it. Uses GEMINI_TEMPERATURE if set, else 0.7.
// Clamped to [0, 2] to match Gemini API valid range.
func DefaultTemperature() float32 {
	var t float32
	if v := os.Getenv("GEMINI_TEMPERATURE"); v != "" {
		if f, err := strconv.ParseFloat(v, 32); err == nil {
			t = float32(f)
		} else {
			t = floatEnv("CHAT_DEFAULT_TEMPERATURE", DefaultTemperatureVal)
		}
	} else {
		t = floatEnv("CHAT_DEFAULT_TEMPERATURE", DefaultTemperatureVal)
	}
	if t < 0 {
		return 0
	}
	if t > 2 {
		return 2
	}
	return t
}

// KBMaxTextChars returns KB_MAX_TEXT_CHARS (truncate fetched Notion text). Default 6000 (~1500 tokens).
func KBMaxTextChars() int {
	return intEnv("KB_MAX_TEXT_CHARS", DefaultKBMaxTextChars)
}

// KBRRFK returns KB_RRF_K (RRF constant for hybrid scoring). Default 60.
func KBRRFK() int {
	return intEnv("KB_RRF_K", DefaultKBRRFK)
}

// KBSearchPrefetchMin returns KB_SEARCH_PREFETCH_MIN (min points to prefetch for re-rank). Default 20.
func KBSearchPrefetchMin() int {
	return intEnv("KB_SEARCH_PREFETCH_MIN", DefaultKBSearchPrefetchMin)
}

func floatEnv(key string, defaultVal float32) float32 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return defaultVal
	}
	return float32(f)
}

func intEnv(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}
