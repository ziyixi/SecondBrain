package agents

import (
	"context"
	"testing"
	"time"

	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/reasoning"
)

func TestReflectAgentGenerateWeeklyReview(t *testing.T) {
	llm := reasoning.NewMockLLM()
	agent := NewReflectAgent(llm)

	result, err := agent.GenerateWeeklyReview(
		context.Background(),
		time.Now().AddDate(0, 0, -7),
		time.Now(),
		[]string{"Task A", "Task B"},
		[]string{"Task C"},
		[]string{"Task D"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ReportMarkdown == "" {
		t.Error("expected non-empty report")
	}

	if len(result.StalledProjects) == 0 {
		t.Error("expected stalled projects from blocked tasks")
	}

	if len(result.SuggestedNextActions) == 0 {
		t.Error("expected suggested next actions")
	}

	if len(result.DormantIdeas) == 0 {
		t.Error("expected dormant ideas for exploration")
	}
}

func TestReflectAgentEmptyTasks(t *testing.T) {
	llm := reasoning.NewMockLLM()
	agent := NewReflectAgent(llm)

	result, err := agent.GenerateWeeklyReview(
		context.Background(),
		time.Now().AddDate(0, 0, -7),
		time.Now(),
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ReportMarkdown == "" {
		t.Error("expected non-empty report even with empty tasks")
	}
}

func TestReflectAgentManyActiveTasks(t *testing.T) {
	llm := reasoning.NewMockLLM()
	agent := NewReflectAgent(llm)

	activeTasks := []string{"T1", "T2", "T3", "T4", "T5", "T6", "T7"}

	result, err := agent.GenerateWeeklyReview(
		context.Background(),
		time.Now().AddDate(0, 0, -7),
		time.Now(),
		nil, activeTasks, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should suggest prioritizing when too many active tasks
	found := false
	for _, action := range result.SuggestedNextActions {
		if action == "Consider prioritizing — too many active tasks" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected suggestion to prioritize with many active tasks")
	}
}
