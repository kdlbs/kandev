package installer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "console",
	})
	return log
}

func TestResolveBinary_FoundInPath(t *testing.T) {
	// "ls" should always be in PATH on any test system
	path, err := ResolveBinary(context.Background(), "ls", nil, nil, testLogger())
	if err != nil {
		t.Fatalf("expected ls to be found in PATH: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestResolveBinary_NotFoundNoStrategy(t *testing.T) {
	_, err := ResolveBinary(context.Background(), "nonexistent-binary-xyz-12345", nil, nil, testLogger())
	if err == nil {
		t.Error("expected error for nonexistent binary with no strategy")
	}
}

func TestResolveBinary_FoundInSearchPaths(t *testing.T) {
	// Create a fake binary in a temp dir
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "my-tool")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	path, err := ResolveBinary(context.Background(), "my-tool", []string{fakeBinary}, nil, testLogger())
	if err != nil {
		t.Fatalf("expected to find binary in search paths: %v", err)
	}
	if path != fakeBinary {
		t.Errorf("expected %s, got %s", fakeBinary, path)
	}
}

type mockStrategy struct {
	binaryPath string
	err        error
	called     bool
}

func (m *mockStrategy) Name() string { return "mock" }
func (m *mockStrategy) Install(_ context.Context) (*InstallResult, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return &InstallResult{BinaryPath: m.binaryPath}, nil
}

func TestResolveBinary_FallsBackToStrategy(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "installed-tool")
	if err := os.WriteFile(fakeBinary, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("failed to create fake binary: %v", err)
	}

	strategy := &mockStrategy{binaryPath: fakeBinary}
	path, err := ResolveBinary(context.Background(), "nonexistent-binary-xyz-12345", nil, strategy, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strategy.called {
		t.Error("expected strategy to be called")
	}
	if path != fakeBinary {
		t.Errorf("expected %s, got %s", fakeBinary, path)
	}
}
