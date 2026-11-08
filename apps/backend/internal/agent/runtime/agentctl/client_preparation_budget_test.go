package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedRuntimePreparationUsesLongRunningClientAndCallerDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(50 * time.Millisecond):
			_, _ = w.Write([]byte(`{"success":true,"command":"prepared","process_generation":2}`))
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, httpClient: &http.Client{Timeout: 10 * time.Millisecond}, longRunningHTTPClient: &http.Client{Timeout: time.Second}}
	directive := &types.ManagedRuntimePreparationDirective{PackageSpec: "@openai/codex@0.166.0"}
	if _, err := c.start(context.Background(), directive); err != nil {
		t.Fatalf("private preparation was cut short by ordinary HTTP timeout: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := c.start(ctx, directive); err == nil {
		t.Fatal("preparation ignored caller deadline")
	}
}
