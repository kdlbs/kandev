package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// burstAgent is a minimal acp.Agent stub used by the load-replay regression
// test. None of these methods are exercised — we only need a peer that holds
// the connection open while we push notifications agent → client.
type burstAgent struct{}

func (burstAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{}, nil
}

func (burstAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}
func (burstAgent) Cancel(context.Context, acp.CancelNotification) error { return nil }
func (burstAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (burstAgent) DeleteSession(context.Context, acp.DeleteSessionRequest) (acp.DeleteSessionResponse, error) {
	return acp.DeleteSessionResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionDelete)
}

func (burstAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (burstAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}

func (burstAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{}, nil
}
func (burstAgent) Prompt(context.Context, acp.PromptRequest) (acp.PromptResponse, error) {
	return acp.PromptResponse{}, nil
}

func (burstAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (burstAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (burstAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

type replayLoadAgent struct {
	burstAgent
	conn *acp.AgentSideConnection
}

func (a *replayLoadAgent) Initialize(_ context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: req.ProtocolVersion,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: true,
		},
	}, nil
}

func (a *replayLoadAgent) LoadSession(ctx context.Context, req acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	usage := usageUpdateWithCost(200_000, 100, 1.23, "USD")
	if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: req.SessionId,
		Update:    acp.SessionUpdate{UsageUpdate: usage},
	}); err != nil {
		return acp.LoadSessionResponse{}, err
	}
	if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: req.SessionId,
		Update: acp.SessionUpdate{Plan: &acp.SessionUpdatePlan{
			Entries: []acp.PlanEntry{{Content: "replayed plan", Status: "in_progress"}},
		}},
	}); err != nil {
		return acp.LoadSessionResponse{}, err
	}
	return acp.LoadSessionResponse{}, nil
}

// burstClient is a minimal acp.Client stub that forwards SessionUpdate
// notifications to the adapter's real handler.
type burstClient struct {
	onUpdate func(acp.SessionNotification)
}

func (b *burstClient) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	b.onUpdate(n)
	return nil
}

func (*burstClient) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, errors.New("not implemented")
}

func (*burstClient) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, errors.New("not implemented")
}

func (*burstClient) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{}, errors.New("not implemented")
}

func (*burstClient) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, errors.New("not implemented")
}

func (*burstClient) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, errors.New("not implemented")
}

func (*burstClient) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, errors.New("not implemented")
}

func (*burstClient) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, errors.New("not implemented")
}

func (*burstClient) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, errors.New("not implemented")
}

// newBurstPair wires a real acp.ClientSideConnection / AgentSideConnection
// pair over in-memory pipes. The client's SessionUpdate forwards to the
// supplied handler. Cleanup is registered on the test.
func newBurstPair(t *testing.T, onUpdate func(acp.SessionNotification)) (*acp.ClientSideConnection, *acp.AgentSideConnection) {
	t.Helper()

	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()

	clientConn := acp.NewClientSideConnection(&burstClient{onUpdate: onUpdate}, c2aW, a2cR)
	agentConn := acp.NewAgentSideConnection(burstAgent{}, a2cW, c2aR)

	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	return clientConn, agentConn
}

func TestLoadSession_DrainsBackedUpReplayBeforeClearingSuppression(t *testing.T) {
	a := newTestAdapter()
	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})
	if err := a.Connect(c2aW, a2cR); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	fake := &replayLoadAgent{}
	fake.conn = acp.NewAgentSideConnection(fake, a2cW, c2aR)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	drainEvents(a)

	workerBlocked := make(chan struct{})
	releaseWorker := make(chan struct{})
	workerReleased := false
	defer func() {
		if !workerReleased {
			close(releaseWorker)
		}
	}()
	a.dialect.suppressNotification = func(n acp.SessionNotification) bool {
		if n.Update.UsageUpdate != nil {
			close(workerBlocked)
			<-releaseWorker
		}
		return false
	}

	loadDone := make(chan error, 1)
	go func() { loadDone <- a.LoadSession(ctx, "replay-session", nil) }()
	select {
	case <-workerBlocked:
	case <-ctx.Done():
		t.Fatal("replay usage did not reach blocked update worker")
	}
	select {
	case err := <-loadDone:
		t.Fatalf("LoadSession returned before replay queue drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseWorker)
	workerReleased = true
	if err := <-loadDone; err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	a.mu.RLock()
	loading := a.isLoadingSession
	a.mu.RUnlock()
	if loading {
		t.Fatal("load suppression remained active after replay barrier")
	}
	if delta, cost := a.consumeUsageDelta("replay-session"); delta != 0 || cost != 0 {
		t.Fatalf("replay baseline leaked into first turn: delta=%d cost=%d", delta, cost)
	}
	a.convertUsageUpdate("replay-session", usageUpdateWithCost(200_000, 120, 1.25, "USD"))
	if delta, cost := a.consumeUsageDelta("replay-session"); delta != 20 || cost != 200 {
		t.Fatalf("post-load turn delta = (%d, %d), want (20, 200)", delta, cost)
	}

	events := drainEvents(a)
	var replayPlanSeen bool
	for _, event := range events {
		if event.Type == streams.EventTypePlan && len(event.PlanEntries) == 1 &&
			event.PlanEntries[0].Description == "replayed plan" {
			replayPlanSeen = true
		}
	}
	if !replayPlanSeen {
		t.Fatal("captured replay plan was not re-emitted after queue drain")
	}
}

// TestLoadReplayBurst_HandlesLargeReplay is a regression test for the
// "notification queue overflow" failure we hit on a 304-exchange auggie
// session load. The acp-go-sdk exposes a hardcoded 1024-slot inbound channel
// (defaultMaxQueuedNotifications) and shuts down the connection when the
// single-goroutine consumer falls behind. Before the adapter_updates.go
// fast-path, json.Marshal + LogRawEvent on every replayed notification was
// slow enough that the session/load burst would back the queue up.
//
// This test wires a real acp.ClientSideConnection / AgentSideConnection pair
// over io.Pipes, sets isLoadingSession=true on the adapter, and pushes a
// large burst of replay notifications agent → client. It then sends an
// AvailableCommandsUpdate (which is intentionally NOT suppressed during
// load) as a sentinel and asserts:
//  1. the connection is still alive after the burst (overflow would have
//     closed it),
//  2. the sentinel flows through the un-suppressed path,
//  3. the last Plan from the burst is captured for post-load re-emit.
//
// Note: in-memory io.Pipe is synchronous, so the producer is naturally
// gated by the consumer drain rate; an overflow here would mean the
// handler is hanging, not just slow. The throughput-sensitive scenario
// is exercised by BenchmarkLoadReplayHandler below.
func TestLoadReplayBurst_HandlesLargeReplay(t *testing.T) {
	const notificationBurstSize = 20000
	const sentinelCmd = "burst-sentinel"

	a := newTestAdapter()
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	// Exercise the production path: SDK → enqueueACPUpdate → notifQueue →
	// runUpdateWorker → handleACPUpdate. NewAdapter starts the worker; we
	// just rely on Close to drain it.
	t.Cleanup(func() { _ = a.Close() })

	clientConn, agentConn := newBurstPair(t, a.enqueueACPUpdate)

	const sessionID = "burst-session"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Push the burst of replay notifications. These would all be suppressed
	// by the adapter (AgentMessageChunk + ToolCall + Plan are on the suppress
	// list) — the point is whether the SDK's 1024-deep queue stays drained.
	for i := 0; i < notificationBurstSize; i++ {
		if err := agentConn.SessionUpdate(ctx, makeReplayNotification(sessionID, i)); err != nil {
			t.Fatalf("SessionUpdate at i=%d failed: %v", i, err)
		}
	}

	// Sentinel: AvailableCommandsUpdate passes through during load, so it
	// will land on updatesCh once the consumer drains the queue.
	if err := agentConn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: acp.SessionId(sessionID),
		Update: acp.SessionUpdate{
			AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
				AvailableCommands: []acp.AvailableCommand{
					{Name: sentinelCmd, Description: "burst sentinel"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("sentinel SessionUpdate failed: %v", err)
	}

	// The sentinel-wait loop below is the authoritative overflow check: the
	// SDK tears down both sides if its inbound queue ever stalls, so a closed
	// Done() observed there proves overflow. A non-blocking peek here would
	// race the async teardown and could produce a false PASS.

	// Wait for the sentinel to flow through to the adapter's updates channel.
	deadline := time.After(15 * time.Second)
	var got *AgentEvent
	for got == nil {
		select {
		case ev := <-a.updatesCh:
			if ev.Type == streams.EventTypeAvailableCommands {
				got = &ev
			}
		case <-clientConn.Done():
			t.Fatal("client connection closed while waiting for sentinel — overflow")
		case <-deadline:
			t.Fatal("timeout waiting for sentinel AvailableCommands event after replay burst")
		}
	}

	if len(got.AvailableCommands) != 1 || got.AvailableCommands[0].Name != sentinelCmd {
		t.Fatalf("unexpected sentinel payload: %+v", got)
	}

	// Plan was sent during the burst and should have been captured (last
	// Plan seen during load is stashed in loadReplayPlan for re-emit).
	a.mu.RLock()
	captured := a.loadReplayPlan
	a.mu.RUnlock()
	if captured == nil {
		t.Fatal("expected loadReplayPlan to be captured during replay burst")
	}
}

// makeReplayNotification builds a representative replay notification of one
// of the suppressed kinds (AgentMessageChunk / ToolCall / Plan). Shared by
// the burst test above and the benchmarks below.
func makeReplayNotification(sessionID string, i int) acp.SessionNotification {
	var update acp.SessionUpdate
	switch i % 3 {
	case 0:
		update = acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock(fmt.Sprintf("replay chunk %d", i)),
			},
		}
	case 1:
		update = acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{
				ToolCallId: acp.ToolCallId(fmt.Sprintf("tc-%d", i)),
				Title:      "replay tool call",
			},
		}
	case 2:
		update = acp.SessionUpdate{
			Plan: &acp.SessionUpdatePlan{
				Entries: []acp.PlanEntry{
					{Content: fmt.Sprintf("plan entry %d", i), Status: "in_progress"},
				},
			},
		}
	}
	return acp.SessionNotification{
		SessionId: acp.SessionId(sessionID),
		Update:    update,
	}
}

// BenchmarkHandleACPUpdate_LoadSuppressed measures the per-notification cost
// of the suppressed-during-load fast path. This is the hot loop during
// session/load replay, and the path that previously did json.Marshal +
// LogRawEvent on every notification before being short-circuited.
func BenchmarkHandleACPUpdate_LoadSuppressed(b *testing.B) {
	a := newTestAdapter()
	b.Cleanup(func() { _ = a.Close() })
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	notes := make([]acp.SessionNotification, 256)
	for i := range notes {
		notes[i] = makeReplayNotification("bench-session", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.handleACPUpdate(notes[i%len(notes)])
	}
}

// BenchmarkHandleACPUpdate_NormalPath measures the per-notification cost of
// the non-loading path (json.Marshal + LogRawEvent + convertNotification +
// updatesCh send). Drain updatesCh in a background goroutine so we don't
// stall on the unbuffered/buffered send.
func BenchmarkHandleACPUpdate_NormalPath(b *testing.B) {
	a := newTestAdapter()
	b.Cleanup(func() { _ = a.Close() })

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-a.updatesCh:
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	notes := make([]acp.SessionNotification, 256)
	for i := range notes {
		notes[i] = makeReplayNotification("bench-session", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.handleACPUpdate(notes[i%len(notes)])
	}
}

// TestUpdateWorker_FIFOAndDrainOnClose verifies the SDK-decoupling worker:
//  1. enqueueACPUpdate hands notifications to a single worker that processes
//     them in FIFO order (preserving the SDK's serial-delivery contract);
//  2. Close cancels the worker via lifetimeCtx, workerWg.Wait returns, and
//     updatesCh is then safely closed — no goroutine leak, no panic.
//
// We don't poll the queue length directly; the contract we care about is
// "the events arrive on updatesCh in order, and Close completes".
func TestUpdateWorker_FIFOAndDrainOnClose(t *testing.T) {
	const burst = 32

	a := newTestAdapter()

	// Push a burst of AvailableCommands notifications (one-shot events that
	// land on updatesCh and carry an identifier we can assert order on).
	for i := 0; i < burst; i++ {
		a.enqueueACPUpdate(acp.SessionNotification{
			SessionId: acp.SessionId("s1"),
			Update: acp.SessionUpdate{
				AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
					AvailableCommands: []acp.AvailableCommand{
						{Name: fmt.Sprintf("cmd-%d", i)},
					},
				},
			},
		})
	}

	// Drain updatesCh until we've seen the full ordered burst.
	deadline := time.After(5 * time.Second)
	for i := 0; i < burst; i++ {
		select {
		case ev := <-a.updatesCh:
			if ev.Type != streams.EventTypeAvailableCommands {
				t.Fatalf("event %d: unexpected type %q", i, ev.Type)
			}
			if len(ev.AvailableCommands) != 1 {
				t.Fatalf("event %d: expected 1 command, got %d", i, len(ev.AvailableCommands))
			}
			want := fmt.Sprintf("cmd-%d", i)
			if got := ev.AvailableCommands[0].Name; got != want {
				t.Fatalf("event %d: out-of-order: got %q want %q (worker is supposed to be FIFO)", i, got, want)
			}
		case <-deadline:
			t.Fatalf("timeout after %d/%d events — worker stalled", i, burst)
		}
	}

	// Close must return quickly and the worker goroutine must exit.
	closed := make(chan error, 1)
	go func() { closed <- a.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return — worker likely leaked")
	}

	// Post-Close enqueues are no-ops (lifetimeCtx is cancelled). Must not
	// block, must not panic.
	done := make(chan struct{})
	go func() {
		a.enqueueACPUpdate(acp.SessionNotification{SessionId: "s1"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueueACPUpdate blocked after Close — lifetimeCtx guard missing")
	}
}
