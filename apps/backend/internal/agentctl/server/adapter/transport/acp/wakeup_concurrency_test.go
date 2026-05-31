package acp

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
)

// concurrencyFakeAgent is a minimal acp.Agent whose Prompt handler blocks until
// released, recording how many Prompt calls are in flight simultaneously. It
// lets a test observe whether kandev issues two overlapping conn.Prompt() calls.
type concurrencyFakeAgent struct {
	entered     chan struct{} // one signal per Prompt entry
	release     chan struct{} // closed to unblock all parked Prompts
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
}

func (f *concurrencyFakeAgent) Prompt(_ context.Context, _ acp.PromptRequest) (acp.PromptResponse, error) {
	cur := f.inFlight.Add(1)
	for {
		old := f.maxInFlight.Load()
		if cur <= old || f.maxInFlight.CompareAndSwap(old, cur) {
			break
		}
	}
	f.entered <- struct{}{}
	<-f.release
	f.inFlight.Add(-1)
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (f *concurrencyFakeAgent) Initialize(_ context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{ProtocolVersion: params.ProtocolVersion}, nil
}

func (f *concurrencyFakeAgent) NewSession(_ context.Context, _ acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: "sess-concurrency-test"}, nil
}

func (f *concurrencyFakeAgent) Authenticate(_ context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}
func (f *concurrencyFakeAgent) Cancel(_ context.Context, _ acp.CancelNotification) error { return nil }
func (f *concurrencyFakeAgent) CloseSession(_ context.Context, _ acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}
func (f *concurrencyFakeAgent) ListSessions(_ context.Context, _ acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}
func (f *concurrencyFakeAgent) ResumeSession(_ context.Context, _ acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}
func (f *concurrencyFakeAgent) SetSessionConfigOption(_ context.Context, _ acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}
func (f *concurrencyFakeAgent) SetSessionMode(_ context.Context, _ acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

// TestWakeupDoesNotRaceConcurrentPromptWithUserPrompt reproduces the
// turn-misalignment bug: when a ScheduleWakeup timer fires while a user prompt
// is still in flight, fireWakeup must NOT issue a second, concurrent
// conn.Prompt() against the bridge. Two overlapping prompts are what desync the
// bridge's per-prompt stop_reason from the turn it belongs to, shifting chat
// turns one prompt behind.
//
// The fake agent blocks inside Prompt and records peak concurrency. Before this
// fix, the synthetic wakeup prompt overlapped the user prompt (maxInFlight == 2)
// because fireWakeup called conn.Prompt() concurrently. This test verifies the
// fix serializes them so maxInFlight never exceeds 1 during the overlap window.
func TestWakeupDoesNotRaceConcurrentPromptWithUserPrompt(t *testing.T) {
	a := newTestAdapter()

	// Two in-memory pipes wire the adapter (client side) to the fake agent.
	//   clientToAgent: adapter writes requests -> agent reads.
	//   agentToClient: agent writes replies   -> adapter reads.
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()

	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	fa := &concurrencyFakeAgent{
		entered: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
	_ = acp.NewAgentSideConnection(fa, a2cW, c2aR)

	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sid, err := a.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	var userWG sync.WaitGroup
	userWG.Add(1)
	go func() {
		defer userWG.Done()
		_ = a.Prompt(ctx, "user message", nil)
	}()

	// Wait until the user prompt is parked inside the agent's Prompt handler.
	select {
	case <-fa.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("user prompt never reached the agent")
	}

	// Fire the wakeup while the user prompt is still in flight.
	a.fireWakeup(sid, "synthetic wakeup prompt")

	// Wakeup must not enter the agent while the user prompt is still parked.
	select {
	case <-fa.entered:
		t.Fatal("wakeup entered agent while user prompt was in flight")
	case <-time.After(100 * time.Millisecond):
	}

	if fa.maxInFlight.Load() > 1 {
		t.Fatalf("maxInFlight=%d during overlap window; prompts must be serialized so the bridge's stop_reason stays aligned with its turn", fa.maxInFlight.Load())
	}

	// Release the user prompt; the queued wakeup should run serially afterwards.
	close(fa.release)
	userWG.Wait()

	select {
	case <-fa.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("wakeup never ran after user prompt completed")
	}

	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aW.Close()
		_ = a2cW.Close()
	})
}

// TestWakeupDroppedWhenSessionChangesWhileQueued covers the case where a wakeup
// prompt queues behind an in-flight user prompt and the adapter's active session
// changes (NewSession/LoadSession/reset) before the wakeup gets the gate. The
// queued wakeup must target the session it was scheduled for; if that session is
// no longer current it must be dropped, not injected into the new session.
func TestWakeupDroppedWhenSessionChangesWhileQueued(t *testing.T) {
	a := newTestAdapter()

	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fa := &concurrencyFakeAgent{
		entered: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
	_ = acp.NewAgentSideConnection(fa, a2cW, c2aR)

	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	origSession, err := a.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	var userWG sync.WaitGroup
	userWG.Add(1)
	go func() {
		defer userWG.Done()
		_ = a.Prompt(ctx, "user message", nil)
	}()
	select {
	case <-fa.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("user prompt never reached the agent")
	}

	// Wakeup fires for the original session and queues behind the gate.
	a.fireWakeup(origSession, "scheduled wakeup for original session")

	// The active session changes while the wakeup waits (e.g. reset/resume).
	a.mu.Lock()
	a.sessionID = "different-session-after-reset"
	a.mu.Unlock()

	// Release the in-flight user prompt so the queued wakeup can run.
	close(fa.release)
	userWG.Wait()

	select {
	case <-fa.entered:
		t.Fatal("queued wakeup was sent after the session changed; it must be dropped to avoid injecting into an unrelated session")
	case <-time.After(100 * time.Millisecond):
	}

	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aW.Close()
		_ = a2cW.Close()
	})
}

// TestWakeupDoesNotConsumePendingContext verifies that a synthetic wakeup prompt
// does not read-and-clear pendingContext, which is reserved for the next user
// prompt (e.g. fork_session resume context).
func TestWakeupDoesNotConsumePendingContext(t *testing.T) {
	a := newTestAdapter()

	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fa := &concurrencyFakeAgent{
		entered: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
	_ = acp.NewAgentSideConnection(fa, a2cW, c2aR)

	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	sid, err := a.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	const canary = "resume-context-for-next-user-prompt"
	a.mu.Lock()
	a.pendingContext = canary
	a.mu.Unlock()

	a.fireWakeup(sid, "scheduled wakeup prompt")

	select {
	case <-fa.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("wakeup never reached the agent")
	}
	close(fa.release)

	a.mu.Lock()
	got := a.pendingContext
	a.mu.Unlock()
	if got != canary {
		t.Fatalf("pendingContext=%q, want %q — wakeup must not consume resume context", got, canary)
	}

	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aW.Close()
		_ = a2cW.Close()
	})
}
