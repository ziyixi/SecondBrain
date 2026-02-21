package config

import (
	"os"
	"testing"
)

func TestDefaults(t *testing.T) {
	// Clear env for default tests
	keys := []string{
		"MEMORIZE_TOPIC_SIMILARITY_THRESHOLD", "WORKING_MEMORY_SIZE", "CHAT_TOOL_LOOP_MAX_ITER",
		"CHAT_DEFAULT_MAX_TOKENS", "CHAT_DEFAULT_TEMPERATURE", "GEMINI_MAX_OUTPUT_TOKENS", "GEMINI_TEMPERATURE",
		"KB_MAX_TEXT_CHARS", "KB_RRF_K", "KB_SEARCH_PREFETCH_MIN",
	}
	for _, k := range keys {
		os.Unsetenv(k)
	}
	defer func() {
		for _, k := range keys {
			os.Unsetenv(k)
		}
	}()

	if v := TopicSimilarityThreshold(); v != DefaultTopicSimilarityThreshold {
		t.Errorf("TopicSimilarityThreshold() = %v, want %v", v, DefaultTopicSimilarityThreshold)
	}
	if v := WorkingMemorySize(); v != DefaultWorkingMemorySize {
		t.Errorf("WorkingMemorySize() = %v, want %v", v, DefaultWorkingMemorySize)
	}
	if v := ChatToolLoopMaxIter(); v != DefaultChatToolLoopMaxIter {
		t.Errorf("ChatToolLoopMaxIter() = %v, want %v", v, DefaultChatToolLoopMaxIter)
	}
	if v := DefaultMaxTokens(); v != DefaultMaxTokensVal {
		t.Errorf("DefaultMaxTokens() = %v, want %v", v, DefaultMaxTokensVal)
	}
	if v := DefaultTemperature(); v != DefaultTemperatureVal {
		t.Errorf("DefaultTemperature() = %v, want %v", v, DefaultTemperatureVal)
	}
	if v := KBMaxTextChars(); v != DefaultKBMaxTextChars {
		t.Errorf("KBMaxTextChars() = %v, want %v", v, DefaultKBMaxTextChars)
	}
	if v := KBRRFK(); v != DefaultKBRRFK {
		t.Errorf("KBRRFK() = %v, want %v", v, DefaultKBRRFK)
	}
	if v := KBSearchPrefetchMin(); v != DefaultKBSearchPrefetchMin {
		t.Errorf("KBSearchPrefetchMin() = %v, want %v", v, DefaultKBSearchPrefetchMin)
	}
}

func TestEnvOverride(t *testing.T) {
	os.Setenv("MEMORIZE_TOPIC_SIMILARITY_THRESHOLD", "0.9")
	os.Setenv("WORKING_MEMORY_SIZE", "50")
	defer os.Unsetenv("MEMORIZE_TOPIC_SIMILARITY_THRESHOLD")
	defer os.Unsetenv("WORKING_MEMORY_SIZE")

	if v := TopicSimilarityThreshold(); v != 0.9 {
		t.Errorf("TopicSimilarityThreshold() = %v, want 0.9", v)
	}
	if v := WorkingMemorySize(); v != 50 {
		t.Errorf("WorkingMemorySize() = %v, want 50", v)
	}
}
