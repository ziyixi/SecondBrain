package memory

import (
	"context"
	"strings"
	"testing"
)

func TestMemorizeInformation_Validation(t *testing.T) {
	// MemorizerService requires real KB and Notion; we only test validation via the error messages.
	// Full flow is covered by integration tests with real Qdrant + fake or real Notion.
	tests := []struct {
		name    string
		topic   string
		content string
		wantErr string
	}{
		{"empty topic", "", "some content", "topic and content are required"},
		{"empty content", "Topic", "", "topic and content are required"},
		{"both empty", "", "", "topic and content are required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use nil - MemorizeInformation will fail on validation before touching kb/notion
			svc := &MemorizerService{threshold: 0.85}
			_, err := svc.MemorizeInformation(context.Background(), tt.topic, tt.content)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
