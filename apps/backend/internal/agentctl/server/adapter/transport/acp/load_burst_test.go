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

func (burstAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
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

	clientConn, agentConn := newBurstPair(t, a.handleACPUpdate)

	const sessionID = "burst-session"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Push the burst of replay notifications. These would all be suppressed
	// by the adapter (AgentMessageChunk + ToolCall + Plan are on the suppress
	// list) — the point is whether the SDK's 1024-deep queue stays drained.
	for i := 0; i < notificationBurstSize; i++ {
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
		if err := agentConn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: acp.SessionId(sessionID),
			Update:    update,
		}); err != nil {
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

	// The connection must NOT have been closed by the SDK's overflow path.
	select {
	case <-clientConn.Done():
		t.Fatal("client connection closed during replay burst — likely notification queue overflow")
	case <-agentConn.Done():
		t.Fatal("agent connection closed during replay burst — likely notification queue overflow")
	default:
	}

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
