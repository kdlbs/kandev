package acp

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type guardedTTYFakeAgent struct {
	burstAgent
	mu             sync.Mutex
	capability     any
	capabilityCall int
	execCall       int
	execParams     map[string]any
	execEntered    chan struct{}
	releaseExec    chan struct{}
}

func (a *guardedTTYFakeAgent) Initialize(_ context.Context, request acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{
		ProtocolVersion: request.ProtocolVersion,
		Meta: map[string]any{
			"guardedTtyExec": map[string]any{
				"capability":       "kandev.guarded-tty-exec",
				"version":          float64(1),
				"capabilityMethod": "_kandev/guarded_tty/capability",
				"execMethod":       "_kandev/guarded_tty/exec",
			},
		},
	}, nil
}

func (a *guardedTTYFakeAgent) NewSession(_ context.Context, _ acpsdk.NewSessionRequest) (acpsdk.NewSessionResponse, error) {
	return acpsdk.NewSessionResponse{SessionId: "acp-session-1"}, nil
}

func (a *guardedTTYFakeAgent) Prompt(_ context.Context, _ acpsdk.PromptRequest) (acpsdk.PromptResponse, error) {
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
}

func (a *guardedTTYFakeAgent) HandleExtensionMethod(_ context.Context, method string, raw json.RawMessage) (any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch method {
	case "_kandev/guarded_tty/capability":
		a.capabilityCall++
		return a.capability, nil
	case "_kandev/guarded_tty/exec":
		a.execCall++
		if err := json.Unmarshal(raw, &a.execParams); err != nil {
			return nil, err
		}
		entered, release := a.execEntered, a.releaseExec
		a.mu.Unlock()
		if entered != nil {
			entered <- struct{}{}
		}
		if release != nil {
			<-release
		}
		a.mu.Lock()
		return map[string]any{
			"capability":     "kandev.guarded-tty-exec",
			"version":        1,
			"session_id":     "acp-session-1",
			"method":         "command/exec",
			"requested_tty":  true,
			"dispatched_tty": true,
			"process_id":     "process-1",
			"cwd":            "/trusted/task/worktree",
			"outcome":        "completed",
			"denial_code":    nil,
			"stdout":         "tty stdout\n",
			"stderr":         "stty ok\n",
			"stdout_bytes":   11,
			"stderr_bytes":   8,
			"output_bytes":   19,
			"exit_code":      0,
			"started_at":     "2026-08-30T10:00:00Z",
			"completed_at":   "2026-08-30T10:00:01Z",
		}, nil
	default:
		return nil, acpsdk.NewMethodNotFound(method)
	}
}

func setupGuardedTTYAdapter(t *testing.T, fake *guardedTTYFakeAgent) *Adapter {
	t.Helper()
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	adapter := newTestAdapterForAgent(codexAgentID)
	if err := adapter.Connect(clientToAgentWriter, agentToClientReader); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	_ = acpsdk.NewAgentSideConnection(fake, agentToClientWriter, clientToAgentReader)
	t.Cleanup(func() {
		_ = adapter.Close()
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})
	return adapter
}

func validGuardedTTYCapability() map[string]any {
	return map[string]any{
		"capability":        "kandev.guarded-tty-exec",
		"version":           1,
		"supported":         true,
		"capability_method": "_kandev/guarded_tty/capability",
		"exec_method":       "_kandev/guarded_tty/exec",
		"session_id":        "acp-session-1",
	}
}

func TestGuardedTTYNegotiationRequiresExactInitializeAndSessionProbe(t *testing.T) {
	fake := &guardedTTYFakeAgent{capability: validGuardedTTYCapability()}
	adapter := setupGuardedTTYAdapter(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := adapter.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if adapter.GuardedTTYAvailable() {
		t.Fatal("guarded TTY became available before an active-session probe")
	}
	if _, err := adapter.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if !adapter.GuardedTTYAvailable() {
		t.Fatal("guarded TTY unavailable after exact advertisement and probe")
	}
	if fake.capabilityCall != 1 {
		t.Fatalf("capability probes = %d, want 1", fake.capabilityCall)
	}
}

func TestGuardedTTYNegotiationFailsClosedWithoutExactProbe(t *testing.T) {
	malformed := validGuardedTTYCapability()
	malformed["exec_method"] = "_kandev/guarded_tty/v0"
	fake := &guardedTTYFakeAgent{capability: malformed}
	adapter := setupGuardedTTYAdapter(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := adapter.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := adapter.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession should survive an unsupported extension: %v", err)
	}
	if adapter.GuardedTTYAvailable() {
		t.Fatal("malformed probe enabled guarded TTY")
	}
	if _, err := adapter.ExecuteGuardedTTY(ctx, streams.GuardedTTYBridgeRequest{
		SessionID: "acp-session-1", Argv: []string{"pwd"},
	}); err == nil {
		t.Fatal("ExecuteGuardedTTY succeeded without exact negotiation")
	}
	if fake.execCall != 0 {
		t.Fatalf("extension exec calls = %d, want 0", fake.execCall)
	}
}

func TestExecuteGuardedTTYMapsExactReceiptAndAllowsConcurrentPrompt(t *testing.T) {
	fake := &guardedTTYFakeAgent{
		capability:  validGuardedTTYCapability(),
		execEntered: make(chan struct{}, 1),
		releaseExec: make(chan struct{}),
	}
	adapter := setupGuardedTTYAdapter(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := adapter.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := adapter.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	receiptCh := make(chan *streams.GuardedTTYExecReceipt, 1)
	errCh := make(chan error, 1)
	go func() {
		receipt, err := adapter.ExecuteGuardedTTY(ctx, streams.GuardedTTYBridgeRequest{
			SessionID: "acp-session-1",
			Argv:      []string{"sh", "-lc", "test -t 0 && test -t 1 && stty && pwd && git status --short"},
		})
		receiptCh <- receipt
		errCh <- err
	}()
	select {
	case <-fake.execEntered:
	case <-ctx.Done():
		t.Fatal("guarded TTY extension did not start")
	}

	promptCtx, promptCancel := context.WithTimeout(ctx, time.Second)
	defer promptCancel()
	if err := adapter.Prompt(promptCtx, "non-TTY remains prompt", nil, 1); err != nil {
		t.Fatalf("concurrent non-TTY prompt: %v", err)
	}
	close(fake.releaseExec)
	if err := <-errCh; err != nil {
		t.Fatalf("ExecuteGuardedTTY: %v", err)
	}
	receipt := <-receiptCh
	if receipt == nil || receipt.ACPSessionID != "acp-session-1" || receipt.Method != "command/exec" ||
		!receipt.RequestedTTY || !receipt.DispatchedTTY || receipt.CWD != "/trusted/task/worktree" ||
		receipt.Outcome != "succeeded" || receipt.ExitCode != 0 || receipt.CompletionCount != 1 ||
		receipt.Stdout != "tty stdout\n" || receipt.Stderr != "stty ok\n" || receipt.OutputBytes != 19 {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if got := fake.execParams["sessionId"]; got != "acp-session-1" {
		t.Fatalf("extension sessionId = %v", got)
	}
	if len(fake.execParams) != 2 {
		t.Fatalf("extension request fields = %v, want only sessionId and argv", fake.execParams)
	}
}

func TestMapGuardedTTYReceiptRejectsMalformedEvidenceAndMapsDenials(t *testing.T) {
	startedAt := "2026-08-30T10:00:00Z"
	completedAt := "2026-08-30T10:00:01Z"
	processID := "process-1"
	cwd := "/trusted/task/worktree"
	tests := []struct {
		code       string
		dispatched bool
	}{
		{code: "stale_session"},
		{code: "timeout", dispatched: true},
		{code: "cancelled", dispatched: true},
		{code: "output_overflow", dispatched: true},
		{code: "invalid_output", dispatched: true},
		{code: "app_server_error", dispatched: true},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			code := tt.code
			wire := guardedTTYWireReceipt{
				Capability: guardedTTYCapabilityName, Version: guardedTTYContractVersion,
				SessionID: "acp-session-1", Method: streams.GuardedTTYExecMethod,
				RequestedTTY: true, DispatchedTTY: tt.dispatched, Outcome: "failed",
				DenialCode: &code, ProcessID: &processID, CWD: &cwd,
				StartedAt: startedAt, CompletedAt: completedAt,
			}
			if !tt.dispatched {
				wire.Outcome = "denied"
				wire.ProcessID = nil
				wire.CWD = nil
			}
			receipt, err := mapGuardedTTYReceipt(wire, "1.7.0")
			if err != nil || receipt == nil || receipt.Outcome != tt.code ||
				receipt.DispatchedTTY != tt.dispatched || receipt.CompletionCount != 1 {
				t.Fatalf("mapGuardedTTYReceipt() = %+v, %v", receipt, err)
			}
		})
	}

	exitCode := 0
	invalid := guardedTTYWireReceipt{
		Capability: guardedTTYCapabilityName, Version: guardedTTYContractVersion,
		SessionID: "acp-session-1", Method: streams.GuardedTTYExecMethod,
		RequestedTTY: true, DispatchedTTY: true, Outcome: "completed", ExitCode: &exitCode,
		Stdout: "secret-sized-wrong", StdoutBytes: 1, OutputBytes: 1,
		StartedAt: startedAt, CompletedAt: completedAt,
	}
	if receipt, err := mapGuardedTTYReceipt(invalid, "1.7.0"); err == nil || receipt != nil {
		t.Fatalf("malformed receipt accepted: %+v, %v", receipt, err)
	}
}
