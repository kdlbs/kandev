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

func TestCodexMCPStartupWarningDoesNotClaimSubmission(t *testing.T) {
	manager := &Manager{deliveryActiveID: "active-submission"}
	manager.TrackDeliverySubmission("generation-submission", 42)
	warning := adapter.AgentEvent{
		Type:                 "mcp_startup_warning",
		PromptGeneration:     42,
		DeliverySubmissionID: "provider-submission",
	}
	if got := manager.deliverySubmissionIDForEvent(warning); got != "" {
		t.Fatalf("startup warning submission = %q, want no prompt attribution", got)
	}
	if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{PromptGeneration: 42}); got != "generation-submission" {
		t.Fatalf("startup warning consumed prompt attribution = %q, want generation-submission", got)
	}
}
