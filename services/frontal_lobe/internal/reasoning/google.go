package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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

// Classify uses the Google GenAI API to classify content into one of the given categories.
func (p *GoogleProvider) Classify(ctx context.Context, content string, categories []string) (string, float64, error) {
	prompt := fmt.Sprintf(
		"Classify the following content into exactly one of these categories: %s\n\nContent: %s\n\nRespond with only the category name.",
		strings.Join(categories, ", "), content,
	)
	result, err := p.Generate(ctx, prompt)
	if err != nil {
		return "", 0, err
	}
	return matchCategory(result, categories)
}

// --- Google GenAI request/response types ---

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
