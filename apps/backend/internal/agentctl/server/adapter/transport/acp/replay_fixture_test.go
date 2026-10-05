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
	conn         *acp.AgentSideConnection
	fixture      replayfixtures.Fixture
	requestError *acp.RequestError
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
	f.requestError = &acp.RequestError{
		Code:    promptErrorFrame.Code,
		Message: promptErrorFrame.Message,
		Data:    promptErrorFrame.Data,
	}
	return acp.PromptResponse{}, f.requestError
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
// observed on updatesCh, the raw RequestError returned by the fixture-driven
// ACP agent, and the error Adapter.Prompt returned. Retainable provider errors
// are represented by a terminal error event and a nil adapter return.
func replayFixtureThroughAdapter(
	t *testing.T,
	fx replayfixtures.Fixture,
) (*Adapter, []AgentEvent, *acp.RequestError, error) {
	t.Helper()

	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()

	a := newTestAdapterForAgent(fx.AgentID)
	fake := &replayFakeAgent{fixture: fx}

	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentW.Close()
		_ = agentToClientW.Close()
	})

	if err := a.Connect(clientToAgentW, agentToClientR); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	fake.conn = acp.NewAgentSideConnection(fake, agentToClientW, clientToAgentR)

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

	return a, drainEvents(a), fake.requestError, promptErr
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
		case streams.EventTypeError:
			tokens = append(tokens, "error")
		}
	}
	return tokens
}

// TestReplayFixtureTransportLayer drives every fixture in the shared ACP
// replay corpus through a live Adapter and asserts the two things
// provider-error-recovery-02.md#replay-harness-semantics assigns to the ACP
// transport layer: expect.events, plus expect.providerError/expect.diagnosticCode
// derived from either a retained terminal error event or the error Adapter.Prompt
// returned for a terminal provider failure.
func TestReplayFixtureTransportLayer(t *testing.T) {
	fixtures := replayfixtures.MustLoad()

	for _, fx := range fixtures {
		t.Run(fx.FileName, func(t *testing.T) {
			a, observedEvents, requestErr, promptErr := replayFixtureThroughAdapter(t, fx)
			if requestErr == nil {
				t.Fatal("fixture ACP agent returned no RequestError")
			}
			promptErrorFrame := fx.Frames[len(fx.Frames)-1]
			if requestErr.Code != promptErrorFrame.Code {
				t.Fatalf("fixture RequestError code = %d, want %d", requestErr.Code, promptErrorFrame.Code)
			}
			wantRetainedFailure := fx.Expect.DiagnosticCode == string(routingerr.CodeProviderOverloaded) ||
				fx.Expect.DiagnosticCode == string(routingerr.CodeModelCapacity) ||
				fx.Expect.DiagnosticCode == string(routingerr.CodeRateLimited)
			if (promptErr == nil) != wantRetainedFailure {
				t.Fatalf("Adapter.Prompt retained failure = %v, want %v for diagnostic %q",
					promptErr == nil, wantRetainedFailure, fx.Expect.DiagnosticCode)
			}
			tokens := tokenizeEvents(observedEvents)

			wantTokens := fx.Expect.Events
			if promptErr != nil {
				wantTokens = wantTokens[:len(wantTokens)-1]
			}
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

			var got *streams.ProviderError
			if promptErr == nil {
				var terminal *AgentEvent
				for i := range observedEvents {
					if observedEvents[i].Type == streams.EventTypeError {
						terminal = &observedEvents[i]
					}
				}
				if terminal == nil {
					t.Fatal("Adapter.Prompt returned nil without a terminal error event")
				}
				if terminal.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
					t.Fatalf("terminal disposition = %q, want retain_runtime", terminal.PromptFailureDisposition)
				}
				got = terminal.ProviderError
			} else {
				var reqErr *acp.RequestError
				if !errors.As(promptErr, &reqErr) {
					t.Fatalf("Adapter.Prompt error = %v, want *acp.RequestError", promptErr)
				}

				providerID, modelID := a.ProviderErrorContext()
				got = ProviderErrorFromError(promptErr, providerID, modelID)
			}
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
				Stderr: got.Message,
			}).Code
			if string(diagnosticCode) != fx.Expect.DiagnosticCode {
				t.Fatalf("diagnosticCode = %q, want %q", diagnosticCode, fx.Expect.DiagnosticCode)
			}
		})
	}
}

func TestReplayFixtureRetainedCapacityUsesMarkedRequestError(t *testing.T) {
	fx := replayfixtures.Fixture{
		AgentID: mockAgentID,
		Identity: replayfixtures.Identity{
			SessionID:        "mock-retained-capacity",
			ExecutionID:      "mock-retained-capacity-execution",
			PromptGeneration: 7,
		},
		Frames: []replayfixtures.Frame{{
			Kind:    replayfixtures.FramePromptError,
			Code:    -32603,
			Message: "Selected model is at capacity. Please try a different model.",
			Data:    map[string]any{"kandevMock": map[string]any{"retainedProviderCapacity": true}},
		}},
	}
	_, observedEvents, requestErr, promptErr := replayFixtureThroughAdapter(t, fx)
	if promptErr != nil {
		t.Fatalf("Adapter.Prompt() error = %v, want retained turn event", promptErr)
	}
	if requestErr == nil || requestErr.Code != -32603 {
		t.Fatalf("raw ACP RequestError = %#v, want code -32603", requestErr)
	}
	data, ok := requestErr.Data.(map[string]any)
	if !ok {
		t.Fatalf("raw ACP RequestError data = %#v, want marker", requestErr.Data)
	}
	meta, ok := data["kandevMock"].(map[string]any)
	if !ok || meta["retainedProviderCapacity"] != true {
		t.Fatalf("raw ACP RequestError marker = %#v, want retainedProviderCapacity", requestErr.Data)
	}
	var terminal *AgentEvent
	for i := range observedEvents {
		if observedEvents[i].Type == streams.EventTypeError {
			terminal = &observedEvents[i]
		}
	}
	if terminal == nil {
		t.Fatal("Adapter.Prompt returned nil without a terminal error event")
	}
	if terminal.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("prompt failure disposition = %q, want retain_runtime", terminal.PromptFailureDisposition)
	}
	if terminal.ProviderError == nil || terminal.ProviderError.Source != streams.ProviderErrorSourceACPPrompt ||
		terminal.ProviderError.RPCCode != -32603 {
		t.Fatalf("terminal provider error = %+v, want the marked ACP prompt error", terminal.ProviderError)
	}
	if terminal.CapacityContinuation == nil || !terminal.CapacityContinuation.SafeFor(fx.Identity.PromptGeneration) {
		t.Fatalf("capacity continuation evidence = %+v, want safe generation %d", terminal.CapacityContinuation, fx.Identity.PromptGeneration)
	}
}
