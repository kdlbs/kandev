package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

// TestServerSurfaceCoordinatorToolCatalog pins the coordinator
// surface's tool set (docs/specs/coordinator/system-design/copilot-tools.md
// #tool-surface, AC-COORDINATOR-COPILOT-003.1): the five existing read tools
// reused verbatim, plus propose_task_kandev, get_coordinator_item_kandev and
// the phase-2 list_coordinator_activity_kandev (which sessions may call it is
// the coordinator tool profile's decision), and nothing else — no plugin tools, no user-question/parent-question/title
// tools, no create/move/message/archive/delete task tools.
func TestServerSurfaceCoordinatorToolCatalog(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	profile := mcpprofile.NewCoordinator()
	s := NewWithProfile(backend, "coordinator-session", "coordinator-task", 10005, log, "", false, profile)

	want := []string{
		"list_workflows_kandev",
		"list_workflow_steps_kandev",
		"list_repositories_kandev",
		"list_tasks_kandev",
		"get_task_conversation_kandev",
		"propose_task_kandev",
		"get_coordinator_item_kandev",
		"list_coordinator_activity_kandev",
	}
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))
}
