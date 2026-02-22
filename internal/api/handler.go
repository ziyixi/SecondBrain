package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/secondbrain/internal/config"
	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
)

// Memorizer is optional; when set, the LLM can call MemorizeInformation(topic, content).
type Memorizer interface {
	MemorizeInformation(ctx context.Context, topic string, content string) (message string, err error)
}

// ChatHandler handles POST /v1/chat/completions with LLM and memory.
type ChatHandler struct {
	LLM       llm.Client
	Working   memory.WorkingMemory
	Facts     memory.UserFactStore
	KB        memory.KnowledgeBase
	Memorizer Memorizer // optional; dynamic categorization (Notion + Qdrant)
	UserID    string    // optional; for profile/facts
}

// HandleChatCompletions implements the OpenAI-compatible chat completions endpoint.
func (h *ChatHandler) HandleChatCompletions(c *gin.Context) {
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages required"})
		return
	}

	userID := h.UserID
	if userID == "" {
		userID = c.GetHeader("X-User-ID")
	}
	if userID == "" {
		userID = "default"
	}

	// Build system prompt: include user profile (facts) from Notion
	systemPrompt := "You are a helpful assistant with access to memory tools. Use UpsertUserFact to save important facts about the user. Use SearchKnowledgeBase to search the knowledge base when needed."
	if h.Memorizer != nil {
		systemPrompt += " Use MemorizeInformation(topic, content) to store information under a topic; the system will create or update a category page in the knowledge base."
	}
	if h.Facts != nil {
		profile, err := h.Facts.GetProfile(c.Request.Context(), userID)
		if err == nil && profile != "" {
			systemPrompt += "\n\nUser profile (known facts):\n" + profile
		}
	}

	// Working memory: seed with last N from request or use only current messages
	messages := req.Messages
	if h.Working != nil {
		h.Working.Clear()
		for _, m := range messages {
			h.Working.Append(m.Role, m.Content)
		}
	}

	tools := h.buildTools()
	maxTokens := config.DefaultMaxTokens()
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	temp := config.DefaultTemperature()
	if req.Temperature != nil {
		temp = *req.Temperature
	}

	// Convert to LLM format and run loop (handle tool calls)
	convMessages := h.openAIMessagesToLLM(messages)
	input := llm.GenerateInput{
		SystemPrompt: systemPrompt,
		Messages:     convMessages,
		Tools:        tools,
		MaxTokens:    maxTokens,
		Temperature:  temp,
	}

	var lastContent string
	for iter := 0; iter < config.ChatToolLoopMaxIter(); iter++ {
		out, err := h.LLM.Generate(c.Request.Context(), input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		lastContent = out.Content
		if h.Working != nil {
			h.Working.Append("assistant", out.Content)
		}
		if len(out.ToolCalls) == 0 {
			break
		}
		// Append assistant turn (with tool calls), then tool results as user turns
		input.Messages = append(input.Messages, llm.ChatMessage{Role: "model", Content: out.Content})
		for _, tc := range out.ToolCalls {
			result := h.executeTool(c.Request.Context(), userID, tc)
			if h.Working != nil {
				h.Working.Append("user", "Tool "+tc.Name+" result: "+result)
			}
			input.Messages = append(input.Messages, llm.ChatMessage{Role: "user", Content: "Tool " + tc.Name + " result: " + result})
		}
	}

	// Build OpenAI response
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []ChatChoice{
			{
				Index:        0,
				Message:      &ChatMessage{Role: "assistant", Content: lastContent},
				FinishReason: "stop",
			},
		},
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) buildTools() []llm.ToolDef {
	tools := []llm.ToolDef{
		{
			Name:        "UpsertUserFact",
			Description: "Save a fact about the user to their profile. Use this when the user shares something you should remember (preferences, name, context).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fact": map[string]any{"type": "string", "description": "The fact to save"},
				},
				"required": []any{"fact"},
			},
		},
		{
			Name:        "SearchKnowledgeBase",
			Description: "Search the knowledge base (Notion-backed documents) with a query. Use when you need to look up information from the user's documents. Returns consolidated text from the most relevant Notion pages.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query"},
				},
				"required": []any{"query"},
			},
		},
	}
	if h.Memorizer != nil {
		tools = append(tools, llm.ToolDef{
			Name:        "MemorizeInformation",
			Description: "Store information under a topic in the knowledge base. The system will either append to an existing similar category (if topic is similar to an existing one) or create a new Notion page. Use for facts, notes, or content the user wants to remember.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topic":   map[string]any{"type": "string", "description": "Category or topic name (e.g. 'Project X', 'Meeting notes')"},
					"content": map[string]any{"type": "string", "description": "The information to store"},
				},
				"required": []any{"topic", "content"},
			},
		})
	}
	return tools
}

func (h *ChatHandler) openAIMessagesToLLM(messages []ChatMessage) []llm.ChatMessage {
	var out []llm.ChatMessage
	for _, m := range messages {
		role := m.Role
		if role == "system" {
			continue
		}
		if role == "assistant" {
			role = "model"
		}
		out = append(out, llm.ChatMessage{Role: role, Content: m.Content})
	}
	return out
}

func (h *ChatHandler) executeTool(ctx context.Context, userID string, tc llm.ToolCall) string {
	getStr := func(key string) string {
		if v, ok := tc.Args[key]; !ok {
			return ""
		} else if s, ok := v.(string); ok {
			return s
		} else if b, err := json.Marshal(v); err == nil {
			return strings.Trim(string(b), `"`)
		}
		return ""
	}
	switch tc.Name {
	case "UpsertUserFact":
		fact := getStr("fact")
		if fact == "" {
			return "error: fact is required"
		}
		if h.Facts == nil {
			return "error: fact store not configured"
		}
		if err := h.Facts.AppendFact(ctx, userID, fact); err != nil {
			return "error: " + err.Error()
		}
		return "Fact saved."
	case "SearchKnowledgeBase":
		query := getStr("query")
		if query == "" {
			return "error: query is required"
		}
		if h.KB == nil {
			return "error: knowledge base not configured"
		}
		hits, err := h.KB.Search(ctx, query, 5)
		if err != nil {
			return "error: " + err.Error()
		}
		if len(hits) == 0 {
			return "No results found."
		}
		var sb strings.Builder
		for i, hit := range hits {
			sb.WriteString("[")
			sb.WriteString(strconv.Itoa(i + 1))
			sb.WriteString("] ")
			sb.WriteString(hit.Text)
			sb.WriteString("\n\n")
		}
		return sb.String()
	case "MemorizeInformation":
		topic := getStr("topic")
		content := getStr("content")
		if topic == "" || content == "" {
			return "error: topic and content are required"
		}
		if h.Memorizer == nil {
			return "error: memorizer not configured (need Notion knowledge database + Qdrant)"
		}
		msg, err := h.Memorizer.MemorizeInformation(ctx, topic, content)
		if err != nil {
			return "error: " + err.Error()
		}
		return msg
	default:
		return "unknown tool: " + tc.Name
	}
}
