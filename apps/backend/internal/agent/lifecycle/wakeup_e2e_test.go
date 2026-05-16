//go:build wakeup_e2e

// Full-pipeline isolation test: bridge → ACP Adapter → Lifecycle Manager →
// event bus. Verifies that ScheduleWakeup-driven wakeup turns reach the same
// event-bus consumers (i.e. the orchestrator) that user-initiated turns do.
//
// This is a layer up from acp/wakeup_e2e_test.go — that test stopped at the
// adapter's Updates() channel. This one wires the adapter's events through
// the real Lifecycle Manager's handleAgentEvent path and captures everything
// published to the event bus.
//
// Build/run:
//
//	go test -tags wakeup_e2e -v -timeout 5m \
//	  ./internal/agent/lifecycle/ -run WakeupE2EFullPipeline
//
// Skips automatically when `npx` is not on PATH or
// ~/.claude/.credentials.json is missing.

package lifecycle

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

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	wakeupE2EDelay  = 60               // ScheduleWakeup clamps to [60, 3600]
	wakeupE2EBuffer = 30 * time.Second // grace after the timer should fire
)

// trackingBus captures every event published, with a timestamp.
type trackingBus struct {
	mu     sync.Mutex
	events []trackedBusEvent
}

type trackedBusEvent struct {
	At      time.Time
	Subject string
	Event   *bus.Event
}

func (b *trackingBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, trackedBusEvent{At: time.Now(), Subject: subject, Event: event})
	return nil
}

func (b *trackingBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *trackingBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (b *trackingBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (b *trackingBus) Close()            {}
func (b *trackingBus) IsConnected() bool { return true }

func (b *trackingBus) snapshot() []trackedBusEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]trackedBusEvent, len(b.events))
	copy(out, b.events)
	return out
}

// teeReader passes bytes through while echoing complete lines to stderr.
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
			i := -1
			for j, v := range t.buf {
				if v == '\n' {
					i = j
					break
				}
			}
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

// TestWakeupE2EFullPipeline_BridgeToEventBus drives the full stack:
//
//	@agentclientprotocol/claude-agent-acp (real bridge process)
//	  ↑↓ stdio
//	acp.Adapter (real)
//	  ↓ Updates() channel
//	[test glue: forwards each event to mgr.handleAgentEvent for the execution]
//	  ↓
//	lifecycle.Manager (real, with tracking event bus)
//	  ↓
//	trackingBus (captures every Publish)
//
// It sends a prompt that triggers ScheduleWakeup with delaySeconds=60, waits
// for the wakeup to fire, and asserts that wakeup-driven events reach the
// event bus — i.e. they'd reach the orchestrator. If this assertion fails,
// the bug lives in the lifecycle layer (it does receive the events but
// doesn't publish them).
func TestWakeupE2EFullPipeline_BridgeToEventBus(t *testing.T) {
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
		t.Fatalf("logger: %v", err)
	}

	// ---- Set up adapter and bridge ----
	cfg := &shared.Config{AgentID: "wakeup-fullpipe-e2e", WorkDir: workDir}
	adp := acp.NewAdapter(cfg, log)

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
		t.Fatalf("start bridge: %v", err)
	}

	if err := adp.Connect(stdinPipe, &teeReader{src: stdoutPipe, label: "bridge→adapter"}); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("Connect: %v", err)
	}

	t.Cleanup(func() {
		_ = adp.Close()
		_ = stdinPipe.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	// ---- Set up lifecycle Manager with tracking bus ----
	mgr := newTestManager()
	trackBus := &trackingBus{}
	// Reach into the manager's event publisher: replace the bus with the
	// tracker. eventPublisher is the only thing that calls eventBus.Publish.
	// We need the manager's eventBus reset too so handleAgentEvent's callers
	// flow through the tracker.
	mgr.eventBus = trackBus
	mgr.eventPublisher = NewEventPublisher(trackBus, log)

	// ---- Create the execution that ties the adapter session to the manager ----
	executionID := "wakeup-e2e-exec-1"
	taskID := "wakeup-e2e-task-1"
	exec := &AgentExecution{
		ID:             executionID,
		TaskID:         taskID,
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
		promptDoneCh:   make(chan PromptCompletionSignal, 1),
		historyEnabled: false, // skip history writes — we're only checking event flow
	}
	mgr.executionStore.Add(exec)

	// ---- Forward adapter events into the manager ----
	// This mirrors what StreamManager.connectUpdatesStream does in production
	// (callback into mgr.handleAgentEvent), but in-process.
	stopForward := make(chan struct{})
	var forwardWG sync.WaitGroup
	forwardWG.Add(1)
	adapterEvents := make([]streams.AgentEvent, 0, 128)
	var adapterEventsMu sync.Mutex
	go func() {
		defer forwardWG.Done()
		for {
			select {
			case ev, ok := <-adp.Updates():
				if !ok {
					return
				}
				adapterEventsMu.Lock()
				adapterEvents = append(adapterEvents, ev)
				adapterEventsMu.Unlock()
				// Stamp the execution session ID into the event if not set
				// (the adapter only knows its own session id after NewSession,
				// but events emitted before that don't carry it).
				if ev.SessionID != "" && exec.SessionID == "" {
					exec.SessionID = ev.SessionID
				}
				fmt.Fprintf(os.Stderr, "[adapter→mgr] event type=%s sessionID=%s text=%q\n",
					ev.Type, ev.SessionID, truncateE2E(ev.Text, 80))
				mgr.handleAgentEvent(exec, ev)
			case <-stopForward:
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(stopForward)
		forwardWG.Wait()
	})

	// ---- Run the wakeup scenario ----
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	t.Logf("Initialize…")
	if err := adp.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Logf("NewSession…")
	sid, err := adp.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	exec.SessionID = sid
	t.Logf("session=%s", sid)

	prompt := fmt.Sprintf(
		"Use the ScheduleWakeup tool exactly once with delaySeconds=%d, "+
			"prompt=\"continue probe\", reason=\"e2e full-pipeline test\". "+
			"Then stop. After waking up, just say \"WAKEUP_FIRED\" and stop.",
		wakeupE2EDelay,
	)

	t.Logf("sending initial prompt…")
	initialStart := time.Now()
	if err := adp.Prompt(ctx, prompt, nil); err != nil {
		t.Fatalf("initial Prompt: %v", err)
	}
	initialEnd := time.Now()
	t.Logf("initial prompt completed in %s", initialEnd.Sub(initialStart))

	preWakeupBusCount := len(trackBus.snapshot())
	t.Logf("bus events after initial prompt: %d", preWakeupBusCount)

	// Wait past the wakeup time.
	waitFor := time.Duration(wakeupE2EDelay)*time.Second + wakeupE2EBuffer
	t.Logf("waiting %s for wakeup to fire…", waitFor)
	deadline := time.Now().Add(waitFor)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		busCount := len(trackBus.snapshot())
		newSinceWakeup := busCount - preWakeupBusCount
		elapsed := time.Since(initialEnd).Truncate(time.Second)
		t.Logf("  t+%s: bus events total=%d (post-prompt-end=%d)", elapsed, busCount, newSinceWakeup)
	}

	final := trackBus.snapshot()
	postWakeupEvents := final[preWakeupBusCount:]
	t.Logf("post-prompt-end bus events: %d", len(postWakeupEvents))

	// Classify each tracked bus event
	type classified struct {
		At      time.Time
		Subject string
		Type    string
		Text    string
		Payload *AgentStreamEventPayload
	}
	var classifiedEvents []classified
	for _, te := range postWakeupEvents {
		c := classified{At: te.At, Subject: te.Subject}
		if te.Event != nil {
			c.Type = te.Event.Type
			if payload, ok := te.Event.Data.(AgentStreamEventPayload); ok {
				c.Payload = &payload
				if payload.Data != nil {
					c.Text = payload.Data.Text
				}
			}
		}
		classifiedEvents = append(classifiedEvents, c)
	}
	for i, c := range classifiedEvents {
		t.Logf("  [%d] subject=%s type=%s text=%q",
			i, c.Subject, c.Type, truncateE2E(c.Text, 80))
	}

	// Count adapter events for comparison (this is what reached the in-process boundary).
	adapterEventsMu.Lock()
	totalAdapterEvents := len(adapterEvents)
	adapterEventsSnapshot := make([]streams.AgentEvent, len(adapterEvents))
	copy(adapterEventsSnapshot, adapterEvents)
	adapterEventsMu.Unlock()

	t.Logf("adapter-side total events: %d", totalAdapterEvents)

	// Sanity: did the adapter actually see wakeup-driven text? It must, from
	// the prior test. Find any post-prompt-end message_chunk events with text.
	wakeupTexts := []string{}
	for _, ev := range adapterEventsSnapshot {
		if ev.Type == streams.EventTypeMessageChunk && strings.Contains(ev.Text, "WAKEUP") {
			wakeupTexts = append(wakeupTexts, ev.Text)
		}
	}
	if len(wakeupTexts) == 0 {
		t.Logf("WARNING: adapter did not surface 'WAKEUP' text. Cannot assert on bus publishing.")
	} else {
		t.Logf("✓ adapter surfaced wakeup text chunks: %v", wakeupTexts)
	}

	// ---- Detailed accounting per turn ----
	//
	// Each completed turn (initial + wakeup) should produce:
	//   - At least one agent.stream message_streaming event (the assistant text)
	//   - Exactly one agent.ready event (signals turn-end to the orchestrator)
	//
	// The orchestrator relies on agent.ready to call completeTurnForSession,
	// evaluate on_turn_complete workflow transitions, and dispatch queued
	// messages. If the wakeup turn doesn't publish agent.ready, the turn never
	// closes — even though messages persisted, workflow state is stuck.

	// Count complete events FROM ALL bus events (not just post-prompt-end, since
	// the first complete might have arrived just before our snapshot).
	all := trackBus.snapshot()
	agentReadyCount := 0
	streamingMsgsByText := []string{}
	completeCount := 0
	for _, te := range all {
		if te.Event == nil {
			continue
		}
		if te.Event.Type == "agent.ready" {
			agentReadyCount++
		}
		if payload, ok := te.Event.Data.(AgentStreamEventPayload); ok && payload.Data != nil {
			switch payload.Data.Type {
			case "message_streaming":
				streamingMsgsByText = append(streamingMsgsByText, payload.Data.Text)
			case "complete":
				completeCount++
			}
		}
	}

	t.Logf("=== full bus accounting ===")
	t.Logf("agent.ready events: %d (expect 2 — one per turn)", agentReadyCount)
	t.Logf("AgentStreamEventPayload type=complete events: %d (expect 2 — one per turn)", completeCount)
	t.Logf("message_streaming text values: %v", streamingMsgsByText)

	// Adapter saw 2 complete events (initial + wakeup), confirmed by debug log.
	// The lifecycle manager should publish 2 agent.ready events.
	if completeCount >= 2 && agentReadyCount < 2 {
		t.Errorf(
			"BUG: adapter delivered %d complete events but only %d agent.ready event(s) reached the bus. "+
				"This means MarkReady's `if execution.Status == AgentStatusReady` guard at "+
				"manager_interaction.go:896 is suppressing the wakeup turn's AgentReady event "+
				"because the wakeup path (fireWakeup → a.Prompt) bypasses SendPrompt and never "+
				"flips the execution back to Running. The orchestrator therefore never sees the "+
				"wakeup turn end → completeTurnForSession is never called → workflow on_turn_complete "+
				"never fires → queued messages aren't dispatched.",
			completeCount, agentReadyCount,
		)
	}
}

func truncateE2E(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
