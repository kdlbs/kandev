package acp

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type codexCapacityContinuityAgent struct {
	burstAgent
	conn       *sdk.AgentSideConnection
	promptRuns atomic.Int32
	newRuns    atomic.Int32
}

func (f *codexCapacityContinuityAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	f.newRuns.Add(1)
	return sdk.NewSessionResponse{SessionId: "capacity-session"}, nil
}

func (f *codexCapacityContinuityAgent) Prompt(ctx context.Context, _ sdk.PromptRequest) (sdk.PromptResponse, error) {
	if f.promptRuns.Add(1) == 1 {
		for _, raw := range []string{
			`{"sessionId":"capacity-session","update":{"sessionUpdate":"session_info_update","_meta":{"threadStatus":{"type":"systemError"}}}}`,
			`{"sessionId":"capacity-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Selected model is at capacity. Please try a different model."}}}`,
		} {
			var notification sdk.SessionNotification
			if err := json.Unmarshal([]byte(raw), &notification); err != nil {
				return sdk.PromptResponse{}, err
			}
			if err := f.conn.SessionUpdate(ctx, notification); err != nil {
				return sdk.PromptResponse{}, err
			}
		}
		return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func TestCodexCapacityKeepsACPConnectionForNextPrompt(t *testing.T) {
	a := newTestAdapterForAgent(codexAgentID)
	clientToAgentR, clientToAgentW := io.Pipe()
	agentToClientR, agentToClientW := io.Pipe()
	fake := &codexCapacityContinuityAgent{}
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
	connection := a.acpConn

	if err := a.Prompt(ctx, "first prompt", nil, 41); err != nil {
		t.Fatalf("capacity prompt returned error: %v", err)
	}
	failure := waitForEventType(t, a, streams.EventTypeError)
	if failure.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime {
		t.Fatalf("failure disposition = %q, want retain_runtime", failure.PromptFailureDisposition)
	}
	if failure.CapacityContinuation == nil || failure.CapacityContinuation.SafeFor(41) || failure.CapacityContinuation.CompletedTools != 0 {
		t.Fatalf("capacity continuation evidence = %+v, want output-only evidence refused", failure.CapacityContinuation)
	}

	if err := a.Prompt(ctx, "follow-up prompt", nil, 42); err != nil {
		t.Fatalf("follow-up prompt returned error: %v", err)
	}
	if got := waitForEventType(t, a, streams.EventTypeComplete).PromptGeneration; got != 42 {
		t.Fatalf("follow-up generation = %d, want 42", got)
	}
	if a.acpConn != connection {
		t.Fatal("follow-up prompt used a different ACP connection")
	}
	if got := fake.promptRuns.Load(); got != 2 {
		t.Fatalf("provider prompt calls = %d, want 2", got)
	}
	if got := fake.newRuns.Load(); got != 1 {
		t.Fatalf("ACP sessions created = %d, want 1", got)
	}
	select {
	case <-connection.Done():
		t.Fatal("ACP connection closed after capacity failure")
	default:
	}
}
