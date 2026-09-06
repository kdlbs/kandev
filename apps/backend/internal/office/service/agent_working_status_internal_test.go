package service

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestIsAgentActive_AcceptsWorking is the tripwire the "working" status
// depends on for its safety.
//
// Setting an agent to "working" while its run is in flight is only harmless
// because isAgentActive — the gate at scheduler_integration.go that decides
// whether a claimed run proceeds — treats "working" as active. If a later
// change narrows this predicate to "idle" only, every agent mid-run would
// silently stop being schedulable: runs would be finished with
// RunOutcomeAgentInactive instead of executing, and nothing else in the
// codebase would fail. This test exists to make that narrowing fail loudly
// here rather than in production.
func TestIsAgentActive_AcceptsWorking(t *testing.T) {
	if !isAgentActive(models.AgentStatusWorking) {
		t.Fatal("isAgentActive rejected AgentStatusWorking: agents are now set " +
			"to 'working' for the duration of a run (see markAgentWorking), so " +
			"narrowing this predicate silently parks every running agent")
	}
	if !isAgentActive(models.AgentStatusIdle) {
		t.Fatal("isAgentActive rejected AgentStatusIdle")
	}
}

// TestIsAgentActive_RejectsInactiveStatuses pins the other half of the
// contract so the tripwire above cannot be satisfied by a predicate that
// simply returns true for everything.
func TestIsAgentActive_RejectsInactiveStatuses(t *testing.T) {
	for _, status := range []models.AgentStatus{
		models.AgentStatusPaused,
		models.AgentStatusStopped,
		models.AgentStatusPendingApproval,
	} {
		if isAgentActive(status) {
			t.Fatalf("isAgentActive(%q) = true, want false", status)
		}
	}
}
