package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestInitialSubmissionIdentitySurvivesRuntimeLoss(t *testing.T) {
	owner, oldBinding, _ := replaceTestRuntime(t)
	manager, eventBus := createTestManagerWithTracking()
	t.Cleanup(func() { _ = manager.Stop() })
	manager.SetRuntimeOwner(owner)
	execution := createTestExecution("execution-initial-loss", "task-1", "session-initial-loss")
	execution.RuntimeName = executor.NameStandalone
	execution.Status = v1.AgentStatusRunning
	execution.DeliveryMode = DurableDeliveryV1
	execution.DeliveryStreamID = "stream-initial-loss"
	execution.DeliveryIncarnationID = "incarnation-initial-loss"
	execution.DeliveryHarnessGeneration = 3
	execution.promptGeneration = 1
	execution.runtimeEpoch = oldBinding.Epoch()
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "prompt:initial-message")
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	manager.reconcileRuntimeLoss(oldBinding.Epoch())
	for _, published := range eventBus.PublishedEvents {
		if published.Event == nil || published.Event.Type != events.AgentctlError {
			continue
		}
		payload, ok := published.Event.Data.(AgentctlEventPayload)
		if !ok || payload.DeliveryRecoveryPhase != string(DeliveryReconciliationPhaseUncertain) {
			continue
		}
		if payload.SessionID != execution.SessionID || payload.AgentExecutionID != execution.ID ||
			payload.DeliverySubmissionID != "prompt:initial-message" || payload.DeliveryStreamID != execution.DeliveryStreamID ||
			payload.DeliveryIncarnationID != execution.DeliveryIncarnationID ||
			payload.DeliveryHarnessGeneration != execution.DeliveryHarnessGeneration {
			t.Fatalf("runtime-loss recovery payload = %+v, want persisted initial submission identity", payload)
		}
		return
	}
	t.Fatalf("events = %+v, want initial submission recovery event", eventBus.PublishedEvents)
}

func TestCompletedInitialSubmissionIsNotReusedAsCurrentIdentity(t *testing.T) {
	execution := &AgentExecution{DeliveryMode: DurableDeliveryV1, SessionID: "session-1"}
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "prompt:initial-message")
	execution.setDeliverySubmissionID("prompt:initial-message")
	execution.clearDeliverySubmissionID("prompt:initial-message")
	clearInitialDeliverySubmissionID(execution, "prompt:initial-message")

	if got := currentDeliverySubmissionID(execution); got != "" {
		t.Fatalf("current delivery identity = %q after terminal completion, want empty", got)
	}
}

func TestInitialSubmissionIdentityFallbackDoesNotReplaceAcceptedSubmission(t *testing.T) {
	execution := &AgentExecution{
		ID: "execution-initial-fallback", SessionID: "session-initial-fallback",
		DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream-initial-fallback",
		DeliveryIncarnationID: "incarnation-initial-fallback", DeliveryHarnessGeneration: 1,
	}
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "prompt:initial-message")
	execution.setDeliverySubmissionID("prompt:later-message")

	identity, err := captureDeliveryReconciliationIdentity(execution)
	if err != nil {
		t.Fatalf("capture delivery identity: %v", err)
	}
	if identity.SubmissionID != "prompt:later-message" {
		t.Fatalf("submission identity = %q, want accepted later submission", identity.SubmissionID)
	}
}
