package middleware

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestExtractTraceContext(t *testing.T) {
	md := metadata.New(map[string]string{
		"traceparent": "00-abcdef1234567890-0123456789abcdef-01",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	trace := ExtractTraceContext(ctx)
	if trace != "00-abcdef1234567890-0123456789abcdef-01" {
		t.Errorf("unexpected trace: %q", trace)
	}
}

func TestExtractTraceContextMissing(t *testing.T) {
	ctx := context.Background()
	trace := ExtractTraceContext(ctx)
	if trace != "" {
		t.Errorf("expected empty trace, got %q", trace)
	}
}
