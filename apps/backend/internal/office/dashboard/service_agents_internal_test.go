package dashboard

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// A working agent must count toward the dashboard's running total (DR-14):
// before agent_working_status.go started writing AgentStatusWorking, every
// agent read idle even mid-run, so this counter was permanently pinned at
// zero regardless of how many agents were actually busy.
func TestCountAgentsByStatus_WorkingCountsAsRunning(t *testing.T) {
	counts := countAgentsByStatus([]*models.AgentInstance{
		{ID: "a1", Status: models.AgentStatusWorking},
		{ID: "a2", Status: models.AgentStatusWorking},
		{ID: "a3", Status: models.AgentStatusIdle},
		{ID: "a4", Status: models.AgentStatusPaused},
		{ID: "a5", Status: models.AgentStatusStopped, PauseReason: "budget exceeded"},
	})

	if counts.running != 2 {
		t.Errorf("running = %d, want 2 (only working agents)", counts.running)
	}
	if counts.paused != 1 {
		t.Errorf("paused = %d, want 1", counts.paused)
	}
	if counts.errors != 1 {
		t.Errorf("errors = %d, want 1 (stopped with a pause reason)", counts.errors)
	}
}
