package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestUnnumberedDeliveryCancellationCannotCompleteSuccessor(t *testing.T) {
	manager, bus := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.promptGeneration = 2
	execution.setDeliverySubmissionID("successor")
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	if manager.handleCompleteEvent(execution, &agentctl.AgentEvent{
		Type: "complete", DeliverySubmissionID: "cancelled-predecessor",
		Data: map[string]any{"stop_reason": "cancelled"},
	}) {
		t.Fatal("predecessor cancellation completed the successor")
	}
	if execution.Status != v1.AgentStatusRunning || execution.deliverySubmissionIDSnapshot() != "successor" {
		t.Fatalf("successor changed: status=%s submission=%s", execution.Status, execution.deliverySubmissionIDSnapshot())
	}
	if len(bus.PublishedEvents) != 0 || len(execution.promptDoneCh) != 0 {
		t.Fatal("predecessor cancellation published or signalled completion")
	}
}

func TestUnnumberedDeliveryCancellationClaimsOnlyItsActiveGeneration(t *testing.T) {
	manager, _ := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.promptGeneration = 2
	execution.setDeliverySubmissionID("active")
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	event := &agentctl.AgentEvent{
		Type: "complete", DeliverySubmissionID: "active",
		Data: map[string]any{"stop_reason": "cancelled"},
	}
	if !manager.handleCompleteEvent(execution, event) {
		t.Fatal("active cancellation was ignored")
	}
	if event.PromptGeneration != 2 || execution.Status != v1.AgentStatusReady || execution.deliverySubmissionIDSnapshot() != "" {
		t.Fatalf("active cancellation not fenced: generation=%d status=%s submission=%s", event.PromptGeneration, execution.Status, execution.deliverySubmissionIDSnapshot())
	}
	if manager.handleCompleteEvent(execution, event) {
		t.Fatal("duplicate cancellation was applied twice")
	}
}
