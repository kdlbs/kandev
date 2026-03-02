package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "warn",
		Format:     "console",
		OutputPath: "stdout",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func TestMonitorParentLiveness_NoEnvVar(t *testing.T) {
	t.Setenv("KANDEV_PARENT_PIPE_FD", "")
	ch := monitorParentLiveness(newTestLogger(t))
	if ch != nil {
		t.Fatal("expected nil channel when env var is absent")
	}
}

func TestMonitorParentLiveness_InvalidEnvVar(t *testing.T) {
	t.Setenv("KANDEV_PARENT_PIPE_FD", "not-a-number")
	ch := monitorParentLiveness(newTestLogger(t))
	if ch != nil {
		t.Fatal("expected nil channel for invalid FD value")
	}
}

func TestMonitorParentLiveness_PipeBreak(t *testing.T) {
	// Create a pipe: r is the read-end (what agentctl would inherit),
	// w is the write-end (what the parent keeps open).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	// Point the env var at the read-end's FD.
	t.Setenv("KANDEV_PARENT_PIPE_FD", fmt.Sprintf("%d", r.Fd()))

	ch := monitorParentLiveness(newTestLogger(t))
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	// Channel should not be closed yet.
	select {
	case <-ch:
		t.Fatal("channel closed before pipe break")
	case <-time.After(50 * time.Millisecond):
	}

	// Close write-end to simulate parent death.
	_ = w.Close()

	select {
	case <-ch:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after pipe break")
	}
}
