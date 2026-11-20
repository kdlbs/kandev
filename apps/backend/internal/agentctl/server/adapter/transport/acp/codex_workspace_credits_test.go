package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/replayfixtures"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const codexWorkspaceCreditsNotice = "Your workspace is out of credits. Add credits to continue."

func replayCodexCreditsFixture(t *testing.T, frames ...replayfixtures.Frame) (*Adapter, []AgentEvent) {
	t.Helper()
	fixture := replayfixtures.Fixture{
		AgentID: codexAgentID,
		Identity: replayfixtures.Identity{
			SessionID:        "session-credits",
			ExecutionID:      "execution-credits",
			PromptGeneration: 1,
		},
		Frames: frames,
	}
	adapter, events, _, promptErr := replayFixtureThroughAdapter(t, fixture)
	if promptErr != nil {
		t.Fatalf("Adapter.Prompt error = %v, want a retained turn event", promptErr)
	}
	return adapter, events
}

func TestCodexWorkspaceCreditsProjectsMatchingGenericPromptError(t *testing.T) {
	_, events := replayCodexCreditsFixture(t,
		replayfixtures.Frame{Kind: replayfixtures.FrameMessageChunk, Role: "assistant", Text: codexWorkspaceCreditsNotice},
		replayfixtures.Frame{Kind: replayfixtures.FramePromptError, Code: -32603, Message: "Internal error"},
	)
	tokens := tokenizeEvents(events)
	if len(tokens) != 2 || tokens[0] != "message_chunk:diagnostic" || tokens[1] != "error" {
		t.Fatalf("events = %v, want the marked credit diagnostic before the retained error", tokens)
	}
	terminal := lastErrorEvent(t, events)
	if terminal.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("disposition = %q, want retain_runtime", terminal.PromptFailureDisposition)
	}
	providerErr := terminal.ProviderError
	if providerErr == nil {
		t.Fatal("ProviderErrorFromError() = nil, want the correlated credit notice")
	}
	if providerErr.Source != streams.ProviderErrorSourceCodexACP || providerErr.ProviderID != codexAgentID {
		t.Fatalf("provider error identity = %+v, want Codex ACP", providerErr)
	}
	if providerErr.RPCCode != -32603 {
		t.Fatalf("provider error RPC code = %d, want -32603", providerErr.RPCCode)
	}
	if !strings.Contains(strings.ToLower(providerErr.Message), "workspace is out of credits") {
		t.Fatalf("provider error message = %q, want the credit notice", providerErr.Message)
	}
	if providerErr.ResetAt != nil {
		t.Fatalf("provider error ResetAt = %v, want nil", providerErr.ResetAt)
	}
	if terminal.Error != providerErr.Message {
		t.Fatalf("retained error text = %q, want %q", terminal.Error, providerErr.Message)
	}
}

func TestCodexWorkspaceCreditsPreservesPromptIsolation(t *testing.T) {
	t.Run("absent notice keeps the bare generic error", func(t *testing.T) {
		_, events := replayCodexCreditsFixture(t,
			replayfixtures.Frame{Kind: replayfixtures.FramePromptError, Code: -32603, Message: "Internal error"},
		)
		terminal := lastErrorEvent(t, events)
		if terminal.ProviderError == nil || terminal.ProviderError.Source != streams.ProviderErrorSourceACPPrompt {
			t.Fatalf("retained provider error = %+v, want the ordinary ACP projection", terminal.ProviderError)
		}
		if strings.Contains(strings.ToLower(terminal.ProviderError.Message), "credits") {
			t.Fatalf("bare generic error acquired the credit notice: %q", terminal.ProviderError.Message)
		}
	})

	t.Run("specific terminal error keeps ordinary handling", func(t *testing.T) {
		_, events := replayCodexCreditsFixture(t,
			replayfixtures.Frame{Kind: replayfixtures.FrameMessageChunk, Role: "assistant", Text: codexWorkspaceCreditsNotice},
			replayfixtures.Frame{Kind: replayfixtures.FramePromptError, Code: -32603, Message: "Internal error: connection reset"},
		)
		terminal := lastErrorEvent(t, events)
		if terminal.ProviderError == nil || terminal.ProviderError.Source != streams.ProviderErrorSourceACPPrompt {
			t.Fatalf("retained provider error = %+v, want the ordinary ACP projection", terminal.ProviderError)
		}
		if strings.Contains(strings.ToLower(terminal.ProviderError.Message), "credits") {
			t.Fatalf("specific terminal error was replaced by the credit notice: %q", terminal.ProviderError.Message)
		}
	})

	t.Run("stale prompt generation is ignored", func(t *testing.T) {
		a := newTestAdapterForAgent(codexAgentID)
		t.Cleanup(func() { _ = a.Close() })
		_, turn := a.registerPromptTurn(context.Background(), 7)
		t.Cleanup(func() { a.clearPromptTurn(turn) })

		stale := &AgentEvent{
			Type: streams.EventTypeMessageChunk, Text: codexWorkspaceCreditsNotice,
			ProviderDiagnosticCandidate: true,
		}
		if a.observeCodexProviderEvidence(8, stale) {
			t.Fatal("stale prompt generation was accepted as credit evidence")
		}
		if _, ok := turn.codexUsageLimitFailure(); ok {
			t.Fatal("stale generation stored a credit notice on the active prompt")
		}
		a.observeCodexProviderEvidence(7, stale)
		if _, ok := turn.codexUsageLimitFailure(); !ok {
			t.Fatal("current prompt did not store the credit notice")
		}
	})

	t.Run("user and fenced chunks never become credit evidence", func(t *testing.T) {
		a := newTestAdapterForAgent(codexAgentID)
		if ev := a.convertMessageChunk("session-1", sdk.TextBlock(codexWorkspaceCreditsNotice), "user"); ev == nil || ev.ProviderDiagnosticCandidate {
			t.Fatalf("user chunk marker = %+v, want none", ev)
		}
		fenced := "```text\n" + codexWorkspaceCreditsNotice + "\n```"
		if ev := a.convertMessageChunk("session-1", sdk.TextBlock(fenced), "assistant"); ev == nil || ev.ProviderDiagnosticCandidate {
			t.Fatalf("fenced chunk marker = %+v, want none", ev)
		}
		if ev := a.convertMessageChunk("session-1", sdk.TextBlock("The child said "+codexWorkspaceCreditsNotice), "assistant"); ev == nil || ev.ProviderDiagnosticCandidate {
			t.Fatalf("mid-sentence chunk marker = %+v, want none", ev)
		}
		if ev := a.convertMessageChunk("session-1", sdk.TextBlock(codexWorkspaceCreditsNotice), "assistant"); ev == nil || !ev.ProviderDiagnosticCandidate {
			t.Fatalf("assistant diagnostic marker = %+v, want set", ev)
		}
	})

	t.Run("cancellation is not a generic credit failure", func(t *testing.T) {
		if isGenericCodexPromptError(errPromptAbandonedAfterCancel) {
			t.Fatal("cancellation matched the generic Codex prompt error predicate")
		}
		if !isGenericCodexPromptError(&sdk.RequestError{Code: -32603, Message: "Internal error"}) {
			t.Fatal("the exact generic Codex prompt error did not match")
		}
	})

	t.Run("successful completion emits no credit failure", func(t *testing.T) {
		fake := &codexCreditsThenSuccessAgent{}
		a := newSingleAgentFixture(t, codexAgentID, fake, func(conn *sdk.AgentSideConnection) { fake.conn = conn })
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Prompt(ctx, "credit notice before success", nil, 11); err != nil {
			t.Fatalf("successful prompt error = %v, want nil", err)
		}
		events := collectEventsThrough(t, a, streams.EventTypeComplete)
		diagnostic, completed := false, false
		for _, event := range events {
			switch event.Type {
			case streams.EventTypeMessageChunk:
				diagnostic = diagnostic || event.ProviderDiagnosticCandidate
			case streams.EventTypeComplete:
				completed = true
			case streams.EventTypeError:
				t.Fatalf("successful completion emitted an error event: %+v", event)
			}
		}
		if !diagnostic || !completed {
			t.Fatalf("events = %+v, want the credit diagnostic and completion without an error", events)
		}
	})
}

// codexCreditsThenSuccessAgent streams the workspace-credit notice and then
// completes normally. The notice must remain ordinary streamed text, never a
// terminal substitution.
type codexCreditsThenSuccessAgent struct {
	burstAgent
	conn *sdk.AgentSideConnection
}

func (a *codexCreditsThenSuccessAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "credits-success-session"}, nil
}

func (a *codexCreditsThenSuccessAgent) Prompt(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	update := sdk.SessionUpdate{AgentMessageChunk: &sdk.SessionUpdateAgentMessageChunk{
		Content: sdk.TextBlock(codexWorkspaceCreditsNotice),
	}}
	if err := a.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: req.SessionId, Update: update}); err != nil {
		return sdk.PromptResponse{}, err
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func collectEventsThrough(t *testing.T, a *Adapter, eventType string) []AgentEvent {
	t.Helper()
	var events []AgentEvent
	for {
		select {
		case event := <-a.updatesCh:
			events = append(events, event)
			if event.Type == eventType {
				return events
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %q event", eventType)
		}
	}
}
