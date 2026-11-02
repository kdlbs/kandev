package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// promptErrorContinuityAgent fails the first len(failures) prompts with a real
// JSON-RPC error response and then answers normally, all on one connection.
type promptErrorContinuityAgent struct {
	burstAgent
	conn       *sdk.AgentSideConnection
	failures   []error
	promptRuns atomic.Int32
	newRuns    atomic.Int32
	emitQuota  bool
}

func (f *promptErrorContinuityAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	f.newRuns.Add(1)
	return sdk.NewSessionResponse{SessionId: "continuity-session"}, nil
}

func (f *promptErrorContinuityAgent) Prompt(ctx context.Context, _ sdk.PromptRequest) (sdk.PromptResponse, error) {
	run := int(f.promptRuns.Add(1))
	if run <= len(f.failures) {
		if f.emitQuota {
			f.emitQuotaEvidence(ctx)
		}
		return sdk.PromptResponse{}, f.failures[run-1]
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func (f *promptErrorContinuityAgent) emitQuotaEvidence(ctx context.Context) {
	emitCodexQuotaEvidence(ctx, f.conn)
}

func emitCodexQuotaEvidence(ctx context.Context, conn *sdk.AgentSideConnection) {
	for _, raw := range []string{
		`{"sessionId":"continuity-session","update":{"sessionUpdate":"session_info_update","_meta":{"threadStatus":{"type":"systemError"}}}}`,
		`{"sessionId":"continuity-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"insufficient_quota for project"}}}`,
	} {
		var notification sdk.SessionNotification
		if err := json.Unmarshal([]byte(raw), &notification); err != nil {
			panic(err)
		}
		if err := conn.SessionUpdate(ctx, notification); err != nil {
			panic(err)
		}
	}
}

func newPromptErrorContinuityFixture(t *testing.T, agentID string, fake *promptErrorContinuityAgent) *Adapter {
	t.Helper()

	a := newTestAdapterForAgent(agentID)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	fake.conn = sdk.NewAgentSideConnection(fake, agentToClientW, clientToAgentR)
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize adapter: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return a
}

func genericPromptError(message string) error {
	return &sdk.RequestError{
		Code:    -32603,
		Message: message,
		Data:    map[string]any{"details": "SQLiteError: database is locked"},
	}
}

func TestPromptErrorRetainsACPConnectionForFollowUp(t *testing.T) {
	providers := []string{
		claudeAgentID,
		codexAgentID,
		opencodeAgentID,
		acpcompat.CursorAgentID,
		"gemini-acp",
		"custom-acp",
	}
	messages := []string{
		"Internal error: Failed to execute statement",
		"Internal error",
		"Authentication required",
		"invalid model id",
		"",
	}

	for _, provider := range providers {
		for _, message := range messages {
			name := fmt.Sprintf("%s/%q", provider, message)
			t.Run(name, func(t *testing.T) {
				fake := &promptErrorContinuityAgent{failures: []error{genericPromptError(message)}}
				a := newPromptErrorContinuityFixture(t, provider, fake)
				connection := a.acpConn
				sessionID := a.sessionID
				promptGeneration := uint64(17)

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				if err := a.Prompt(ctx, "first prompt", nil, promptGeneration); err != nil {
					t.Fatalf("prompt returned error instead of a retained failure: %v", err)
				}
				failure := waitForEventType(t, a, streams.EventTypeError)
				if failure.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
					t.Fatalf("failure disposition = %q, want retain_runtime", failure.PromptFailureDisposition)
				}
				if failure.PromptGeneration != promptGeneration {
					t.Fatalf("failure generation = %d, want %d", failure.PromptGeneration, promptGeneration)
				}
				if failure.ProviderError == nil || !failure.ProviderError.Valid() {
					t.Fatalf("failure provider error = %+v, want a valid diagnostic", failure.ProviderError)
				}
				if failure.Error == "" {
					t.Fatal("retained failure has no sanitized message")
				}
				if failure.Error == genericPromptError(message).Error() {
					t.Fatalf("retained failure leaked the raw SDK error: %q", failure.Error)
				}

				if err := a.Prompt(ctx, "follow-up prompt", nil, promptGeneration+1); err != nil {
					t.Fatalf("follow-up prompt failed: %v", err)
				}
				if got := waitForEventType(t, a, streams.EventTypeComplete).PromptGeneration; got != promptGeneration+1 {
					t.Fatalf("follow-up generation = %d, want %d", got, promptGeneration+1)
				}
				if a.acpConn != connection {
					t.Fatal("follow-up prompt used a different ACP connection")
				}
				if a.sessionID != sessionID {
					t.Fatalf("session identity changed: %q -> %q", sessionID, a.sessionID)
				}
				if got := fake.promptRuns.Load(); got != 2 {
					t.Fatalf("provider prompt calls = %d, want 2", got)
				}
				if got := fake.newRuns.Load(); got != 1 {
					t.Fatalf("ACP sessions created = %d, want 1", got)
				}
			})
		}
	}
}

func TestPromptErrorRetainsAfterRepeatedFailures(t *testing.T) {
	fake := &promptErrorContinuityAgent{failures: []error{
		genericPromptError("Internal error: Failed to execute statement"),
		genericPromptError("Internal error: Failed to execute statement"),
	}}
	a := newPromptErrorContinuityFixture(t, opencodeAgentID, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for generation := uint64(1); generation <= 2; generation++ {
		if err := a.Prompt(ctx, "failing prompt", nil, generation); err != nil {
			t.Fatalf("prompt %d returned error: %v", generation, err)
		}
		failure := waitForEventType(t, a, streams.EventTypeError)
		if failure.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
			t.Fatalf("prompt %d disposition = %q", generation, failure.PromptFailureDisposition)
		}
	}

	if err := a.Prompt(ctx, "recovering prompt", nil, 3); err != nil {
		t.Fatalf("recovering prompt failed: %v", err)
	}
	if got := waitForEventType(t, a, streams.EventTypeComplete).PromptGeneration; got != 3 {
		t.Fatalf("recovering generation = %d, want 3", got)
	}
}

func TestPromptErrorPreservesCodexQuotaDiagnostic(t *testing.T) {
	fake := &promptErrorContinuityAgent{
		failures:  []error{genericPromptError("Internal error")},
		emitQuota: true,
	}
	a := newPromptErrorContinuityFixture(t, codexAgentID, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.Prompt(ctx, "quota prompt", nil, 9); err != nil {
		t.Fatalf("prompt returned error instead of a retained failure: %v", err)
	}
	failure := waitForEventType(t, a, streams.EventTypeError)
	if failure.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("failure disposition = %q, want retain_runtime", failure.PromptFailureDisposition)
	}
	if failure.ProviderError == nil || failure.ProviderError.Source != streams.ProviderErrorSourceCodexACP {
		t.Fatalf("provider error = %+v, want the correlated Codex quota diagnostic", failure.ProviderError)
	}
	if failure.Error != failure.ProviderError.Message || failure.Error == "" {
		t.Fatalf("failure message = %q, provider message = %q", failure.Error, failure.ProviderError.Message)
	}

	if err := a.Prompt(ctx, "follow-up prompt", nil, 10); err != nil {
		t.Fatalf("follow-up prompt failed: %v", err)
	}
	waitForEventType(t, a, streams.EventTypeComplete)
}

func TestLocalPromptFailureDoesNotRetain(t *testing.T) {
	a := newTestAdapterForAgent(claudeAgentID)
	for _, err := range []error{
		sdk.NewInternalError(map[string]any{"details": "local"}),
		sdk.NewRequestCancelled(nil),
		sdk.NewAuthRequired(nil),
		errors.New("boom"),
	} {
		if providerError, ok := a.retainedACPApplicationError(err); ok {
			t.Fatalf("local failure %v produced retained evidence %+v", err, providerError)
		}
	}
}

// codexQuotaThenWaitAgent streams quota evidence and then holds the prompt open
// until the client's context deadline. The resulting failure is local, not a
// response from the peer.
type codexQuotaThenWaitAgent struct {
	burstAgent
	conn *sdk.AgentSideConnection
}

func (a *codexQuotaThenWaitAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "quota-deadline-session"}, nil
}

func (a *codexQuotaThenWaitAgent) Prompt(ctx context.Context, _ sdk.PromptRequest) (sdk.PromptResponse, error) {
	emitCodexQuotaEvidence(ctx, a.conn)
	<-ctx.Done()
	// Give the client time to observe its local deadline before the agent side
	// tries to answer a request the client already abandoned.
	time.Sleep(2 * time.Second)
	return sdk.PromptResponse{}, ctx.Err()
}

func newSingleAgentFixture(t *testing.T, agentID string, agent sdk.Agent, setConn func(*sdk.AgentSideConnection)) *Adapter {
	t.Helper()
	a := newTestAdapterForAgent(agentID)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	conn := sdk.NewAgentSideConnection(agent, agentToClientW, clientToAgentR)
	if setConn != nil {
		setConn(conn)
	}
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize adapter: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return a
}

func TestCodexQuotaEvidenceWithLocalDeadlineDoesNotRetain(t *testing.T) {
	fake := &codexQuotaThenWaitAgent{}
	a := newSingleAgentFixture(t, codexAgentID, fake, func(conn *sdk.AgentSideConnection) { fake.conn = conn })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Prompt(ctx, "quota with local deadline", nil, 21) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		turn := a.currentPromptTurn()
		if turn != nil {
			if _, ok := turn.codexUsageLimitFailure(); ok {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	var promptErr error
	select {
	case promptErr = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not settle after its local deadline")
	}
	var providerErr *providerPromptError
	if !errors.As(promptErr, &providerErr) {
		t.Fatalf("prompt error = %v, want the conservative quota diagnostic wrapper", promptErr)
	}
	if !strings.Contains(strings.ToLower(providerErr.ProviderError.Message), "quota") {
		t.Fatalf("provider diagnostic = %q, want the quota guidance", providerErr.ProviderError.Message)
	}
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeError && event.PromptFailureDisposition == streams.PromptFailureDispositionRetainRuntime {
			t.Fatalf("local deadline retained the runtime: %+v", event)
		}
	}
}
