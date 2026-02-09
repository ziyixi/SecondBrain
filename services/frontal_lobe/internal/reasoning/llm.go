package reasoning

import (
	"context"
	"fmt"
	"strings"
)

// LLMProvider is the interface for LLM backends.
type LLMProvider interface {
	// Generate produces a text response from a prompt.
	Generate(ctx context.Context, prompt string) (string, error)

	// Classify classifies content into a category.
	Classify(ctx context.Context, content string, categories []string) (string, float64, error)
}

// MockLLM is a mock LLM provider for testing and development.
type MockLLM struct{}

// NewMockLLM creates a new mock LLM.
func NewMockLLM() *MockLLM {
	return &MockLLM{}
}

// Generate returns a canned response based on prompt keywords.
func (m *MockLLM) Generate(ctx context.Context, prompt string) (string, error) {
	lower := strings.ToLower(prompt)

	if strings.Contains(lower, "weekly review") || strings.Contains(lower, "report") {
		return `# Weekly Review

## Summary
This week's progress has been steady across all projects.

## Completed Tasks
- Reviewed and filed incoming documents
- Updated project statuses

## Active Projects
- Second Brain development is on track

## Recommendations
- Continue focusing on high-priority tasks
- Review stalled projects for next actions`, nil
	}

	if strings.Contains(lower, "classify") {
		return "ACTIONABLE", nil
	}

	if strings.Contains(lower, "extract") {
		return `{"title": "Extracted Document", "type": "reference"}`, nil
	}

	return fmt.Sprintf("Processed: %s", truncate(prompt, 100)), nil
}

// Classify returns a mock classification.
func (m *MockLLM) Classify(ctx context.Context, content string, categories []string) (string, float64, error) {
	if len(categories) == 0 {
		return "unknown", 0.0, nil
	}

	lower := strings.ToLower(content)

	// Simple keyword-based classification
	if strings.Contains(lower, "urgent") || strings.Contains(lower, "deadline") || strings.Contains(lower, "action") {
		return "ACTIONABLE", 0.9, nil
	}

	if strings.Contains(lower, "spam") || strings.Contains(lower, "unsubscribe") || strings.Contains(lower, "promotion") {
		return "TRASH", 0.85, nil
	}

	return "REFERENCE", 0.7, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
