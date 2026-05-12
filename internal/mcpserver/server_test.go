package mcpserver

import (
	"bytes"
	"context"
	"io"
	"log"
	"strings"
	"testing"
)

func TestServeInitializeAndPing(t *testing.T) {
	var out bytes.Buffer
	server := New(log.New(io.Discard, "", 0))
	server.HandleInitialize(func(ctx context.Context, req Request) (any, error) {
		return map[string]any{
			"protocolVersion": "2025-06-18",
		}, nil
	})
	server.HandleMethod("ping", func(ctx context.Context, req Request) (any, error) {
		return map[string]any{}, nil
	})

	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}` + "\n",
	)

	if err := server.Serve(context.Background(), input, &out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("expected initialize response, got %s", got)
	}
	if !strings.Contains(got, `"id":2`) {
		t.Fatalf("expected ping response, got %s", got)
	}
}
