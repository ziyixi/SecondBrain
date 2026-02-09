package reasoning

import (
	"context"
	"strings"
	"testing"
)

func TestMockLLMGenerate(t *testing.T) {
	llm := NewMockLLM()

	tests := []struct {
		name     string
		prompt   string
		contains string
	}{
		{"weekly review", "Generate a weekly review report", "Weekly Review"},
		{"classify", "Classify this email content", "ACTIONABLE"},
		{"extract", "Extract metadata from document", "Extracted"},
		{"generic", "What is the weather?", "Processed:"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := llm.Generate(context.Background(), tc.prompt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(resp, tc.contains) {
				t.Errorf("expected response to contain %q, got %q", tc.contains, resp)
			}
		})
	}
}

func TestMockLLMClassify(t *testing.T) {
	llm := NewMockLLM()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"urgent", "This is urgent and needs action", "ACTIONABLE"},
		{"deadline", "Meeting deadline tomorrow", "ACTIONABLE"},
		{"spam", "Unsubscribe from this list", "TRASH"},
		{"reference", "Here is an article about Go programming", "REFERENCE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, confidence, err := llm.Classify(context.Background(), tc.content, []string{"ACTIONABLE", "REFERENCE", "TRASH"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.want {
				t.Errorf("expected %q, got %q", tc.want, result)
			}
			if confidence <= 0 {
				t.Error("expected positive confidence")
			}
		})
	}
}

func TestMockLLMClassifyEmptyCategories(t *testing.T) {
	llm := NewMockLLM()

	result, _, err := llm.Classify(context.Background(), "content", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "unknown" {
		t.Errorf("expected 'unknown', got %q", result)
	}
}
