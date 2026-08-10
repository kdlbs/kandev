package acp

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
)

// turnStampRecorder is a test double for shared.RecordTurnStartFunc that
// records every call's timestamp, guarded by its own mutex since sendPrompt
// may be invoked from concurrent callers.
type turnStampRecorder struct {
	mu    sync.Mutex
	calls []time.Time
}

func (r *turnStampRecorder) record(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, t)
}

func (r *turnStampRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// newTurnStampTestAdapter wires an Adapter with an injectable RecordTurnStart
// hook to a blocking fake agent, mirroring setupConcurrencyFakeAgent but with
// a caller-supplied cfg.RecordTurnStart so V1-07's three sendPrompt callers
// can be asserted independently.
func newTurnStampTestAdapter(t *testing.T, recorder *turnStampRecorder) (*Adapter, *concurrencyFakeAgent) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger init failed: %v", err)
	}
	cfg := &shared.Config{
		AgentID:         "test-agent",
		WorkDir:         t.TempDir(),
		RecordTurnStart: recorder.record,
	}
	a := NewAdapter(cfg, log)
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aW.Close()
		_ = a2cW.Close()
	})
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fa := &concurrencyFakeAgent{
		entered:     make(chan struct{}, 8),
		release:     make(chan struct{}),
		initialized: make(chan acp.InitializeRequest, 1),
	}
	// Release immediately: this test only cares whether RecordTurnStart was
	// called for each dispatch, not about serialization between them.
	close(fa.release)
	_ = acp.NewAgentSideConnection(fa, a2cW, c2aR)
	return a, fa
}

// TestSendPrompt_RecordsTurnStartOnAllThreeDispatchPaths verifies V1-07 /
// D-1: the turn-start stamp fires on every non-dropped dispatch that reaches
// conn.Prompt — the operator path (Prompt), mid-turn steering (PromptSteer),
// and the synthetic wakeup path (fireWakeup) — because all three funnel
// through sendPrompt's single, unconditional RecordTurnStart call placed
// after beginPromptTurn.
func TestSendPrompt_RecordsTurnStartOnAllThreeDispatchPaths(t *testing.T) {
	recorder := &turnStampRecorder{}
	a, fa := newTurnStampTestAdapter(t, recorder)
	ctx := context.Background()

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sid, err := a.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Operator path.
	if err := a.Prompt(ctx, "operator message", nil, 0); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	waitForPromptComplete(t, a)
	if got := recorder.count(); got != 1 {
		t.Fatalf("after Prompt: RecordTurnStart called %d times, want 1", got)
	}

	// Mid-turn steering path.
	if err := a.PromptSteer(ctx, "steer message", nil, 1); err != nil {
		t.Fatalf("PromptSteer: %v", err)
	}
	waitForPromptComplete(t, a)
	if got := recorder.count(); got != 2 {
		t.Fatalf("after PromptSteer: RecordTurnStart called %d times, want 2", got)
	}

	// Synthetic ScheduleWakeup path.
	a.fireWakeup(sid, "wakeup message")
	select {
	case <-fa.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("fireWakeup never reached the agent")
	}
	waitForPromptComplete(t, a)
	if got := recorder.count(); got != 3 {
		t.Fatalf("after fireWakeup: RecordTurnStart called %d times, want 3", got)
	}
}

// TestSendPrompt_DoesNotRecordTurnStartForDroppedWakeup verifies D-1's other
// half: a wakeup pinned to a session that is no longer current is dropped by
// sendPrompt's early return, before beginPromptTurn — so it must record
// nothing. A stamp on a dropped dispatch would bias a later probe toward
// "live" for a turn that never actually started.
func TestSendPrompt_DoesNotRecordTurnStartForDroppedWakeup(t *testing.T) {
	recorder := &turnStampRecorder{}
	a, _ := newTurnStampTestAdapter(t, recorder)
	ctx := context.Background()

	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// A wakeup pinned to a session that does not match the adapter's current
	// session is dropped by sendPrompt before beginPromptTurn.
	a.fireWakeup("stale-session-id", "stale wakeup")

	// Give the (synchronous, in this drop case) path a moment, then confirm
	// nothing was recorded.
	time.Sleep(50 * time.Millisecond)
	if got := recorder.count(); got != 0 {
		t.Fatalf("RecordTurnStart called %d times for a dropped wakeup, want 0", got)
	}
}
