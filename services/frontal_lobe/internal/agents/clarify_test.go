package agents

import (
	"context"
	"testing"

	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/reasoning"
)

func TestClarifyAgentActionable(t *testing.T) {
	llm := reasoning.NewMockLLM()
	agent := NewClarifyAgent(llm)

	result, err := agent.Process(context.Background(), "This is an urgent task with a deadline", "email", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Classification != "ACTIONABLE" {
		t.Errorf("expected ACTIONABLE, got %q", result.Classification)
	}

	if result.Confidence <= 0 {
		t.Error("expected positive confidence")
	}

	if len(result.ThoughtChain) == 0 {
		t.Error("expected thought chain to be populated")
	}
}

func TestClarifyAgentReference(t *testing.T) {
	llm := reasoning.NewMockLLM()
	agent := NewClarifyAgent(llm)

	result, err := agent.Process(context.Background(), "Here is a research paper about machine learning", "browser", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Classification != "REFERENCE" {
		t.Errorf("expected REFERENCE, got %q", result.Classification)
	}

	if result.SuggestedArea == "" {
		t.Error("expected suggested area")
	}
}

func TestClarifyAgentTrash(t *testing.T) {
	llm := reasoning.NewMockLLM()
	agent := NewClarifyAgent(llm)

	result, err := agent.Process(context.Background(), "Unsubscribe from promotional emails", "email", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Classification != "TRASH" {
		t.Errorf("expected TRASH, got %q", result.Classification)
	}

	if result.Priority != "LOW" {
		t.Errorf("expected LOW priority for trash, got %q", result.Priority)
	}
}

func TestClarifyAgentAreaRouting(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{"Bank statement and payment info", "Financial Health"},
		{"Research paper on deep learning", "Academic Publishing"},
		{"Lease agreement for apartment", "Housing"},
		{"Code review and deploy pipeline", "Engineering"},
		{"Random stuff", "General"},
	}

	llm := reasoning.NewMockLLM()
	agent := NewClarifyAgent(llm)

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result, err := agent.Process(context.Background(), tc.content, "email", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.SuggestedArea != tc.expected {
				t.Errorf("expected area %q, got %q", tc.expected, result.SuggestedArea)
			}
		})
	}
}

func TestClarifyAgentProjectDetection(t *testing.T) {
	llm := reasoning.NewMockLLM()
	agent := NewClarifyAgent(llm)

	result, err := agent.Process(context.Background(), "Update on PhaseNet seismic model training", "email", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SuggestedProject == "" {
		t.Error("expected project to be detected")
	}
}
