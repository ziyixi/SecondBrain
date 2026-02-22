package config

import (
	"os"
	"strconv"
	"strings"
)

// All configurable values are read from env in one place via Load().
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
	DefaultPort                     = "8080"
	DefaultQdrantHost               = "localhost"
	DefaultQdrantPort               = 6334
	DefaultQdrantCollection         = "knowledge"
	DefaultGeminiChatModel          = "gemini-2.5-flash"
	DefaultGeminiEmbeddingModel     = "text-embedding-004"
	DefaultNotionKnowledgeTitleProp = "Name"
)

// Config holds all env-derived configuration. Load once at startup via Load() and pass to components.
type Config struct {
	// Server
	Port string

	// Gemini / LLM
	GoogleAPIKey      string
	GeminiChatModel   string
	GeminiEmbedModel  string
	GeminiMaxTokens   int
	GeminiTemperature float32

	// Notion (empty means feature disabled)
	NotionToken                  string
	NotionUserProfilePageID      string
	NotionKnowledgeDatabaseID    string
	NotionKnowledgeTitleProperty string

	// Qdrant (empty host/zero port can mean disabled where used)
	QdrantHost       string
	QdrantPort       int
	QdrantCollection string

	// Tunables
	TopicSimilarityThreshold float32
	WorkingMemorySize        int
	ChatToolLoopMaxIter      int
	DefaultMaxTokens         int
	DefaultTemperature       float32
	KBMaxTextChars           int
	KBRRFK                   int
	KBSearchPrefetchMin      int
}

// Load reads all configuration from the environment once. Call at startup and pass the result to components.
func Load() *Config {
	cfg := &Config{}

	cfg.Port = envOrDefault("PORT", DefaultPort)

	cfg.GoogleAPIKey = strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	cfg.GeminiChatModel = envOrDefault("GEMINI_CHAT_MODEL", DefaultGeminiChatModel)
	cfg.GeminiEmbedModel = envOrDefault("GEMINI_EMBEDDING_MODEL", DefaultGeminiEmbeddingModel)
	cfg.GeminiMaxTokens = intEnv("GEMINI_MAX_OUTPUT_TOKENS", DefaultMaxTokensVal)
	if cfg.GeminiMaxTokens <= 0 {
		cfg.GeminiMaxTokens = DefaultMaxTokensVal
	}
	// DefaultTemperature / GeminiTemperature: when request omits, use GEMINI_TEMPERATURE if set else CHAT_DEFAULT_TEMPERATURE (clamped to [0,2])
	if v := os.Getenv("GEMINI_TEMPERATURE"); v != "" {
		if f, err := strconv.ParseFloat(v, 32); err == nil {
			cfg.GeminiTemperature = clampTemperature(float32(f))
		} else {
			cfg.GeminiTemperature = clampTemperature(floatEnv("CHAT_DEFAULT_TEMPERATURE", DefaultTemperatureVal))
		}
	} else {
		cfg.GeminiTemperature = clampTemperature(floatEnv("CHAT_DEFAULT_TEMPERATURE", DefaultTemperatureVal))
	}

	cfg.NotionToken = strings.TrimSpace(os.Getenv("NOTION_TOKEN"))
	cfg.NotionUserProfilePageID = strings.TrimSpace(os.Getenv("NOTION_USER_PROFILE_PAGE_ID"))
	cfg.NotionKnowledgeDatabaseID = strings.TrimSpace(os.Getenv("NOTION_KNOWLEDGE_DATABASE_ID"))
	cfg.NotionKnowledgeTitleProperty = envOrDefault("NOTION_KNOWLEDGE_TITLE_PROPERTY", DefaultNotionKnowledgeTitleProp)

	// Qdrant: only set when env is present so main can treat "unset" as "feature disabled"
	cfg.QdrantHost = strings.TrimSpace(os.Getenv("QDRANT_HOST"))
	if v := os.Getenv("QDRANT_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.QdrantPort = n
		}
	}
	cfg.QdrantCollection = envOrDefault("QDRANT_COLLECTION", DefaultQdrantCollection)

	cfg.TopicSimilarityThreshold = floatEnv("MEMORIZE_TOPIC_SIMILARITY_THRESHOLD", DefaultTopicSimilarityThreshold)
	cfg.WorkingMemorySize = intEnv("WORKING_MEMORY_SIZE", DefaultWorkingMemorySize)
	if cfg.WorkingMemorySize < 0 {
		cfg.WorkingMemorySize = DefaultWorkingMemorySize
	}
	cfg.ChatToolLoopMaxIter = intEnv("CHAT_TOOL_LOOP_MAX_ITER", DefaultChatToolLoopMaxIter)
	if cfg.ChatToolLoopMaxIter < 0 {
		cfg.ChatToolLoopMaxIter = DefaultChatToolLoopMaxIter
	}
	// DefaultMaxTokens: when request omits max_tokens, use GEMINI_MAX_OUTPUT_TOKENS if set else CHAT_DEFAULT_MAX_TOKENS
	cfg.DefaultMaxTokens = intEnv("GEMINI_MAX_OUTPUT_TOKENS", 0)
	if cfg.DefaultMaxTokens <= 0 {
		cfg.DefaultMaxTokens = intEnv("CHAT_DEFAULT_MAX_TOKENS", DefaultMaxTokensVal)
	}
	if cfg.DefaultMaxTokens <= 0 {
		cfg.DefaultMaxTokens = DefaultMaxTokensVal
	}
	cfg.DefaultTemperature = cfg.GeminiTemperature
	cfg.KBMaxTextChars = intEnv("KB_MAX_TEXT_CHARS", DefaultKBMaxTextChars)
	if cfg.KBMaxTextChars < 0 {
		cfg.KBMaxTextChars = DefaultKBMaxTextChars
	}
	cfg.KBRRFK = intEnv("KB_RRF_K", DefaultKBRRFK)
	if cfg.KBRRFK < 0 {
		cfg.KBRRFK = DefaultKBRRFK
	}
	cfg.KBSearchPrefetchMin = intEnv("KB_SEARCH_PREFETCH_MIN", DefaultKBSearchPrefetchMin)
	if cfg.KBSearchPrefetchMin < 0 {
		cfg.KBSearchPrefetchMin = DefaultKBSearchPrefetchMin
	}

	return cfg
}

func envOrDefault(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

func clampTemperature(t float32) float32 {
	if t < 0 {
		return 0
	}
	if t > 2 {
		return 2
	}
	return t
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
