package server

import (
	"context"
	"testing"

	"log/slog"
	"os"

	agentv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1"
	commonv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/ingestion/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestHealthCheck(t *testing.T) {
	s := NewCortexServer(newTestLogger())

	resp, err := s.Check(context.Background(), &commonv1.HealthCheckRequest{
		Service: "cortex",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != commonv1.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING, got %v", resp.Status)
	}

	if resp.Version != "0.1.0" {
		t.Errorf("expected version 0.1.0, got %q", resp.Version)
	}

	if resp.Timestamp == nil {
		t.Error("expected timestamp to be set")
	}
}

func TestClassifyItemWithoutFrontalLobe(t *testing.T) {
	s := NewCortexServer(newTestLogger())

	resp, err := s.ClassifyItem(context.Background(), &agentv1.ClassifyRequest{
		Content: "test content",
		Source:  "email",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Classification != agentv1.ClassifyResponse_REFERENCE {
		t.Errorf("expected REFERENCE (fallback), got %v", resp.Classification)
	}
}

func TestGenerateWeeklyReviewWithoutFrontalLobe(t *testing.T) {
	s := NewCortexServer(newTestLogger())

	resp, err := s.GenerateWeeklyReview(context.Background(), &agentv1.WeeklyReviewRequest{
		UserId: "test-user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ReportMarkdown == "" {
		t.Error("expected non-empty report")
	}
}

func TestIngestItemWithoutHippocampus(t *testing.T) {
	s := NewCortexServer(newTestLogger())

	resp, err := s.IngestItem(context.Background(), &ingestionv1.IngestRequest{
		Item: &ingestionv1.InboxItem{
			Id:         "item-1",
			Content:    "Test email content",
			Source:     "email",
			ReceivedAt: timestamppb.Now(),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Accepted {
		t.Error("expected item to be accepted")
	}

	if resp.ItemId != "item-1" {
		t.Errorf("expected item ID 'item-1', got %q", resp.ItemId)
	}
}
