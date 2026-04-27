package service_test

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestResolvePermissions_Defaults(t *testing.T) {
	perms := service.ResolvePermissions(models.AgentRoleCEO, "")
	if !service.HasPermission(perms, service.PermCanCreateAgents) {
		t.Error("CEO should have can_create_agents by default")
	}
	if !service.HasPermission(perms, service.PermCanApprove) {
		t.Error("CEO should have can_approve by default")
	}

	workerPerms := service.ResolvePermissions(models.AgentRoleWorker, "")
	if service.HasPermission(workerPerms, service.PermCanCreateAgents) {
		t.Error("Worker should not have can_create_agents by default")
	}
	if service.HasPermission(workerPerms, service.PermCanApprove) {
		t.Error("Worker should not have can_approve by default")
	}
}

func TestResolvePermissions_Override(t *testing.T) {
	overrides := `{"can_create_agents": true, "max_subtask_depth": 5}`
	perms := service.ResolvePermissions(models.AgentRoleWorker, overrides)

	if !service.HasPermission(perms, service.PermCanCreateAgents) {
		t.Error("override should grant can_create_agents to worker")
	}
	depth, ok := perms[service.PermMaxSubtaskDepth]
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
	if !service.HasPermission(perms, service.PermCanCreateTasks) {
		t.Error("worker should retain can_create_tasks default")
	}
}

func TestResolvePermissions_InvalidJSON(t *testing.T) {
	perms := service.ResolvePermissions(models.AgentRoleCEO, "not-json")
	if !service.HasPermission(perms, service.PermCanCreateAgents) {
		t.Error("invalid override JSON should fall back to defaults")
	}
}

func TestHasPermission(t *testing.T) {
	perms := map[string]interface{}{
		"can_create_tasks":  true,
		"can_approve":       false,
		"max_subtask_depth": 3,
	}
	if !service.HasPermission(perms, "can_create_tasks") {
		t.Error("expected true for can_create_tasks")
	}
	if service.HasPermission(perms, "can_approve") {
		t.Error("expected false for can_approve")
	}
	if service.HasPermission(perms, "nonexistent") {
		t.Error("expected false for nonexistent key")
	}
	// Non-bool value should return false.
	if service.HasPermission(perms, "max_subtask_depth") {
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
	if err := service.ValidateNoEscalation(callerPerms, requested); err != nil {
		t.Fatalf("caller should be able to grant owned perms: %v", err)
	}
}

func TestNoEscalation_CallerCannotGrant(t *testing.T) {
	callerPerms := map[string]interface{}{
		"can_create_tasks":  true,
		"can_create_agents": false,
	}
	requested := `{"can_create_agents": true}`
	err := service.ValidateNoEscalation(callerPerms, requested)
	if err == nil {
		t.Fatal("caller without can_create_agents should not grant it")
	}
}

func TestNoEscalation_DepthEscalation(t *testing.T) {
	callerPerms := map[string]interface{}{
		"max_subtask_depth": float64(2),
	}
	requested := `{"max_subtask_depth": 5}`
	err := service.ValidateNoEscalation(callerPerms, requested)
	if err == nil {
		t.Fatal("caller should not grant higher depth than it has")
	}
}

func TestNoEscalation_EmptyRequest(t *testing.T) {
	callerPerms := map[string]interface{}{"can_create_tasks": true}
	if err := service.ValidateNoEscalation(callerPerms, ""); err != nil {
		t.Fatalf("empty request should pass: %v", err)
	}
	if err := service.ValidateNoEscalation(callerPerms, "{}"); err != nil {
		t.Fatalf("empty object should pass: %v", err)
	}
}

func TestAllPermissionKeys(t *testing.T) {
	keys := service.AllPermissionKeys()
	if len(keys) != 6 {
		t.Errorf("expected 6 permission keys, got %d", len(keys))
	}
}

func TestDefaultPermissionsRoundTrip(t *testing.T) {
	// Ensure DefaultPermissions JSON is valid and round-trips.
	raw := service.DefaultPermissions(models.AgentRoleCEO)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("DefaultPermissions produced invalid JSON: %v", err)
	}
	if len(m) == 0 {
		t.Error("CEO should have non-empty default permissions")
	}
}
