package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	replayBurstProducerWatchdogTimeout = 20 * time.Second
	burstProducerCleanupTimeout        = time.Second
)

var (
	errReplayBurstProducerStalled            = errors.New("replay burst producer stalled")
	errReplayBurstProducerBeforeWriteStalled = errors.New("replay burst producer stalled before writer entry")
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

type burstPair struct {
	clientConn *acp.ClientSideConnection
	agentConn  *acp.AgentSideConnection

	c2aR *io.PipeReader
	c2aW *io.PipeWriter
	a2cR *io.PipeReader
	a2cW *io.PipeWriter

	agentToClientWriteEntered <-chan struct{}
	closed                    chan struct{}
	closeOnce                 sync.Once
}

func (p *burstPair) Close() {
	p.closeOnce.Do(func() {
		_ = p.c2aR.Close()
		_ = p.c2aW.Close()
		_ = p.a2cR.Close()
		_ = p.a2cW.Close()
		close(p.closed)
	})
}

// writeEntrySignalWriter reports immediately before delegating to its writer.
// When it wraps an unread io.PipeWriter, receiving the signal proves the
// producer has entered the write that will block until the pipe is closed.
type writeEntrySignalWriter struct {
	writer  io.Writer
	entered chan struct{}
	once    sync.Once
}

func (w *writeEntrySignalWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	return w.writer.Write(p)
}

// newBurstPair wires an AgentSideConnection over in-memory pipes. When
// onUpdate is supplied, it also creates a ClientSideConnection whose
// SessionUpdate forwards to that handler. Cleanup is registered on the test.
func newBurstPair(t *testing.T, onUpdate func(acp.SessionNotification)) *burstPair {
	t.Helper()

	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	a2cWriteEntered := make(chan struct{})
	a2cWriteObserver := &writeEntrySignalWriter{writer: a2cW, entered: a2cWriteEntered}

	pair := &burstPair{
		agentConn:                 acp.NewAgentSideConnection(burstAgent{}, a2cWriteObserver, c2aR),
		c2aR:                      c2aR,
		c2aW:                      c2aW,
		a2cR:                      a2cR,
		a2cW:                      a2cW,
		agentToClientWriteEntered: a2cWriteEntered,
		closed:                    make(chan struct{}),
	}
	if onUpdate != nil {
		pair.clientConn = acp.NewClientSideConnection(
			&burstClient{onUpdate: onUpdate}, c2aW, a2cR,
			acp.WithMaxQueuedNotifications(acpNotifQueueCapacity()),
		)
	}
	t.Cleanup(pair.Close)

	return pair
}

// newStalledBurstPair leaves the agent-to-client pipe unread so writes from
// AgentSideConnection block until the pair is closed.
func newStalledBurstPair(t *testing.T) *burstPair {
	return newBurstPair(t, nil)
}

// runBurstProducer ensures a stalled synchronous io.Pipe write fails the
// test promptly. acp-go-sdk's write path cannot observe context cancellation
// after entering io.Pipe.Write, so the watchdog must close this pair's pipes.
func runBurstProducer(pair *burstPair, timeout time.Duration, produce func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- produce()
	}()

	entryTimer := time.NewTimer(timeout)
	defer entryTimer.Stop()

	// JSON encoding and SDK setup happen before the synchronous io.Pipe write.
	// Bound that phase separately: it must not prevent the test from cleaning
	// up a producer which never reaches the instrumented writer.
	select {
	case <-pair.agentToClientWriteEntered:
	case err := <-done:
		return err
	case <-entryTimer.C:
		return stalledBurstProducerError(pair, timeout, done, errReplayBurstProducerBeforeWriteStalled)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		return stalledBurstProducerError(pair, timeout, done, errReplayBurstProducerStalled)
	}
}

// stalledBurstProducerError owns the failure path for both producer phases.
// It always reports the timeout that triggered cleanup, even if closing the
// pipes lets the producer return nil.
func stalledBurstProducerError(pair *burstPair, timeout time.Duration, done <-chan error, stallErr error) error {
	pair.Close()

	cleanupTimer := time.NewTimer(burstProducerCleanupTimeout)
	defer cleanupTimer.Stop()
	select {
	case err := <-done:
		return fmt.Errorf("%w after %s: %v", stallErr, timeout, err)
	case <-cleanupTimer.C:
		return fmt.Errorf("%w after %s: pipe cleanup did not release producer", stallErr, timeout)
	}
}

func TestBurstProducerWatchdog_ClosesStalledPipes(t *testing.T) {
	const watchdogTimeout = 100 * time.Millisecond

	pair := newStalledBurstPair(t)
	producerExited := make(chan struct{})
	err := runBurstProducer(pair, watchdogTimeout, func() error {
		defer close(producerExited)
		return pair.agentConn.SessionUpdate(context.Background(), makeReplayNotification("stalled-session", 1))
	})

	select {
	case <-pair.agentToClientWriteEntered:
	default:
		t.Fatal("producer did not enter the agent-to-client pipe write")
	}
	select {
	case <-producerExited:
	default:
		t.Fatal("watchdog returned before the stalled producer exited")
	}
	if !errors.Is(err, errReplayBurstProducerStalled) {
		t.Fatalf("watchdog error = %v, want stalled producer error", err)
	}
}

func TestBurstProducerWatchdog_ClosesPipesBeforeAgentToClientWrite(t *testing.T) {
	const watchdogTimeout = 100 * time.Millisecond

	pair := newStalledBurstPair(t)
	producerBlocked := make(chan struct{})
	producerExited := make(chan struct{})
	runDone := make(chan error, 1)
	go func() {
		runDone <- runBurstProducer(pair, watchdogTimeout, func() error {
			close(producerBlocked)
			<-pair.closed
			close(producerExited)
			return nil
		})
	}()

	select {
	case <-producerBlocked:
	case <-time.After(watchdogTimeout):
		t.Fatal("producer did not block before SessionUpdate")
	}

	select {
	case err := <-runDone:
		if !errors.Is(err, errReplayBurstProducerBeforeWriteStalled) {
			t.Fatalf("watchdog error = %v, want pre-write stalled producer error", err)
		}
	case <-time.After(watchdogTimeout + burstProducerCleanupTimeout + time.Second):
		t.Fatal("writer-entry watchdog did not close pipes and return")
	}

	select {
	case <-producerExited:
	default:
		t.Fatal("writer-entry watchdog returned before the pre-write producer exited")
	}
	select {
	case <-pair.closed:
	default:
		t.Fatal("writer-entry watchdog did not close the owned pipes")
	}
	select {
	case <-pair.agentToClientWriteEntered:
		t.Fatal("producer entered agent-to-client write before watchdog cleanup")
	default:
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

	pair := newBurstPair(t, a.enqueueACPUpdate)

	const sessionID = "burst-session"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Push the burst of replay notifications. These would all be suppressed
	// by the adapter (AgentMessageChunk + ToolCall + Plan are on the suppress
	// list) — the point is whether the SDK's 1024-deep queue stays drained.
	// A stalled SDK pipe write does not honor ctx; bound the producer phase and
	// close only this pair's pipes if that happens.
	if err := runBurstProducer(pair, replayBurstProducerWatchdogTimeout, func() error {
		for i := 0; i < notificationBurstSize; i++ {
			if err := pair.agentConn.SessionUpdate(ctx, makeReplayNotification(sessionID, i)); err != nil {
				return fmt.Errorf("SessionUpdate at i=%d: %w", i, err)
			}
		}

		// Sentinel: AvailableCommandsUpdate passes through during load, so it
		// will land on updatesCh once the consumer drains the queue.
		if err := pair.agentConn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: acp.SessionId(sessionID),
			Update: acp.SessionUpdate{
				AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
					AvailableCommands: []acp.AvailableCommand{
						{Name: sentinelCmd, Description: "burst sentinel"},
					},
				},
			},
		}); err != nil {
			return fmt.Errorf("sentinel SessionUpdate: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
		case <-pair.clientConn.Done():
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
