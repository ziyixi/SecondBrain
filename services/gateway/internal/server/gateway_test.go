package server

import (
	"context"
	"log/slog"
	"os"
	"testing"

	commonv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/ingestion/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestGatewayHealthCheck(t *testing.T) {
	s := NewGatewayServer(newTestLogger())

	resp, err := s.Check(context.Background(), &commonv1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != commonv1.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING, got %v", resp.Status)
	}
}

func TestIngestItem(t *testing.T) {
	s := NewGatewayServer(newTestLogger())

	resp, err := s.IngestItem(context.Background(), &ingestionv1.IngestRequest{
		Item: &ingestionv1.InboxItem{
			Id:         "test-1",
			Content:    "Test content",
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
	if resp.ItemId != "test-1" {
		t.Errorf("expected item ID 'test-1', got %q", resp.ItemId)
	}
}

func TestIngestItemNilItem(t *testing.T) {
	s := NewGatewayServer(newTestLogger())

	resp, err := s.IngestItem(context.Background(), &ingestionv1.IngestRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Accepted {
		t.Error("expected item to be rejected")
	}
}

func TestGetItemStatus(t *testing.T) {
	s := NewGatewayServer(newTestLogger())

	// Add item directly
	s.AddItem(&ingestionv1.InboxItem{
		Id:         "test-1",
		Content:    "Content",
		Source:     "email",
		ReceivedAt: timestamppb.Now(),
	})

	resp, err := s.GetItemStatus(context.Background(), &ingestionv1.ItemStatusRequest{
		ItemId: "test-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ItemId != "test-1" {
		t.Errorf("expected item ID 'test-1', got %q", resp.ItemId)
	}
	if resp.Status != commonv1.ProcessingStatus_PROCESSING_STATUS_NEW {
		t.Errorf("expected NEW status, got %v", resp.Status)
	}
}

func TestGetItemStatusNotFound(t *testing.T) {
	s := NewGatewayServer(newTestLogger())

	resp, err := s.GetItemStatus(context.Background(), &ingestionv1.ItemStatusRequest{
		ItemId: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != commonv1.ProcessingStatus_PROCESSING_STATUS_UNSPECIFIED {
		t.Errorf("expected UNSPECIFIED status, got %v", resp.Status)
	}
}

func TestListItems(t *testing.T) {
	s := NewGatewayServer(newTestLogger())

	s.AddItem(&ingestionv1.InboxItem{Id: "1", Content: "A", Source: "email"})
	s.AddItem(&ingestionv1.InboxItem{Id: "2", Content: "B", Source: "slack"})

	resp, err := s.ListItems(context.Background(), &ingestionv1.ListItemsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.TotalCount != 2 {
		t.Errorf("expected 2 items, got %d", resp.TotalCount)
	}
}
