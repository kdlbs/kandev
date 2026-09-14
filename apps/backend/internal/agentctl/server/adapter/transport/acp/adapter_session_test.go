package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types"
)

type sessionRequestCaptureAgent struct {
	newRequest   acpsdk.NewSessionRequest
	loadRequest  acpsdk.LoadSessionRequest
	newStarted   chan struct{}
	releaseNew   chan struct{}
	closeStarted chan struct{}
	releaseClose chan struct{}
	loadStarted  chan struct{}

	mu                      sync.Mutex
	sessionCounter          int
	failNewSessionOnAttempt int
	closeRequests           []acpsdk.CloseSessionRequest
	closeErr                error
}

// recordedCloseRequests returns a snapshot of every session/close request the
// fake received, in call order.
func (a *sessionRequestCaptureAgent) recordedCloseRequests() []acpsdk.CloseSessionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]acpsdk.CloseSessionRequest, len(a.closeRequests))
	copy(out, a.closeRequests)
	return out
}

var (
	_ acpsdk.Agent       = (*sessionRequestCaptureAgent)(nil)
	_ acpsdk.AgentLoader = (*sessionRequestCaptureAgent)(nil)
)

func (*sessionRequestCaptureAgent) Authenticate(context.Context, acpsdk.AuthenticateRequest) (acpsdk.AuthenticateResponse, error) {
	return acpsdk.AuthenticateResponse{}, nil
}

func (*sessionRequestCaptureAgent) Initialize(context.Context, acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{}, nil
}

func (*sessionRequestCaptureAgent) Logout(context.Context, acpsdk.LogoutRequest) (acpsdk.LogoutResponse, error) {
	return acpsdk.LogoutResponse{}, nil
}

func (*sessionRequestCaptureAgent) Cancel(context.Context, acpsdk.CancelNotification) error {
	return nil
}

func (a *sessionRequestCaptureAgent) CloseSession(_ context.Context, req acpsdk.CloseSessionRequest) (acpsdk.CloseSessionResponse, error) {
	if a.closeStarted != nil {
		close(a.closeStarted)
	}
	if a.releaseClose != nil {
		<-a.releaseClose
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeRequests = append(a.closeRequests, req)
	return acpsdk.CloseSessionResponse{}, a.closeErr
}

func (*sessionRequestCaptureAgent) DeleteSession(context.Context, acpsdk.DeleteSessionRequest) (acpsdk.DeleteSessionResponse, error) {
	return acpsdk.DeleteSessionResponse{}, nil
}

func (*sessionRequestCaptureAgent) ListSessions(context.Context, acpsdk.ListSessionsRequest) (acpsdk.ListSessionsResponse, error) {
	return acpsdk.ListSessionsResponse{}, nil
}

func (a *sessionRequestCaptureAgent) NewSession(_ context.Context, request acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	a.newRequest = request
	if a.newStarted != nil {
		close(a.newStarted)
		<-a.releaseNew
	}
	a.mu.Lock()
	attempt := a.sessionCounter + 1
	if a.failNewSessionOnAttempt == attempt {
		a.mu.Unlock()
		return acpsdk.NewSessionResponse{}, errors.New("session/new failed")
	}
	a.sessionCounter = attempt
	sessionID := fmt.Sprintf("session-%d", attempt)
	a.mu.Unlock()
	return acpsdk.NewSessionResponse{SessionId: acpsdk.SessionId(sessionID)}, nil
}

func (*sessionRequestCaptureAgent) Prompt(context.Context, acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (*sessionRequestCaptureAgent) ResumeSession(context.Context, acpsdk.ResumeSessionRequest) (acpsdk.ResumeSessionResponse, error) {
	return acpsdk.ResumeSessionResponse{}, nil
}

func (*sessionRequestCaptureAgent) SetSessionConfigOption(context.Context, acpsdk.SetSessionConfigOptionRequest) (acpsdk.SetSessionConfigOptionResponse, error) {
	return acpsdk.SetSessionConfigOptionResponse{}, nil
}

func (*sessionRequestCaptureAgent) SetSessionMode(context.Context, acpsdk.SetSessionModeRequest) (acpsdk.SetSessionModeResponse, error) {
	return acpsdk.SetSessionModeResponse{}, nil
}

func (a *sessionRequestCaptureAgent) LoadSession(_ context.Context, request acpsdk.LoadSessionRequest) (acpsdk.LoadSessionResponse, error) {
	a.loadRequest = request
	if a.loadStarted != nil {
		close(a.loadStarted)
	}
	return acpsdk.LoadSessionResponse{}, nil
}

func TestMCPSessionNewAndLoadUseHTTPWithSSEFallback(t *testing.T) {
	tests := []struct {
		name         string
		capabilities acpsdk.McpCapabilities
		wantType     string
	}{
		{name: "HTTP preferred", capabilities: acpsdk.McpCapabilities{Http: true, Sse: true}, wantType: "http"},
		{name: "SSE fallback", capabilities: acpsdk.McpCapabilities{Sse: true}, wantType: "sse"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, capture := newSessionRequestCaptureAdapter(t, tt.capabilities)
			servers := []types.McpServer{
				{Name: "kandev", Type: "http", URL: "http://localhost:10005/mcp"},
				{Name: "kandev", Type: "sse", URL: "http://localhost:10005/sse"},
			}

			if _, err := adapter.NewSession(context.Background(), servers); err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			assertCapturedKandevTransport(t, capture.newRequest.McpServers, tt.wantType)

			if err := adapter.LoadSession(context.Background(), "session-1", servers); err != nil {
				t.Fatalf("LoadSession: %v", err)
			}
			assertCapturedKandevTransport(t, capture.loadRequest.McpServers, tt.wantType)
		})
	}
}

func TestResetSessionInvalidatesPromptOwnership(t *testing.T) {
	adapter, _ := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})

	_, owner := newPromptTurnState(context.Background(), 1, false)
	if err := adapter.acquirePromptTurn(context.Background(), owner, true); err != nil {
		t.Fatalf("acquire original prompt: %v", err)
	}
	adapter.toolCallParents["child-1"] = "parent-1"
	adapter.handoffProtectedToolCalls["parent-1"] = struct{}{}

	type acquireResult struct {
		turn *promptTurnState
		err  error
	}
	waiterDone := make(chan acquireResult, 1)
	go func() {
		_, waiter := newPromptTurnState(context.Background(), 0, false)
		err := adapter.acquirePromptTurn(context.Background(), waiter, false)
		waiterDone <- acquireResult{turn: waiter, err: err}
	}()

	if _, err := adapter.ResetSession(context.Background(), nil); err != nil {
		t.Fatalf("ResetSession: %v", err)
	}

	var successor *promptTurnState
	select {
	case result := <-waiterDone:
		if result.err != nil {
			t.Fatalf("queued prompt did not acquire after reset: %v", result.err)
		}
		successor = result.turn
	case <-time.After(time.Second):
		t.Fatal("reset leaked the queued prompt ownership waiter")
	}
	defer adapter.finishPromptTurn(successor)

	if adapter.claimPromptTurnCompletion(owner) {
		t.Fatal("stale pre-reset prompt retained response ownership")
	}
	adapter.finishPromptTurn(owner)
	if len(adapter.toolCallParents) != 0 || len(adapter.handoffProtectedToolCalls) != 0 {
		t.Fatal("reset retained stale prompt handoff tool ownership")
	}
}

func TestResetSessionCannotInvalidateInheritedPromptOwnership(t *testing.T) {
	adapter, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	capture.newStarted = make(chan struct{})
	capture.releaseNew = make(chan struct{})

	_, owner := newPromptTurnState(context.Background(), 1, true)
	if err := adapter.acquirePromptTurn(context.Background(), owner, true); err != nil {
		t.Fatalf("acquire original prompt: %v", err)
	}
	if !adapter.markPromptHandoff("session-1", 1) {
		t.Fatal("original prompt did not accept handoff")
	}

	resetDone := make(chan error, 1)
	go func() {
		_, err := adapter.ResetSession(context.Background(), nil)
		resetDone <- err
	}()
	select {
	case <-capture.newStarted:
	case <-time.After(time.Second):
		t.Fatal("reset did not reach provider session creation")
	}

	_, successor := newPromptTurnState(context.Background(), 2, true)
	if err := adapter.acquirePromptTurn(context.Background(), successor, true); err != nil {
		t.Fatalf("successor did not inherit prompt ownership: %v", err)
	}
	close(capture.releaseNew)
	select {
	case err := <-resetDone:
		if err != nil {
			t.Fatalf("ResetSession: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reset did not complete")
	}

	if !adapter.claimPromptTurnCompletion(successor) {
		t.Fatal("reset invalidated the inherited successor prompt")
	}
	if adapter.claimPromptTurnCompletion(owner) {
		t.Fatal("transferred predecessor reclaimed completion ownership")
	}
	adapter.finishPromptTurn(owner)
	adapter.finishPromptTurn(successor)
}

func TestResetSessionClosesSupersededSessionWhenAdvertised(t *testing.T) {
	adapter, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	adapter.capabilities.SessionCapabilities.Close = &acpsdk.SessionCloseCapabilities{}

	firstID, err := adapter.NewSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	secondID, err := adapter.ResetSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	if secondID == firstID {
		t.Fatalf("ResetSession returned the same session id %q twice", secondID)
	}

	closes := capture.recordedCloseRequests()
	if len(closes) != 1 {
		t.Fatalf("CloseSession called %d times, want 1", len(closes))
	}
	if string(closes[0].SessionId) != firstID {
		t.Fatalf("closed session id = %q, want the superseded id %q", closes[0].SessionId, firstID)
	}
}

// TestResetSessionSkipsCloseWhenSessionChangedConcurrently pins the guard
// against the interleaving where agentctl's per-request WS dispatch (each
// request runs in its own unserialized goroutine) lets a concurrent
// LoadSession make the superseded session live again before the reset's
// close check runs. It replays that exact sequence of adapter-visible state
// transitions deterministically instead of racing goroutines against an
// unobservable two-statement gap in ResetSession, then invokes
// closeSupersededSession with the stale (previous, newID) pair a real reset
// would have captured.
func TestResetSessionSkipsCloseWhenSessionChangedConcurrently(t *testing.T) {
	adapter, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	adapter.capabilities.SessionCapabilities.Close = &acpsdk.SessionCloseCapabilities{}

	firstID, err := adapter.NewSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Reset's session/new succeeds, producing the new session that should
	// become current.
	secondID, err := adapter.NewSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewSession (reset): %v", err)
	}

	// A concurrent request on another goroutine loads the superseded session
	// back in, making it current again before the reset's close check runs.
	if err := adapter.LoadSession(context.Background(), firstID, nil); err != nil {
		t.Fatalf("LoadSession (concurrent): %v", err)
	}

	// The reset now runs its close check against the (previous, newID) pair
	// it captured before either of the above happened.
	adapter.closeSupersededSession(context.Background(), adapter.acpConn, firstID, secondID)

	if closes := capture.recordedCloseRequests(); len(closes) != 0 {
		t.Fatalf("closeSupersededSession closed %v even though session %q is current again, would have killed a live session", closes, firstID)
	}
}

func TestResetSessionSkipsCloseWithoutCapability(t *testing.T) {
	adapter, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	// newSessionRequestCaptureAdapter does not set SessionCapabilities.Close.

	if _, err := adapter.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := adapter.ResetSession(context.Background(), nil); err != nil {
		t.Fatalf("ResetSession: %v", err)
	}

	if closes := capture.recordedCloseRequests(); len(closes) != 0 {
		t.Fatalf("CloseSession called %d times, want 0 when the agent omits the close capability", len(closes))
	}
}

func TestResetSessionSkipsCloseWhenNewSessionFails(t *testing.T) {
	adapter, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	adapter.capabilities.SessionCapabilities.Close = &acpsdk.SessionCloseCapabilities{}

	if _, err := adapter.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	capture.failNewSessionOnAttempt = 2
	if _, err := adapter.ResetSession(context.Background(), nil); err == nil {
		t.Fatal("ResetSession succeeded despite a failing session/new")
	}

	if closes := capture.recordedCloseRequests(); len(closes) != 0 {
		t.Fatalf("CloseSession called %d times, want 0 after a failed reset (live session must stay intact)", len(closes))
	}
}

func TestResetSessionSucceedsWhenCloseSessionErrors(t *testing.T) {
	adapter, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	adapter.capabilities.SessionCapabilities.Close = &acpsdk.SessionCloseCapabilities{}
	capture.closeErr = errors.New("close failed")

	firstID, err := adapter.NewSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	newID, err := adapter.ResetSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("ResetSession returned an error despite CloseSession failing: %v", err)
	}
	if newID == "" {
		t.Fatal("ResetSession returned an empty session id")
	}
	closes := capture.recordedCloseRequests()
	if len(closes) != 1 {
		t.Fatalf("CloseSession called %d times, want 1", len(closes))
	}
	if string(closes[0].SessionId) != firstID {
		t.Fatalf("closed session id = %q, want the superseded id %q", closes[0].SessionId, firstID)
	}
}

func TestResetSessionSerializesConcurrentLoadDuringClose(t *testing.T) {
	adapter, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	adapter.capabilities.SessionCapabilities.Close = &acpsdk.SessionCloseCapabilities{}

	firstID, err := adapter.NewSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	capture.closeStarted = make(chan struct{})
	capture.releaseClose = make(chan struct{})
	capture.loadStarted = make(chan struct{})
	resetDone := make(chan error, 1)
	go func() {
		_, err := adapter.ResetSession(context.Background(), nil)
		resetDone <- err
	}()

	select {
	case <-capture.closeStarted:
	case <-time.After(time.Second):
		t.Fatal("reset did not reach session/close")
	}

	loadDone := make(chan error, 1)
	go func() {
		loadDone <- adapter.LoadSession(context.Background(), firstID, nil)
	}()

	select {
	case <-capture.loadStarted:
		t.Fatal("LoadSession reached the provider while reset cleanup was in flight")
	case <-time.After(time.Second):
	}

	close(capture.releaseClose)
	select {
	case err := <-resetDone:
		if err != nil {
			t.Fatalf("ResetSession: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reset did not complete")
	}
	select {
	case <-capture.loadStarted:
	case <-time.After(time.Second):
		t.Fatal("LoadSession did not reach the provider after reset cleanup")
	}
	select {
	case err := <-loadDone:
		if err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoadSession did not complete after reset cleanup")
	}
}

func newSessionRequestCaptureAdapter(t *testing.T, capabilities acpsdk.McpCapabilities) (*Adapter, *sessionRequestCaptureAgent) {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	capture := &sessionRequestCaptureAgent{}
	clientConnection := acpsdk.NewClientSideConnection(acpclient.NewClient(), clientToAgentWriter, agentToClientReader)
	_ = acpsdk.NewAgentSideConnection(capture, agentToClientWriter, clientToAgentReader)

	t.Cleanup(func() {
		_ = clientToAgentWriter.Close()
		_ = clientToAgentReader.Close()
		_ = agentToClientWriter.Close()
		_ = agentToClientReader.Close()
	})

	adapter := newTestAdapter()
	t.Cleanup(func() { _ = adapter.Close() })
	adapter.acpConn = clientConnection
	adapter.capabilities = acpsdk.AgentCapabilities{
		LoadSession:     true,
		McpCapabilities: capabilities,
	}
	return adapter, capture
}

func assertCapturedKandevTransport(t *testing.T, servers []acpsdk.McpServer, wantType string) {
	t.Helper()
	if len(servers) != 1 {
		t.Fatalf("captured MCP servers = %+v, want one deduplicated kandev server", servers)
	}
	switch wantType {
	case "http":
		if servers[0].Http == nil || servers[0].Http.Name != "kandev" || servers[0].Http.Url != "http://localhost:10005/mcp" {
			t.Fatalf("captured MCP server = %+v, want kandev HTTP", servers[0])
		}
	case "sse":
		if servers[0].Sse == nil || servers[0].Sse.Name != "kandev" || servers[0].Sse.Url != "http://localhost:10005/sse" {
			t.Fatalf("captured MCP server = %+v, want kandev SSE", servers[0])
		}
	default:
		t.Fatalf("unknown transport %q", wantType)
	}
}
