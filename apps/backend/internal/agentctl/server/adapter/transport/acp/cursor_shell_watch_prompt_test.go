package acp

import (
	"context"
	"io"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type cursorCancelledPromptAgent struct {
	sessionRequestCaptureAgent
	conn            *acpsdk.AgentSideConnection
	terminalCallID  string
	promptSessionID string
}

func (a *cursorCancelledPromptAgent) Initialize(
	_ context.Context,
	request acpsdk.InitializeRequest,
) (acpsdk.InitializeResponse, error) {
	return acpsdk.InitializeResponse{ProtocolVersion: request.ProtocolVersion}, nil
}

func (a *cursorCancelledPromptAgent) Prompt(
	ctx context.Context,
	_ acpsdk.PromptRequest,
) (acpsdk.PromptResponse, error) {
	if a.terminalCallID != "" {
		update := cursorShellWatchUpdateNotification(
			a.promptSessionID, a.terminalCallID, acpsdk.ToolCallStatusCompleted,
		)
		if err := a.conn.SessionUpdate(ctx, update); err != nil {
			return acpsdk.PromptResponse{}, err
		}
	}
	return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonCancelled}, nil
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.7
func TestCancelledPromptInterruptsOnlyItsOpenCursorWatches(t *testing.T) {
	a, fake := newCursorCancelledPromptFixture(t)
	sessionID := a.GetSessionID()
	for _, test := range []struct {
		callID     string
		generation uint64
	}{
		{callID: "watch-cancelled", generation: 42},
		{callID: "watch-finished", generation: 42},
		{callID: "watch-prior-turn", generation: 41},
	} {
		a.handleACPUpdate(cursorShellWatchRegistrationNotification(
			sessionID, test.callID, test.callID, "Wait for command", "/tmp/watch.sh",
		), test.generation)
		a.handleACPUpdate(cursorShellWatchUpdateNotification(
			sessionID, test.callID, acpsdk.ToolCallStatusInProgress,
		), test.generation)
		if events := backgroundObservations(drainEvents(a)); len(events) != 1 || events[0].State != streams.RunStateRunning {
			t.Fatalf("watch %s launch events = %+v, want one running observation", test.callID, events)
		}
	}
	fake.terminalCallID = "watch-finished"
	fake.promptSessionID = sessionID

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Prompt(ctx, "cancel this turn", nil, 42); err != nil {
		t.Fatalf("cancelled prompt: %v", err)
	}

	var cancelled, explicitlyCompleted *streams.WorkloadRunObservation
	var sawCancelledPrompt bool
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeComplete && event.Data["stop_reason"] == string(acpsdk.StopReasonCancelled) {
			sawCancelledPrompt = true
		}
		if event.Type != streams.EventTypeBackgroundWorkUpdated || event.BackgroundWork == nil {
			continue
		}
		observation := *event.BackgroundWork
		switch observation.SourceCallID {
		case "watch-cancelled":
			cancelled = &observation
		case "watch-finished":
			explicitlyCompleted = &observation
		case "watch-prior-turn":
			t.Fatalf("cancelled prompt changed a watch from another turn: %+v", observation)
		}
	}
	if !sawCancelledPrompt {
		t.Fatal("prompt response did not report the cancelled stop reason")
	}
	if cancelled == nil || cancelled.State != streams.RunStateInterrupted || cancelled.ExitCode != nil {
		t.Fatalf("open watch after cancelled prompt = %+v, want interrupted without shell exit code", cancelled)
	}
	if explicitlyCompleted == nil || explicitlyCompleted.State != streams.RunStateCompleted {
		t.Fatalf("watch with drained terminal evidence = %+v, want completed", explicitlyCompleted)
	}
	a.mu.RLock()
	priorTurnState := a.cursorShellWatches["watch-prior-turn"].state
	a.mu.RUnlock()
	if priorTurnState != streams.RunStateRunning {
		t.Fatalf("prior-turn watch state = %s, want running", priorTurnState)
	}
}

func newCursorCancelledPromptFixture(t *testing.T) (*Adapter, *cursorCancelledPromptAgent) {
	t.Helper()
	a := newTestAdapterForAgent(cursorAgentID)
	a.backgroundWorkEnabled = true
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	fake := &cursorCancelledPromptAgent{}
	conn := acpsdk.NewAgentSideConnection(fake, agentToClientWriter, clientToAgentReader)
	fake.conn = conn
	t.Cleanup(func() {
		_ = a.Close()
		_ = clientToAgentWriter.Close()
		_ = clientToAgentReader.Close()
		_ = agentToClientWriter.Close()
		_ = agentToClientReader.Close()
	})
	if err := a.Connect(clientToAgentWriter, agentToClientReader); err != nil {
		t.Fatalf("connect Cursor test agent: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("initialize Cursor test agent: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("create Cursor test session: %v", err)
	}
	drainEvents(a)
	return a, fake
}
