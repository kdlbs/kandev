package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestDeliveryRetryReportsPausedProducerWithoutReplacingLiveProcess(t *testing.T) {
	for _, testcase := range []struct {
		health string
		reason string
	}{
		{`{"state":"guarded","producer_paused":true}`, "delivery_output_paused"},
		{`{"state":"cancelling","cancellation_pending":true}`, "delivery_cancellation_pending"},
		{`{"state":"guarded","producer_paused":false}`, "delivery_storage_pressure"},
	} {
		t.Run(testcase.reason, func(t *testing.T) {
			sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
			peer := newDeliveryReconciliationTestPeer()
			peer.streamOpen = true
			peer.status = func(context.Context, int) (*agentctl.StatusResponse, error) {
				var status agentctl.StatusResponse
				require.NoError(t, json.Unmarshal([]byte(`{"agent_status":"running","delivery_health":`+testcase.health+`}`), &status))
				return &status, nil
			}
			execution := newDeliveryReconciliationTestExecution()
			result := sm.reconcileDeliveryWithPeer(context.Background(), execution, peer)
			require.Equal(t, DeliveryReconciliationBlocked, result.Outcome)
			require.Equal(t, testcase.reason, result.Reason)
			require.False(t, result.ProcessTerminated)
			require.Equal(t, "running", string(execution.Status))
		})
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.6
func TestDeliveryFailedWriterNeverReportsRecovered(t *testing.T) {
	for name, status := range map[string]func() *agentctl.StatusResponse{
		"sticky_failure": func() *agentctl.StatusResponse {
			response := &agentctl.StatusResponse{AgentStatus: "running"}
			response.DeliveryHealth.State = deliveryFailedHealthState
			response.DeliveryHealth.Failure = &types.DeliveryFailureSnapshot{
				Reason: types.DeliveryFailureEventTooLarge, Revision: 1,
				SessionID: "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 3,
				StreamID: "stream-1", SubmissionID: "submission-1", PromptGeneration: 1,
			}
			return response
		},
		"completed_cancellation_without_terminal": func() *agentctl.StatusResponse {
			response := &agentctl.StatusResponse{AgentStatus: "running"}
			response.DeliveryHealth.State = deliveryInterruptedHealthState
			return response
		},
	} {
		t.Run(name, func(t *testing.T) {
			sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
			peer := newDeliveryReconciliationTestPeer()
			peer.streamOpen = true
			peer.status = func(context.Context, int) (*agentctl.StatusResponse, error) { return status(), nil }
			phases := make(chan DeliveryReconciliationPhase, 4)
			sm.onDeliveryReconciliationPhase = func(_ *AgentExecution, _ DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
				phases <- phase
			}
			result := sm.reconcileDeliveryWithPeer(context.Background(), newDeliveryReconciliationTestExecution(), peer)
			if result.Outcome != DeliveryReconciliationBlocked {
				t.Fatalf("outcome = %q, want blocked", result.Outcome)
			}
			if name == "sticky_failure" && result.Reason != "delivery_event_too_large" {
				t.Fatalf("failure reason = %q, want delivery_event_too_large", result.Reason)
			}
			seen := map[DeliveryReconciliationPhase]bool{}
			for len(phases) > 0 {
				seen[<-phases] = true
			}
			if !seen[DeliveryReconciliationPhaseReconnecting] || !seen[DeliveryReconciliationPhaseUncertain] {
				t.Fatalf("published phases = %v, want reconnecting and uncertain", seen)
			}
			if seen[DeliveryReconciliationPhaseRecovered] {
				t.Fatal("a live process and open socket dismissed a failed writer")
			}
			sm.Wait()
		})
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestDeliveryHealthySocketStillReportsRecovered(t *testing.T) {
	sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
	peer := newDeliveryReconciliationTestPeer()
	peer.streamOpen = true
	phases := make(chan DeliveryReconciliationPhase, 4)
	sm.onDeliveryReconciliationPhase = func(_ *AgentExecution, _ DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
		phases <- phase
	}
	result := sm.reconcileDeliveryWithPeer(context.Background(), newDeliveryReconciliationTestExecution(), peer)
	if result.Outcome != DeliveryReconciliationRunningAttached {
		t.Fatalf("outcome = %q, want running attached", result.Outcome)
	}
	seen := map[DeliveryReconciliationPhase]bool{}
	for len(phases) > 0 {
		seen[<-phases] = true
	}
	if !seen[DeliveryReconciliationPhaseRecovered] || seen[DeliveryReconciliationPhaseUncertain] {
		t.Fatalf("published phases = %v, want recovered without uncertainty", seen)
	}
	sm.Wait()
}
