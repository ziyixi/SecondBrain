package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProviderConfig holds configuration for an LLM provider.
type ProviderConfig struct {
	Name    string // "openai", "google", "mock"
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

// Router routes LLM requests to different providers based on model name.
type Router struct {
	mu        sync.RWMutex
	providers map[string]LLMProvider // model name -> provider
	fallback  LLMProvider
}

// NewRouter creates a new provider router with a fallback provider.
func NewRouter(fallback LLMProvider) *Router {
	return &Router{
		providers: make(map[string]LLMProvider),
		fallback:  fallback,
	}
}

// Register associates a model name with a provider.
func (r *Router) Register(model string, provider LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[model] = provider
}

// ListModels returns all registered model names.
func (r *Router) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	models := make([]string, 0, len(r.providers))
	for m := range r.providers {
		models = append(models, m)
	}
	return models
}

// ForModel returns the provider for the given model, or the fallback.
func (r *Router) ForModel(model string) LLMProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.providers[model]; ok {
		return p
	}
	return r.fallback
}

// Generate routes to the appropriate provider (uses fallback if model is unknown).
func (r *Router) Generate(ctx context.Context, prompt string) (string, error) {
	return r.fallback.Generate(ctx, prompt)
}

// Classify routes to the appropriate provider (uses fallback if model is unknown).
func (r *Router) Classify(ctx context.Context, content string, categories []string) (string, float64, error) {
	return r.fallback.Classify(ctx, content, categories)
}

// GenerateWithModel routes to the provider registered for the given model.
func (r *Router) GenerateWithModel(ctx context.Context, model, prompt string) (string, error) {
	return r.ForModel(model).Generate(ctx, prompt)
}

// OpenAIProvider calls the OpenAI-compatible chat completions API.
type OpenAIProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewOpenAIProvider creates a provider that calls the OpenAI API.
func NewOpenAIProvider(apiKey, baseURL, model string, timeout time.Duration) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Generate calls the OpenAI chat completions endpoint.
func (p *OpenAIProvider) Generate(ctx context.Context, prompt string) (string, error) {
	reqBody := openAIChatRequest{
		Model: p.model,
		Messages: []openAIChatMessage{
			{Role: "user", Content: prompt},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/chat/completions", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling OpenAI API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshaling response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("OpenAI API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// Classify uses the OpenAI API to classify content.
func (p *OpenAIProvider) Classify(ctx context.Context, content string, categories []string) (string, float64, error) {
	prompt := fmt.Sprintf(
		"Classify the following content into exactly one of these categories: %s\n\nContent: %s\n\nRespond with only the category name.",
		strings.Join(categories, ", "), content,
	)
	result, err := p.Generate(ctx, prompt)
	if err != nil {
		return "", 0, err
	}
	result = strings.TrimSpace(result)
	for _, cat := range categories {
		if strings.EqualFold(result, cat) {
			return cat, 0.8, nil
		}
	}
	if len(categories) > 0 {
		return categories[0], 0.5, nil
	}
	return result, 0.5, nil
}

// GoogleProvider calls the Google Generative AI (Gemini) API.
type GoogleProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// NewGoogleProvider creates a provider that calls the Google GenAI API.
func NewGoogleProvider(apiKey, model string, timeout time.Duration) *GoogleProvider {
	if model == "" {
		model = "gemini-pro"
	}
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	return &GoogleProvider{
		apiKey:  apiKey,
		baseURL: "https://generativelanguage.googleapis.com",
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

type googleGenRequest struct {
	Contents []googleContent `json:"contents"`
}

type googleContent struct {
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleGenResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Generate calls the Google GenAI generateContent endpoint.
func (p *GoogleProvider) Generate(ctx context.Context, prompt string) (string, error) {
	reqBody := googleGenRequest{
		Contents: []googleContent{
			{Parts: []googlePart{{Text: prompt}}},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		p.baseURL, p.model, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling Google GenAI API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var genResp googleGenResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return "", fmt.Errorf("unmarshaling response: %w", err)
	}

	if genResp.Error != nil {
		return "", fmt.Errorf("Google GenAI API error: %s", genResp.Error.Message)
	}
	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return genResp.Candidates[0].Content.Parts[0].Text, nil
}

// Classify uses the Google GenAI API to classify content.
func (p *GoogleProvider) Classify(ctx context.Context, content string, categories []string) (string, float64, error) {
	prompt := fmt.Sprintf(
		"Classify the following content into exactly one of these categories: %s\n\nContent: %s\n\nRespond with only the category name.",
		strings.Join(categories, ", "), content,
	)
	result, err := p.Generate(ctx, prompt)
	if err != nil {
		return "", 0, err
	}
	result = strings.TrimSpace(result)
	for _, cat := range categories {
		if strings.EqualFold(result, cat) {
			return cat, 0.8, nil
		}
	}
	if len(categories) > 0 {
		return categories[0], 0.5, nil
	}
	return result, 0.5, nil
}
