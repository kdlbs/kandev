//go:build !windows

package acpdbg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLingerAfterPromptDrainsDelayedFrameAndAutoReplies proves the prompt
// linger path: a frame the agent emits after session/prompt already returned is
// still drained and recorded, an agent-initiated request during the window is
// auto-replied so the agent does not hang, and the linger deadline stops the
// drain harmlessly.
func TestLingerAfterPromptDrainsDelayedFrameAndAutoReplies(t *testing.T) {
	framesPath := filepath.Join(t.TempDir(), "frames.jsonl")
	// Emit one late agent-initiated request (has id + method) then idle. The
	// linger window must outlive the emit but the deadline must still land.
	req := `{"jsonrpc":"2.0","id":99,"method":"cursor/task"}`
	cmd := fmt.Sprintf("printf '%%s\\n' '%s'; sleep 30", req)
	ctx, cancel := context.WithCancel(context.Background())
	runner, err := NewRunner(ctx, framesPath, RunConfig{
		AgentID: "linger-helper",
		Command: []string{"sh", "-c", cmd},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		runner.tree.kill(runner.cmd)
		runner.Close("test cleanup")
	})

	start := time.Now()
	lingerAfterPrompt(ctx, runner, 300*time.Millisecond)
	elapsed := time.Since(start)

	// The deadline was honored: lingering returned only after the window, and
	// well before the child's 30s sleep.
	if elapsed < 250*time.Millisecond {
		t.Fatalf("linger returned after %s, want it to honor the ~300ms window", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("linger returned after %s, want it bounded by the window", elapsed)
	}

	// The delayed request was recorded and auto-replied with method-not-found.
	raw, err := os.ReadFile(framesPath)
	if err != nil {
		t.Fatalf("read frames: %v", err)
	}
	frames := string(raw)
	if !strings.Contains(frames, `"cursor/task"`) {
		t.Fatalf("delayed request not recorded; frames:\n%s", frames)
	}
	if !strings.Contains(frames, "-32601") {
		t.Fatalf("delayed agent request was not auto-replied with method-not-found; frames:\n%s", frames)
	}
}

// TestLingerAfterPromptDeadlineWithNoFramesIsHarmless proves the linger window
// terminates cleanly when the agent emits nothing after the prompt.
func TestLingerAfterPromptDeadlineWithNoFramesIsHarmless(t *testing.T) {
	framesPath := filepath.Join(t.TempDir(), "frames.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	runner, err := NewRunner(ctx, framesPath, RunConfig{
		AgentID: "linger-idle-helper",
		Command: []string{"sh", "-c", "sleep 30"},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		runner.tree.kill(runner.cmd)
		runner.Close("test cleanup")
	})

	start := time.Now()
	lingerAfterPrompt(ctx, runner, 200*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond {
		t.Fatalf("linger returned after %s, want it to honor the ~200ms window", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("linger returned after %s, want it bounded by the window", elapsed)
	}
}
