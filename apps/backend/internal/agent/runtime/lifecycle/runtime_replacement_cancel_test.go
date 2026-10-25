package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestRuntimeReplacementAfterCancelEscalation(t *testing.T) {
	for _, submissionID := range []string{"submission-1", ""} {
		t.Run("pending="+submissionID, func(t *testing.T) {
			owner, oldBinding, _ := replaceTestRuntime(t)
			manager, eventBus := createTestManagerWithTracking()
			manager.SetRuntimeOwner(owner)
			execution := createTestExecution("old-execution", "task-1", "session-1")
			execution.RuntimeName = executor.NameStandalone
			execution.runtimeEpoch = oldBinding.Epoch()
			execution.Status = v1.AgentStatusFailed
			execution.promptGeneration = 1
			execution.cancelEscalatedPromptGeneration.Store(1)
			execution.DeliveryMode = DurableDeliveryV1
			execution.DeliveryStreamID = "stream-1"
			execution.DeliveryIncarnationID = "incarnation-1"
			execution.DeliveryHarnessGeneration = 3
			execution.setDeliverySubmissionID(submissionID)
			if submissionID != "" {
				execution.FailureCode = durableDeliveryReconnectingFailureCode
				execution.FailureDetails = submissionID
			}
			if err := manager.executionStore.Add(execution); err != nil {
				t.Fatalf("add execution: %v", err)
			}

			manager.reconcileRuntimeLoss(oldBinding.Epoch())

			var recoveries []AgentctlEventPayload
			for _, published := range eventBus.PublishedEvents {
				if published.Event.Type != events.AgentctlError {
					continue
				}
				payload, ok := published.Event.Data.(AgentctlEventPayload)
				if ok && payload.DeliveryRecoveryPhase != "" {
					recoveries = append(recoveries, payload)
				}
			}
			if submissionID == "" {
				if len(recoveries) != 0 || execution.FailureCode != "" {
					t.Fatalf("settled execution gained delivery recovery: %+v", recoveries)
				}
				return
			}
			if len(recoveries) == 0 {
				t.Fatalf("recovery events = %+v, want uncertain delivery after cancellation", recoveries)
			}
			for _, got := range recoveries {
				if got.DeliveryRecoveryPhase != string(DeliveryReconciliationPhaseUncertain) ||
					got.AgentExecutionID != execution.ID || got.SessionID != execution.SessionID ||
					got.DeliverySubmissionID != submissionID || got.DeliveryStreamID != execution.DeliveryStreamID ||
					got.DeliveryIncarnationID != execution.DeliveryIncarnationID || got.DeliveryHarnessGeneration != 3 {
					t.Fatalf("recovery = %+v, want the original uncertain submission identity", got)
				}
			}
			if execution.Status != v1.AgentStatusFailed || currentDeliverySubmissionID(execution) != submissionID {
				t.Fatal("runtime loss changed cancellation status or cleared the unsettled delivery")
			}
		})
	}
}
