package shared_test

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
)

func TestResolvePermissions_Defaults(t *testing.T) {
	perms := shared.ResolvePermissions(shared.AgentRoleCEO, "")
	if !shared.HasPermission(perms, shared.PermCanCreateAgents) {
		t.Error("CEO should have can_create_agents by default")
	}
	if !shared.HasPermission(perms, shared.PermCanApprove) {
		t.Error("CEO should have can_approve by default")
	}

	workerPerms := shared.ResolvePermissions(shared.AgentRoleWorker, "")
	if shared.HasPermission(workerPerms, shared.PermCanCreateAgents) {
		t.Error("Worker should not have can_create_agents by default")
	}
	if shared.HasPermission(workerPerms, shared.PermCanApprove) {
		t.Error("Worker should not have can_approve by default")
	}
	// Workers can assign tasks — this is required for execution-policy review handoffs.
	if !shared.HasPermission(workerPerms, shared.PermCanAssignTasks) {
		t.Error("Worker should have can_assign_tasks by default")
	}

	specialistPerms := shared.ResolvePermissions(shared.AgentRoleSpecialist, "")
	if shared.HasPermission(specialistPerms, shared.PermCanAssignTasks) {
		t.Error("Specialist should not have can_assign_tasks by default")
	}
}

func TestResolvePermissions_Override(t *testing.T) {
	overrides := `{"can_create_agents": true, "max_subtask_depth": 5}`
	perms := shared.ResolvePermissions(shared.AgentRoleWorker, overrides)

	if !shared.HasPermission(perms, shared.PermCanCreateAgents) {
		t.Error("override should grant can_create_agents to worker")
	}
	depth, ok := perms[shared.PermMaxSubtaskDepth]
	if !ok {
		t.Fatal("max_subtask_depth should be present")
	}
	depthVal, ok := depth.(float64)
	if !ok {
		t.Fatalf("expected float64, got %T", depth)
	}
	if depthVal != 5 {
		t.Errorf("max_subtask_depth = %v, want 5", depthVal)
	}
	// Non-overridden defaults should still be present.
	if !shared.HasPermission(perms, shared.PermCanCreateTasks) {
		t.Error("worker should retain can_create_tasks default")
	}
}

func TestResolvePermissions_InvalidJSON(t *testing.T) {
	perms := shared.ResolvePermissions(shared.AgentRoleCEO, "not-json")
	if !shared.HasPermission(perms, shared.PermCanCreateAgents) {
		t.Error("invalid override JSON should fall back to defaults")
	}
}

func TestHasPermission(t *testing.T) {
	perms := map[string]interface{}{
		"can_create_tasks":  true,
		"can_approve":       false,
		"max_subtask_depth": 3,
	}
	if !shared.HasPermission(perms, "can_create_tasks") {
		t.Error("expected true for can_create_tasks")
	}
	if shared.HasPermission(perms, "can_approve") {
		t.Error("expected false for can_approve")
	}
	if shared.HasPermission(perms, "nonexistent") {
		t.Error("expected false for nonexistent key")
	}
	// Non-bool value should return false.
	if shared.HasPermission(perms, "max_subtask_depth") {
		t.Error("expected false for int value checked as bool")
	}
}

func TestNoEscalation_CallerCanGrant(t *testing.T) {
	callerPerms := map[string]interface{}{
		"can_create_tasks":  true,
		"can_create_agents": true,
		"max_subtask_depth": float64(3),
	}
	requested := `{"can_create_tasks": true, "max_subtask_depth": 2}`
	if err := shared.ValidateNoEscalation(callerPerms, requested); err != nil {
		t.Fatalf("caller should be able to grant owned perms: %v", err)
	}
}

func TestNoEscalation_CallerCannotGrant(t *testing.T) {
	callerPerms := map[string]interface{}{
		"can_create_tasks":  true,
		"can_create_agents": false,
	}
	requested := `{"can_create_agents": true}`
	err := shared.ValidateNoEscalation(callerPerms, requested)
	if err == nil {
		t.Fatal("caller without can_create_agents should not grant it")
	}
}

func TestNoEscalation_DepthEscalation(t *testing.T) {
	callerPerms := map[string]interface{}{
		"max_subtask_depth": float64(2),
	}
	requested := `{"max_subtask_depth": 5}`
	err := shared.ValidateNoEscalation(callerPerms, requested)
	if err == nil {
		t.Fatal("caller should not grant higher depth than it has")
	}
}

func TestNoEscalation_EmptyRequest(t *testing.T) {
	callerPerms := map[string]interface{}{"can_create_tasks": true}
	if err := shared.ValidateNoEscalation(callerPerms, ""); err != nil {
		t.Fatalf("empty request should pass: %v", err)
	}
	if err := shared.ValidateNoEscalation(callerPerms, "{}"); err != nil {
		t.Fatalf("empty object should pass: %v", err)
	}
}

func TestAllPermissionKeys(t *testing.T) {
	keys := shared.AllPermissionKeys()
	if len(keys) != 6 {
		t.Errorf("expected 6 permission keys, got %d", len(keys))
	}
}

func TestDefaultPermissionsRoundTrip(t *testing.T) {
	// Ensure DefaultPermissions JSON is valid and round-trips.
	raw := shared.DefaultPermissions(shared.AgentRoleCEO)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("DefaultPermissions produced invalid JSON: %v", err)
	}
	if len(m) == 0 {
		t.Error("CEO should have non-empty default permissions")
	}
}

func TestResolvePermissions_NewRoles(t *testing.T) {
	cases := []struct {
		role            shared.AgentRole
		wantApprove     bool
		wantAssign      bool
		wantCreateAgent bool
	}{
		{shared.AgentRoleSecurity, true, true, false},
		{shared.AgentRoleQA, false, false, false},
		{shared.AgentRoleDevOps, false, false, false},
	}
	for _, tc := range cases {
		perms := shared.ResolvePermissions(tc.role, "")
		if shared.HasPermission(perms, shared.PermCanApprove) != tc.wantApprove {
			t.Errorf("role %s: can_approve = %v, want %v", tc.role, !tc.wantApprove, tc.wantApprove)
		}
		if shared.HasPermission(perms, shared.PermCanAssignTasks) != tc.wantAssign {
			t.Errorf("role %s: can_assign_tasks = %v, want %v", tc.role, !tc.wantAssign, tc.wantAssign)
		}
		if shared.HasPermission(perms, shared.PermCanCreateAgents) != tc.wantCreateAgent {
			t.Errorf("role %s: can_create_agents = %v, want %v", tc.role, !tc.wantCreateAgent, tc.wantCreateAgent)
		}
	}
}
