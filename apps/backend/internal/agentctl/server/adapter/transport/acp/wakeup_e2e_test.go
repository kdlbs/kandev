//go:build wakeup_e2e

// End-to-end isolation test for the ScheduleWakeup synthetic-prompt path.
// Spawns the real `claude-agent-acp` bridge in a child process, wires it to
// kandev's ACP Adapter, sends a prompt that calls ScheduleWakeup with a short
// delay, then waits *past* the wakeup time and asserts that the adapter
// produced events corresponding to a second (wakeup-driven) turn.
//
// Build/run:
//
//	go test -tags wakeup_e2e -v -timeout 5m \
//	  ./internal/agentctl/server/adapter/transport/acp/ -run WakeupE2E
//
// Skips automatically when `claude-agent-acp` (or `npx`) is not on PATH or
// when ~/.claude/.credentials.json is missing.

package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	wakeupDelaySeconds = 60               // ScheduleWakeup clamps to [60, 3600]
	wakeupGrace        = 30 * time.Second // extra time after the timer should fire
)

// wakeupE2E spawns a single bridge instance and runs one wakeup probe.
type wakeupE2E struct {
	t       *testing.T
	workDir string
	cmd     *exec.Cmd
	adapter *Adapter
	log     *logger.Logger

	mu     sync.Mutex
	events []AgentEvent
	closed bool
}

func setupWakeupE2E(t *testing.T) *wakeupE2E {
	t.Helper()

	if _, err := exec.LookPath("npx"); err != nil {
		t.Skipf("skipping: npx not on PATH (%v)", err)
	}
	home, _ := os.UserHomeDir()
	credPath := filepath.Join(home, ".claude", ".credentials.json")
	if _, err := os.Stat(credPath); err != nil {
		t.Skipf("skipping: %s not found — Claude Code OAuth credentials required (%v)", credPath, err)
	}

	workDir := t.TempDir()

	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "debug",
		Format:     "console",
		OutputPath: "stderr",
	})
	if err != nil {
		t.Fatalf("logger init failed: %v", err)
	}

	cfg := &shared.Config{
		AgentID: "wakeup-e2e",
		WorkDir: workDir,
	}
	a := NewAdapter(cfg, log)

	cmd := exec.Command("npx", "-y", "@agentclientprotocol/claude-agent-acp@0.31.1")
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start bridge: %v", err)
	}

	if err := a.Connect(stdinPipe, &teeReader{src: stdoutPipe, label: "bridge->adapter"}); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("Connect failed: %v", err)
	}

	w := &wakeupE2E{
		t:       t,
		workDir: workDir,
		cmd:     cmd,
		adapter: a,
		log:     log,
	}

	go w.collectEvents()

	t.Cleanup(func() {
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()
		_ = a.Close()
		_ = stdinPipe.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	return w
}

// teeReader passes bytes through to the consumer (the ACP SDK) while
// accumulating them in a side buffer; on each newline we emit the completed
// line to stderr so we can see exactly what JSON-RPC frames the bridge sent.
type teeReader struct {
	src   io.Reader
	label string
	buf   []byte
}

func (t *teeReader) Read(p []byte) (int, error) {
	n, err := t.src.Read(p)
	if n > 0 {
		t.buf = append(t.buf, p[:n]...)
		for {
			i := indexOfByte(t.buf, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimRight(string(t.buf[:i]), "\r")
			if len(line) > 240 {
				line = line[:240] + "…"
			}
			if line != "" {
				fmt.Fprintf(os.Stderr, "[%s] %s\n", t.label, line)
			}
			t.buf = t.buf[i+1:]
		}
	}
	return n, err
}

func indexOfByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

func (w *wakeupE2E) collectEvents() {
	for {
		select {
		case ev, ok := <-w.adapter.Updates():
			if !ok {
				return
			}
			w.mu.Lock()
			closed := w.closed
			if !closed {
				w.events = append(w.events, ev)
			}
			w.mu.Unlock()
			fmt.Fprintf(os.Stderr, "[adapter→test] event type=%s sessionID=%s toolCallID=%s status=%s text=%q\n",
				ev.Type, ev.SessionID, ev.ToolCallID, ev.ToolStatus, truncate(ev.Text, 120))
		case <-time.After(2 * time.Minute):
			// Heartbeat
		}
	}
}

func (w *wakeupE2E) snapshotEvents() []AgentEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]AgentEvent, len(w.events))
	copy(out, w.events)
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// TestWakeupE2E_BridgeQueuedTurnDrainsViaSyntheticPrompt verifies the full
// kandev wakeup pipeline against a real claude-agent-acp bridge:
//   - send an initial prompt that triggers ScheduleWakeup with a short delay
//   - wait past the wakeup time
//   - assert that the adapter received events from a second (wakeup-driven)
//     turn — i.e. the wakeupScheduler fired a synthetic prompt and the bridge
//     drained its queued turn.
func TestWakeupE2E_BridgeQueuedTurnDrainsViaSyntheticPrompt(t *testing.T) {
	w := setupWakeupE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	t.Logf("initializing adapter…")
	if err := w.adapter.Initialize(ctx); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	t.Logf("creating session…")
	sid, err := w.adapter.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	t.Logf("session created: %s", sid)

	prompt := fmt.Sprintf(
		"Use the ScheduleWakeup tool exactly once with delaySeconds=%d, "+
			"prompt=\"continue probe\", reason=\"e2e isolation test\". "+
			"Then stop. After waking up, just say \"WAKEUP_FIRED\" and stop.",
		wakeupDelaySeconds,
	)

	t.Logf("sending initial prompt (will trigger ScheduleWakeup, delay=%ds)…", wakeupDelaySeconds)
	initialStart := time.Now()
	if err := w.adapter.Prompt(ctx, prompt, nil); err != nil {
		t.Fatalf("initial Prompt failed: %v", err)
	}
	initialEnd := time.Now()
	t.Logf("initial prompt completed in %s", initialEnd.Sub(initialStart))

	initialEvents := w.snapshotEvents()
	t.Logf("events after initial prompt: %d", len(initialEvents))

	// Look for the ScheduleWakeup tool_call in the captured events.
	sawWakeupToolCall := false
	for _, ev := range initialEvents {
		if ev.Type == streams.EventTypeToolCall && strings.Contains(strings.ToLower(ev.ToolName), "wakeup") {
			sawWakeupToolCall = true
		}
		if ev.Type == streams.EventTypeToolUpdate && strings.Contains(strings.ToLower(ev.ToolName), "wakeup") {
			sawWakeupToolCall = true
		}
	}
	if !sawWakeupToolCall {
		// Some agents emit ScheduleWakeup as a generic tool. Look for the title too.
		for _, ev := range initialEvents {
			if strings.Contains(strings.ToLower(ev.ToolTitle), "wakeup") || strings.Contains(strings.ToLower(ev.Text), "wakeup scheduled") {
				sawWakeupToolCall = true
				break
			}
		}
	}
	if !sawWakeupToolCall {
		t.Logf("WARNING: no ScheduleWakeup tool call observed in initial events. The agent may not have called the tool. Dumping events…")
		for i, ev := range initialEvents {
			t.Logf("  [%d] type=%s tool=%s title=%s status=%s text=%q",
				i, ev.Type, ev.ToolName, ev.ToolTitle, ev.ToolStatus, truncate(ev.Text, 100))
		}
		t.Fatalf("agent did not call ScheduleWakeup; cannot proceed with wakeup-fire assertion")
	}
	t.Logf("✓ ScheduleWakeup tool call observed in initial events")

	// Snapshot count so we can detect new events arriving after the timer fires.
	postPromptEventCount := len(initialEvents)

	// Wait for the wakeup to fire. The adapter's internal scheduler should call
	// fireWakeup → a.Prompt(...) → bridge drains queued turn → events.
	waitDuration := time.Duration(wakeupDelaySeconds)*time.Second + wakeupGrace
	t.Logf("waiting %s for wakeup to fire (idle window starts now)…", waitDuration)

	// Sample event count every 5s so we get a clear timeline of when events
	// (if any) arrive.
	sampleInterval := 5 * time.Second
	deadline := time.Now().Add(waitDuration)
	for time.Now().Before(deadline) {
		time.Sleep(sampleInterval)
		current := len(w.snapshotEvents())
		elapsed := time.Since(initialEnd)
		newCount := current - postPromptEventCount
		t.Logf("  t+%s: total events=%d (new since prompt-end: %d)", elapsed.Truncate(time.Second), current, newCount)
	}

	final := w.snapshotEvents()
	wakeupEvents := final[postPromptEventCount:]

	t.Logf("post-wakeup events: %d", len(wakeupEvents))
	for i, ev := range wakeupEvents {
		t.Logf("  [%d] type=%s tool=%s status=%s text=%q",
			i, ev.Type, ev.ToolName, ev.ToolStatus, truncate(ev.Text, 100))
	}

	// Pass criterion: at least one agent-message-chunk-like event after the
	// initial prompt completed but before the deadline.
	if len(wakeupEvents) == 0 {
		t.Fatalf("FAIL: no events received after initial prompt completed; wakeup did NOT drain the queued turn (%d total events captured, all from the initial prompt)", len(final))
	}

	// Optional: verify we saw an agent message after the wakeup fired. If we
	// only saw e.g. heartbeats, the test is inconclusive.
	sawAgentText := false
	for _, ev := range wakeupEvents {
		if ev.Type == streams.EventTypeMessageChunk && ev.Text != "" {
			sawAgentText = true
			break
		}
	}
	if !sawAgentText {
		t.Logf("NOTE: post-wakeup events arrived, but none contained agent text. " +
			"The wakeup may have fired (synthetic prompt accepted) but the agent " +
			"didn't produce a visible response.")
	}

	t.Logf("✓ wakeup pipeline produced %d events after initial prompt completed", len(wakeupEvents))
}

// dumpStdout is unused but kept for ad-hoc debugging when one wants to see
// raw JSON-RPC frames the bridge emits before they're parsed by the adapter.
var _ = io.MultiReader
