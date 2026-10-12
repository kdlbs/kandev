package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

func TestLateTerminalKeepsItsSubmissionDuringSuccessorDispatch(t *testing.T) {
	manager := &Manager{deliveryActiveID: "successor"}
	manager.TrackDeliverySubmission("predecessor", 1)
	manager.TrackDeliverySubmission("successor", 2)
	id := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{
		Type: adapter.EventTypeComplete, PromptGeneration: 1,
	})
	if id != "predecessor" {
		t.Fatalf("late terminal submission = %q, want predecessor", id)
	}
	if manager.deliverySubmissionIDForEvent(adapter.AgentEvent{PromptGeneration: 2}) != "successor" {
		t.Fatal("late terminal removed successor association")
	}
}
