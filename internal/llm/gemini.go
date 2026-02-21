package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

const (
	DefaultChatModel       = "gemini-2.5-flash"
	DefaultEmbeddingModel  = "text-embedding-004"
	DefaultMaxOutputTokens = 2048
	DefaultTemperature     = 0.7
)

// GeminiClient implements llm.Client using Google Gemini API.
type GeminiClient struct {
	client     *genai.Client
	chatModel  string
	embedModel string
	maxTokens  int
	temp       float32
}

// NewGeminiClient creates a Gemini client from environment variables.
// GEMINI_CHAT_MODEL, GEMINI_EMBEDDING_MODEL, GOOGLE_API_KEY.
func NewGeminiClient(ctx context.Context) (*GeminiClient, error) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY is required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	chatModel := os.Getenv("GEMINI_CHAT_MODEL")
	if chatModel == "" {
		chatModel = DefaultChatModel
	}
	embedModel := os.Getenv("GEMINI_EMBEDDING_MODEL")
	if embedModel == "" {
		embedModel = DefaultEmbeddingModel
	}
	maxTokens := DefaultMaxOutputTokens
	if n := os.Getenv("GEMINI_MAX_OUTPUT_TOKENS"); n != "" {
		var v int
		if _, err := fmt.Sscanf(n, "%d", &v); err == nil && v > 0 {
			maxTokens = v
		}
	}
	var temp float32 = DefaultTemperature
	if t := os.Getenv("GEMINI_TEMPERATURE"); t != "" {
		var f float64
		if _, err := fmt.Sscanf(t, "%f", &f); err == nil {
			if f < 0 {
				f = 0
			}
			if f > 2 {
				f = 2
			}
			temp = float32(f)
		}
	}
	gc := &GeminiClient{
		client:     client,
		chatModel:  chatModel,
		embedModel: embedModel,
		maxTokens:  maxTokens,
		temp:       temp,
	}
	return gc, nil
}

// Generate implements llm.Client.
func (c *GeminiClient) Generate(ctx context.Context, in GenerateInput) (*GenerateOutput, error) {
	maxTok := in.MaxTokens
	if maxTok <= 0 {
		maxTok = c.maxTokens
	}
	tempF := in.Temperature
	if tempF < 0 {
		tempF = c.temp
	}
	temp := &tempF

	var parts []*genai.Part
	if in.SystemPrompt != "" {
		parts = append(parts, &genai.Part{Text: "System: " + in.SystemPrompt})
	}
	for _, m := range in.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		parts = append(parts, &genai.Part{Text: role + ": " + m.Content})
	}

	contents := []*genai.Content{{Parts: parts}}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxTok),
		Temperature:     temp,
	}

	if len(in.Tools) > 0 {
		var fds []*genai.FunctionDeclaration
		for _, t := range in.Tools {
			fd := &genai.FunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
			}
			if t.Parameters != nil {
				fd.ParametersJsonSchema = t.Parameters
			}
			fds = append(fds, fd)
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: fds}}
	}

	resp, err := c.client.Models.GenerateContent(ctx, c.chatModel, contents, config)
	if err != nil {
		return nil, err
	}

	out := &GenerateOutput{FinishReason: "stop"}
	if resp.UsageMetadata != nil {
		out.UsagePrompt = int(resp.UsageMetadata.PromptTokenCount)
		out.UsageCompletion = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	if len(resp.Candidates) == 0 {
		return out, nil
	}
	cand := resp.Candidates[0]
	if cand.FinishReason != "" {
		out.FinishReason = strings.TrimPrefix(string(cand.FinishReason), "FINISH_REASON_")
	}
	if cand.Content != nil {
		for _, p := range cand.Content.Parts {
			if p.Text != "" {
				out.Content += p.Text
			}
			if p.FunctionCall != nil {
				fc := p.FunctionCall
				tc := ToolCall{
					ID:   fc.ID,
					Name: fc.Name,
					Args: make(map[string]any),
				}
				if fc.ID == "" {
					tc.ID = "call_" + fc.Name
				}
				for k, v := range fc.Args {
					tc.Args[k] = v
				}
				out.ToolCalls = append(out.ToolCalls, tc)
			}
		}
	}
	return out, nil
}

// Embed implements llm.Client.
func (c *GeminiClient) Embed(ctx context.Context, text string) ([]float32, error) {
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: text}}}}
	resp, err := c.client.Models.EmbedContent(ctx, c.embedModel, contents, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.Embeddings) == 0 || resp.Embeddings[0] == nil || len(resp.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return resp.Embeddings[0].Values, nil
}

// Close releases the client (no-op if SDK does not require it).
func (c *GeminiClient) Close() error {
	return nil
}

// Ensure GeminiClient implements Client at compile time.
var _ Client = (*GeminiClient)(nil)
