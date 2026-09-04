package acp

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/replayfixtures"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// replayFakeAgent replays one fixture's frames onto the retained
// AgentSideConnection, then returns the *acp.RequestError its prompt_error
// frame declares — the shape provider-error-recovery-02.md#replay-harness-semantics
// calls for: "for each frame before the prompt_error, it calls
// AgentSideConnection.SessionUpdate ...; then it returns
// &acp.RequestError{Code, Message, Data} built from the prompt_error frame."
type replayFakeAgent struct {
	concurrencyFakeAgent
	conn    *acp.AgentSideConnection
	fixture replayfixtures.Fixture
}

func (f *replayFakeAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: acp.SessionId(f.fixture.Identity.SessionID + "-acp")}, nil
}

func (f *replayFakeAgent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	var promptErrorFrame replayfixtures.Frame
	for _, frame := range f.fixture.Frames {
		if frame.Kind == replayfixtures.FramePromptError {
			promptErrorFrame = frame
			continue
		}
		update, ok := buildReplaySessionUpdate(frame)
		if !ok {
			continue
		}
		if err := f.conn.SessionUpdate(ctx, acp.SessionNotification{SessionId: req.SessionId, Update: update}); err != nil {
			return acp.PromptResponse{}, err
		}
	}
	return acp.PromptResponse{}, &acp.RequestError{
		Code:    promptErrorFrame.Code,
		Message: promptErrorFrame.Message,
		Data:    promptErrorFrame.Data,
	}
}

// buildReplaySessionUpdate converts one non-prompt_error fixture frame into
// the acp.SessionUpdate notification the frame table in
// provider-error-recovery-02.md#fixture-document names. ok is false for a
// frame kind (only prompt_error today) that carries no notification.
func buildReplaySessionUpdate(frame replayfixtures.Frame) (acp.SessionUpdate, bool) {
	switch frame.Kind {
	case replayfixtures.FrameMessageChunk:
		content := acp.ContentBlock{Text: &acp.ContentBlockText{Text: frame.Text}}
		if frame.Role == "user" {
			return acp.SessionUpdate{UserMessageChunk: &acp.SessionUpdateUserMessageChunk{Content: content}}, true
		}
		return acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: content}}, true
	case replayfixtures.FrameThoughtChunk:
		content := acp.ContentBlock{Text: &acp.ContentBlockText{Text: frame.Text}}
		return acp.SessionUpdate{AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{Content: content}}, true
	case replayfixtures.FrameToolCall:
		status := frame.Status
		if status == "" {
			status = "pending"
		}
		return acp.SessionUpdate{ToolCall: &acp.SessionUpdateToolCall{
			ToolCallId: acp.ToolCallId(frame.ToolCallID),
			Title:      frame.Title,
			Status:     acp.ToolCallStatus(status),
		}}, true
	case replayfixtures.FrameToolUpdate:
		status := acp.ToolCallStatus(frame.Status)
		return acp.SessionUpdate{ToolCallUpdate: &acp.SessionToolCallUpdate{
			ToolCallId: acp.ToolCallId(frame.ToolCallID),
			Status:     &status,
		}}, true
	case replayfixtures.FrameModelSettled:
		return acp.SessionUpdate{ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
			ConfigOptions: []acp.SessionConfigOption{{
				Select: &acp.SessionConfigOptionSelect{
					Id:           acp.SessionConfigId(configOptionIDModel),
					Name:         "Model",
					Type:         "select",
					CurrentValue: acp.SessionConfigValueId(frame.ModelID),
					Options: acp.SessionConfigSelectOptions{
						Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
							{Value: acp.SessionConfigValueId(frame.ModelID), Name: frame.ModelID},
						},
					},
				},
			}},
		}}, true
	default:
		return acp.SessionUpdate{}, false
	}
}

// replayFixtureThroughAdapter drives fx's frames through a live Adapter over
// a real io.Pipe-backed ACP connection pair, exactly as
// provider-error-recovery-02.md#replay-harness-semantics describes: two pipe
// pairs, a real acp.ClientSideConnection assigned to Adapter.acpConn, and a
// real acp.AgentSideConnection wrapping the fixture-driven fake agent. It
// returns the live Adapter (so callers can read state the replay actually
// settled, such as ProviderErrorContext), the tokenized event sequence
// observed on updatesCh (excluding the terminal error, which has no
// AgentEvent counterpart on this path), and the error Adapter.Prompt
// returned.
func replayFixtureThroughAdapter(t *testing.T, fx replayfixtures.Fixture) (*Adapter, []string, error) {
	t.Helper()

	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	a := newTestAdapterForAgent(fx.AgentID)
	fake := &replayFakeAgent{fixture: fx}

	if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	fake.conn = acp.NewAgentSideConnection(fake, agentToClientW, clientToAgentR)
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})

	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("new session: %v", err)
	}

	promptDone := make(chan error, 1)
	go func() {
		promptDone <- a.Prompt(ctx, "continue", nil, fx.Identity.PromptGeneration)
	}()

	var promptErr error
	select {
	case promptErr = <-promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Adapter.Prompt did not return")
	}

	return a, tokenizeEvents(drainEvents(a)), promptErr
}

// tokenizeEvents keeps only the events whose type is in the closed
// expect.events vocabulary (provider-error-recovery-02.md#fixture-document),
// mapping a marked message chunk to the ":diagnostic" token. A
// session-configuration event from a model_settled frame is not in the
// vocabulary and is filtered out, exactly as the design specifies.
func tokenizeEvents(events []AgentEvent) []string {
	var tokens []string
	for _, ev := range events {
		switch ev.Type {
		case streams.EventTypeMessageChunk:
			if ev.ProviderDiagnosticCandidate {
				tokens = append(tokens, "message_chunk:diagnostic")
			} else {
				tokens = append(tokens, "message_chunk")
			}
		case streams.EventTypeReasoning:
			tokens = append(tokens, "thought_chunk")
		case streams.EventTypeToolCall:
			tokens = append(tokens, "tool_call")
		case streams.EventTypeToolUpdate:
			tokens = append(tokens, "tool_update")
		}
	}
	return tokens
}

// TestReplayFixtureTransportLayer drives every fixture in the shared ACP
// replay corpus through a live Adapter and asserts the two things
// provider-error-recovery-02.md#replay-harness-semantics assigns to the ACP
// transport layer: expect.events up to (but excluding) the trailing error
// token, and the error token plus expect.providerError/expect.diagnosticCode
// derived from ProviderErrorFromError applied to the error Adapter.Prompt
// returned.
func TestReplayFixtureTransportLayer(t *testing.T) {
	fixtures := replayfixtures.MustLoad()

	for _, fx := range fixtures {
		t.Run(fx.FileName, func(t *testing.T) {
			a, tokens, promptErr := replayFixtureThroughAdapter(t, fx)

			wantTokens := fx.Expect.Events[:len(fx.Expect.Events)-1]
			if len(tokens) != len(wantTokens) {
				t.Fatalf("tokenized events = %v, want %v", tokens, wantTokens)
			}
			for i := range tokens {
				if tokens[i] != wantTokens[i] {
					t.Fatalf("tokenized events = %v, want %v", tokens, wantTokens)
				}
			}
			if fx.Expect.Events[len(fx.Expect.Events)-1] != "error" {
				t.Fatalf("fixture %s: expect.events must end with error", fx.FileName)
			}

			if promptErr == nil {
				t.Fatal("Adapter.Prompt returned nil, want the fixture's prompt_error")
			}

			var reqErr *acp.RequestError
			if !errors.As(promptErr, &reqErr) {
				t.Fatalf("Adapter.Prompt error = %v, want *acp.RequestError", promptErr)
			}

			providerID, modelID := a.ProviderErrorContext()
			got := ProviderErrorFromError(promptErr, providerID, modelID)
			if got == nil {
				t.Fatal("ProviderErrorFromError() = nil, want a projection")
			}
			want := fx.Expect.ProviderError
			if got.Source != want.Source {
				t.Fatalf("providerError.source = %q, want %q", got.Source, want.Source)
			}
			if got.RPCCode != want.RPCCode {
				t.Fatalf("providerError.rpc_code = %d, want %d", got.RPCCode, want.RPCCode)
			}
			if got.ErrorKind != want.ErrorKind {
				t.Fatalf("providerError.error_kind = %q, want %q", got.ErrorKind, want.ErrorKind)
			}
			if got.ProviderID != want.ProviderID {
				t.Fatalf("providerError.provider_id = %q, want %q", got.ProviderID, want.ProviderID)
			}
			if got.ModelID != want.ModelID {
				t.Fatalf("providerError.model_id = %q, want %q", got.ModelID, want.ModelID)
			}

			diagnosticCode := routingerr.Classify(routingerr.Input{
				Phase:  routingerr.PhasePromptSend,
				Stderr: reqErr.Message,
			}).Code
			if string(diagnosticCode) != fx.Expect.DiagnosticCode {
				t.Fatalf("diagnosticCode = %q, want %q", diagnosticCode, fx.Expect.DiagnosticCode)
			}
		})
	}
}
