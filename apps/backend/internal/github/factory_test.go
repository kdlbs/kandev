package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestNewLegacyGitTransportCredentialReturnsPAT(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITHUB", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GITHUB_TOKEN", "legacy-workspace-token")
	t.Setenv("GH_TOKEN", "")

	client, method, credential, err := newLegacyGitTransportCredential(
		context.Background(), nil, newTestLogger(),
	)
	if err != nil {
		t.Fatalf("newLegacyGitTransportCredential(): %v", err)
	}
	if method != AuthMethodPAT || credential != "legacy-workspace-token" {
		t.Fatalf("method/credential = %q/%q, want PAT legacy-workspace-token", method, credential)
	}
	if _, ok := client.(*PATClient); !ok {
		t.Fatalf("client = %T, want *PATClient", client)
	}
}

func TestNewLegacyGitTransportCredentialPrefersAmbientTokenOverStoredGH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX gh fixture")
	}
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "gh-invoked")
	ghPath := filepath.Join(binDir, "gh")
	script := "#!/bin/sh\n: > \"$KANDEV_TEST_GH_MARKER\"\nexit 0\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("KANDEV_MOCK_GITHUB", "")
	t.Setenv("PATH", binDir)
	t.Setenv("KANDEV_TEST_GH_MARKER", marker)
	t.Setenv("GITHUB_TOKEN", "lower-priority-ambient-token")
	t.Setenv("GH_TOKEN", "ambient-legacy-token")

	_, method, credential, err := newLegacyGitTransportCredential(
		context.Background(), nil, newTestLogger(),
	)
	if err != nil {
		t.Fatalf("newLegacyGitTransportCredential(): %v", err)
	}
	if method != AuthMethodPAT || credential != "ambient-legacy-token" {
		t.Fatalf("method/credential = %q/%q, want PAT ambient-legacy-token", method, credential)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stored gh account was consulted, stat error = %v", err)
	}
}
