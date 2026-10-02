package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kandev/kandev/internal/coordinator"
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
	}
	assert.ElementsMatch(t, want, getRegisteredToolNames(s))
}

func TestServerSurfaceCoordinatorRegistersBoundActivityTool(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	profile := mcpprofile.NewCoordinator()
	profile.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
		Version: 1, CoordinatorID: "c", WorkspaceID: "w", ConversationTaskID: "t",
		ToolNames: []string{"list_tasks_kandev", "list_coordinator_activity_kandev"},
	}
	s := NewWithProfile(backend, "s", "t", 10005, log, "", false, profile)

	assert.ElementsMatch(t, []string{"list_tasks_kandev", "list_coordinator_activity_kandev"}, getRegisteredToolNames(s))
}

func TestServerSurfaceCoordinatorRegistersEveryBoundToolForEveryPolicy(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	actions := []coordinator.Action{coordinator.ActionCreateTask, coordinator.ActionMessage, coordinator.ActionMove, coordinator.ActionResume}
	for mask := 0; mask < 1<<len(actions); mask++ {
		policy := coordinator.PhaseOnePolicy()
		for i, a := range actions {
			setting := coordinator.SettingDenied
			if mask&(1<<i) != 0 {
				setting = coordinator.SettingRequiresApproval
			}
			policy.Actions[a] = setting
		}
		want := coordinator.ToolNames(policy, true, false)
		profile := mcpprofile.NewCoordinator()
		profile.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
			Version: 1, CoordinatorID: "c", WorkspaceID: "w", ConversationTaskID: "t", ToolNames: want,
		}
		s := NewWithProfile(backend, "s", "t", 10005, log, "", false, profile)
		assert.ElementsMatch(t, want, getRegisteredToolNames(s), "mask %d", mask)
	}
}

func TestServerSurfaceCoordinatorRegistersBoundTurnsToolOnly(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	bound := func(names ...string) []string {
		profile := mcpprofile.NewCoordinator()
		profile.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
			Version: 1, CoordinatorID: "c", WorkspaceID: "w", ConversationTaskID: "t", ToolNames: names,
		}
		return getRegisteredToolNames(NewWithProfile(backend, "s", "t", 10005, log, "", false, profile))
	}
	assert.ElementsMatch(t, []string{"list_tasks_kandev", "list_coordinator_turns_kandev"}, bound("list_tasks_kandev", "list_coordinator_turns_kandev"))
	assert.NotContains(t, bound("list_tasks_kandev", "list_coordinator_activity_kandev"), "list_coordinator_turns_kandev")
}
