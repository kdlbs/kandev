package github

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "json",
		OutputPath: "stdout",
	})
	return log
}

func TestNewClient_MockEnvVar(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITHUB", "true")

	client, method, err := NewClient(context.Background(), nil, newTestLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != "mock" {
		t.Errorf("method = %q, want %q", method, "mock")
	}
	if _, ok := client.(*MockClient); !ok {
		t.Errorf("expected *MockClient, got %T", client)
	}
}

func TestNewClient_NoAuth_ReturnsNoop(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITHUB", "")

	client, method, err := NewClient(context.Background(), nil, newTestLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// If gh CLI is installed and authenticated, we get gh_cli — skip that case.
	if method == "gh_cli" {
		t.Skip("gh CLI is authenticated on this machine, skipping noop test")
	}
	if method != "none" {
		t.Errorf("method = %q, want %q", method, "none")
	}
	if _, ok := client.(*NoopClient); !ok {
		t.Errorf("expected *NoopClient, got %T", client)
	}
}
