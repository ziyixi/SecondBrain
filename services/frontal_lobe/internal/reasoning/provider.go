package reasoning

import (
	"strings"
	"time"
)

// ProviderConfig holds configuration for an LLM provider.
type ProviderConfig struct {
	Name    string // "openai", "google", "mock"
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

// matchCategory matches an LLM classification result against known categories.
// Returns the matched category with high confidence, or falls back to the first
// category with low confidence.
func matchCategory(result string, categories []string) (string, float64, error) {
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
