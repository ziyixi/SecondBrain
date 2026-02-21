package llm

import (
	"context"
)

// ChatMessage represents a single turn in a conversation for the LLM.
type ChatMessage struct {
	Role    string // "user", "model", "system"
	Content string
}

// ToolCall represents a function call requested by the model.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult is the result of executing a tool, to be fed back to the model.
type ToolResult struct {
	CallID string
	Output string
}

// GenerateInput is the input for a single generate call (with optional tools).
type GenerateInput struct {
	SystemPrompt string
	Messages     []ChatMessage
	Tools        []ToolDef
	MaxTokens    int
	Temperature  float32
}

// ToolDef describes a tool the model can call.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON schema subset
}

// GenerateOutput is the raw output from the LLM.
type GenerateOutput struct {
	Content         string
	ToolCalls       []ToolCall
	FinishReason    string
	UsagePrompt     int
	UsageCompletion int
}

// Client is the interface for the LLM backend (e.g. Gemini).
type Client interface {
	// Generate produces a completion; if ToolCalls are present, caller should execute them and call again.
	Generate(ctx context.Context, in GenerateInput) (*GenerateOutput, error)
	// Embed returns the embedding vector for the given text (e.g. for vector search).
	Embed(ctx context.Context, text string) ([]float32, error)
}
