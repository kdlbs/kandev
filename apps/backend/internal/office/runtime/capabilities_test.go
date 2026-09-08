package runtime

import (
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestDecisionToolIsNotRuntimeCapability(t *testing.T) {
	caps := Capabilities{}
	if caps.Allows(AvailableActionRecordStepDecision) {
		t.Fatal("decision tool availability must not be treated as a runtime capability")
	}
	for _, key := range caps.AllowedKeys() {
		if key == AvailableActionRecordStepDecision {
			t.Fatalf("runtime capability list must not include the decision tool: %v", caps.AllowedKeys())
		}
	}
}

func TestCapabilities_AllowsListTasks(t *testing.T) {
	if (Capabilities{}).Allows(CapabilityListTasks) {
		t.Fatal("Allows(list_tasks) = true for zero-value capabilities, want false")
	}
	if !(Capabilities{CanListTasks: true}).Allows(CapabilityListTasks) {
		t.Fatal("Allows(list_tasks) = false with CanListTasks set, want true")
	}
}

func TestCapabilities_AllowsUnknownKeyDefaultsFalse(t *testing.T) {
	if (Capabilities{
		CanPostComments: true, CanUpdateTaskStatus: true, CanCreateTasks: true,
		CanCreateSubtasks: true, CanCreateAgents: true, CanListProjects: true,
		CanListTasks: true, CanCreateProjects: true, CanRequestApproval: true,
		CanReadMemory: true, CanWriteMemory: true, CanListSkills: true,
		CanSpawnAgentRun: true, CanModifyAgents: true, CanDeleteSkills: true,
	}).Allows("not_a_real_capability") {
		t.Fatal("Allows(unknown key) = true, want false even with every other capability granted")
	}
}

func TestCapabilities_AllowedKeysIncludesListTasksInStableOrder(t *testing.T) {
	caps := Capabilities{CanListProjects: true, CanListTasks: true, CanPostComments: true}
	keys := caps.AllowedKeys()
	want := []string{CapabilityPostComment, CapabilityListProjects, CapabilityListTasks}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
}

func TestFromAgentNeverGrantsRecordStepDecision(t *testing.T) {
	ceo := &models.AgentInstance{
		ID:          "agent-ceo",
		WorkspaceID: "ws-1",
		Role:        models.AgentRoleCEO,
	}
	caps := FromAgent(ceo)
	if caps.Allows(AvailableActionRecordStepDecision) {
		t.Fatal("Allows must not authorize the decision tool")
	}
}

func TestCapabilities_AllowedKeysOmitsDisabledListTasks(t *testing.T) {
	caps := Capabilities{CanListTasks: false, CanListProjects: true}
	for _, k := range caps.AllowedKeys() {
		if k == CapabilityListTasks {
			t.Fatalf("AllowedKeys() = %v, want list_tasks omitted when CanListTasks is false", caps.AllowedKeys())
		}
	}
}

func TestFromAgent_GrantsListTasksForEveryRole(t *testing.T) {
	roles := []models.AgentRole{
		models.AgentRoleCEO, models.AgentRoleWorker, models.AgentRoleSpecialist,
		models.AgentRoleAssistant, models.AgentRoleSecurity, models.AgentRoleQA, models.AgentRoleDevOps,
	}
	for _, role := range roles {
		agent := &models.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1", Role: role}
		caps := FromAgent(agent)
		if !caps.CanListTasks {
			t.Fatalf("role %v: CanListTasks = false, want true", role)
		}
		if !caps.Allows(CapabilityListTasks) {
			t.Fatalf("role %v: Allows(list_tasks) = false, want true", role)
		}
	}
}

func TestFromAgent_NilAgentGrantsNothing(t *testing.T) {
	caps := FromAgent(nil)
	if caps.CanListTasks || caps.Allows(CapabilityListTasks) {
		t.Fatal("FromAgent(nil) granted list_tasks, want a zero-value Capabilities")
	}
}

func TestWithTaskScope_FiltersEmptyAndWhitespaceIDs(t *testing.T) {
	caps := Capabilities{}.WithTaskScope("task-1", "", "   ", "task-2")
	want := []string{"task-1", "task-2"}
	if len(caps.AllowedTaskIDs) != len(want) {
		t.Fatalf("AllowedTaskIDs = %v, want %v", caps.AllowedTaskIDs, want)
	}
	for i, id := range want {
		if caps.AllowedTaskIDs[i] != id {
			t.Fatalf("AllowedTaskIDs = %v, want %v", caps.AllowedTaskIDs, want)
		}
	}
}

func TestWithTaskScope_NoArgumentsYieldsEmptyNotWildcard(t *testing.T) {
	caps := Capabilities{}.WithTaskScope()
	if len(caps.AllowedTaskIDs) != 0 {
		t.Fatalf("AllowedTaskIDs = %v, want empty", caps.AllowedTaskIDs)
	}
	for _, id := range caps.AllowedTaskIDs {
		if id == WildcardTaskScope {
			t.Fatal("WithTaskScope() with no args produced a wildcard scope")
		}
	}
}

func TestWithTaskScope_AllWhitespaceArgumentsYieldEmptyScope(t *testing.T) {
	caps := Capabilities{}.WithTaskScope("", "   ", "\t")
	if len(caps.AllowedTaskIDs) != 0 {
		t.Fatalf("AllowedTaskIDs = %v, want empty", caps.AllowedTaskIDs)
	}
}

func TestWithTaskScope_RejectsWildcardSentinel(t *testing.T) {
	caps := Capabilities{}.WithTaskScope(WildcardTaskScope, "task-1")
	want := []string{"task-1"}
	if len(caps.AllowedTaskIDs) != len(want) || caps.AllowedTaskIDs[0] != want[0] {
		t.Fatalf(
			"AllowedTaskIDs = %v, want %v — the reserved wildcard sentinel must never be assignable from external input",
			caps.AllowedTaskIDs, want,
		)
	}
}
