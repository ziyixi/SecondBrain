package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourusername/secondbrain/internal/config"
	"google.golang.org/genai"
)

// GeminiClient implements llm.Client using Google Gemini API.
type GeminiClient struct {
	client     *genai.Client
	chatModel  string
	embedModel string
	maxTokens  int
	temp       float32
}

// NewGeminiClient creates a Gemini client from the central config.
func NewGeminiClient(ctx context.Context, cfg *config.Config) (*GeminiClient, error) {
	if cfg.GoogleAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY is required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GoogleAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	gc := &GeminiClient{
		client:     client,
		chatModel:  cfg.GeminiChatModel,
		embedModel: cfg.GeminiEmbedModel,
		maxTokens:  cfg.GeminiMaxTokens,
		temp:       cfg.GeminiTemperature,
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

	var contents []*genai.Content
	for _, m := range in.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: []*genai.Part{{Text: m.Content}},
		})
	}

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxTok),
		Temperature:     temp,
	}
	if in.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: in.SystemPrompt}},
		}
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
