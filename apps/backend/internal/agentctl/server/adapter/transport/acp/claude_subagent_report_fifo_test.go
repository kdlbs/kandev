//go:build !windows

package acp

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A named pipe at the reported path must not block settlement: the reader
// rejects non-regular files before opening them, so the card completes at once.
func TestClaudeSubagentFIFOReportDoesNotBlockSettlement(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "agent-123.output")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	adapter := newClaudeSubagentTestAdapter(t)
	startClaudeAsyncSubagent(t, adapter, "session-1", "task-1", "agent-123", fifo, true, 7)

	started := time.Now()
	done := make(chan struct{})
	go func() {
		adapter.settleClaudeSubagents("session-1", 7, "end_turn", false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("settlement blocked on a FIFO report")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("settlement took %s with a FIFO report", elapsed)
	}

	update := findClaudeSubagentEvent(drainEvents(adapter), "task-1")
	if update == nil || update.NormalizedPayload.SubagentTask().Status != "completed" ||
		update.NormalizedPayload.SubagentTask().ResultText != "" {
		t.Fatalf("settled payload = %+v, want completed without a result", update)
	}
}
