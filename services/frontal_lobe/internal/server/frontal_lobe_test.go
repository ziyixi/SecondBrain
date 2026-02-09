package server

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/config"
	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/reasoning"
	agentv1 "github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/common/v1"
)

func newTestServer() *FrontalLobeServer {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := &config.Config{LLMProvider: "mock"}
	llm := reasoning.NewMockLLM()
	return NewFrontalLobeServer(logger, cfg, llm)
}

func TestFrontalLobeHealthCheck(t *testing.T) {
	s := newTestServer()
	resp, err := s.Check(context.Background(), &commonv1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != commonv1.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING, got %v", resp.Status)
	}
}

func TestClassifyItemActionable(t *testing.T) {
	s := newTestServer()

	resp, err := s.ClassifyItem(context.Background(), &agentv1.ClassifyRequest{
		Content: "Urgent deadline for project delivery",
		Source:  "email",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Classification != agentv1.ClassifyResponse_ACTIONABLE {
		t.Errorf("expected ACTIONABLE, got %v", resp.Classification)
	}

	if resp.Confidence <= 0 {
		t.Error("expected positive confidence")
	}
}

func TestClassifyItemTrash(t *testing.T) {
	s := newTestServer()

	resp, err := s.ClassifyItem(context.Background(), &agentv1.ClassifyRequest{
		Content: "Click here to unsubscribe from this promotion",
		Source:  "email",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Classification != agentv1.ClassifyResponse_TRASH {
		t.Errorf("expected TRASH, got %v", resp.Classification)
	}
}

func TestGenerateWeeklyReview(t *testing.T) {
	s := newTestServer()

	resp, err := s.GenerateWeeklyReview(context.Background(), &agentv1.WeeklyReviewRequest{
		UserId:         "user-1",
		CompletedTasks: []string{"Task A", "Task B"},
		ActiveTasks:    []string{"Task C"},
		BlockedTasks:   []string{"Task D"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ReportMarkdown == "" {
		t.Error("expected non-empty report")
	}

	if len(resp.StalledProjects) == 0 {
		t.Error("expected stalled projects")
	}

	if len(resp.SuggestedNextActions) == 0 {
		t.Error("expected suggested next actions")
	}
}
