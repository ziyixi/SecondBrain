package api

// OpenAI-compatible request/response structs for POST /v1/chat/completions.
// See https://platform.openai.com/docs/api-reference/chat/create

// ChatCompletionRequest represents the incoming OpenAI-style request.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float32      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Tools       []ChatTool    `json:"tools,omitempty"`
	ToolChoice  *ToolChoice   `json:"tool_choice,omitempty"`
}

// ChatMessage represents a single message in the conversation.
type ChatMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
	// Name optional; ToolCallID / ToolCalls for assistant tool use (we handle server-side)
}

// ChatTool represents a tool definition (we inject server-side; client may send empty).
type ChatTool struct {
	Type     string        `json:"type"`
	Function *FunctionSpec `json:"function,omitempty"`
}

// FunctionSpec describes a function tool.
type FunctionSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolChoice controls tool use ("none", "auto", or specific tool).
type ToolChoice struct {
	Type     string `json:"type,omitempty"` // "none" | "auto" | "function"
	Function *struct {
		Name string `json:"name"`
	} `json:"function,omitempty"`
}

// ChatCompletionResponse is the OpenAI-style response.
type ChatCompletionResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []ChatChoice     `json:"choices"`
	Usage   *CompletionUsage `json:"usage,omitempty"`
}

// ChatChoice represents a single completion choice.
type ChatChoice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message"`
	FinishReason string       `json:"finish_reason"` // "stop", "length", "tool_calls"
}

// CompletionUsage holds token counts.
type CompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
