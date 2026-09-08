package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

// realPeerDisconnected is the exact ACP transport-death envelope observed in
// production: the upstream provider connection drops mid-turn on prompt-send.
const realPeerDisconnected = `{"code":-32603,"message":"Internal error","data":{"error":"peer disconnected before response"}}`

func TestHandleTransientFailure_TransportLostSchedulesRetry(t *testing.T) {
	svc, mc := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	armTransientPromptEvidence(svc)

	took := svc.handleTransientFailure(context.Background(), watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "execution-1",
		PromptGeneration: 7,
		ErrorMessage:     realPeerDisconnected,
	})
	if !took {
		t.Fatal("handleTransientFailure = false, want true (should own the transport-lost failure)")
	}

	if _, ok := svc.transientRetries.Load("s1"); !ok {
		t.Fatal("expected a transient retry entry for s1")
	}

	if len(mc.sessionMessages) != 1 {
		t.Fatalf("expected 1 status message, got %d", len(mc.sessionMessages))
	}
	msg := mc.sessionMessages[0]
	if msg.metadata["failure_code"] != "agent_transport_lost" {
		t.Errorf("failure_code = %v, want agent_transport_lost", msg.metadata["failure_code"])
	}
	// Must NOT be the red recovery banner.
	if msg.metadata["recovery_actions"] == true {
		t.Errorf("transient retry must not set recovery_actions=true (that is the red banner)")
	}
}

func TestTransientFailureLabelAndExhaustedMessage_TransportLost(t *testing.T) {
	classified := &routingerr.Error{Code: routingerr.CodeAgentTransportLost}

	if got, want := transientFailureLabel(classified), "Agent connection lost"; got != want {
		t.Errorf("transientFailureLabel(CodeAgentTransportLost) = %q, want %q", got, want)
	}

	want := "The agent connection kept dropping after several retries. Resume to try again, or start a fresh session."
	if got := transientFailureExhaustedMessage(classified); got != want {
		t.Errorf("transientFailureExhaustedMessage(CodeAgentTransportLost) = %q, want %q", got, want)
	}
}
