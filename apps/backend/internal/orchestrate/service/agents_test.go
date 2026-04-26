package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func makeAgent(name string, role models.AgentRole) *models.AgentInstance {
	return &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        name,
		Role:        role,
		Status:      models.AgentStatusIdle,
	}
}

func TestCreateAgent_SetsDefaults(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}
	if agent.MaxConcurrentSessions != 1 {
		t.Errorf("max_concurrent_sessions = %d, want 1", agent.MaxConcurrentSessions)
	}
	if agent.Permissions == "" || agent.Permissions == "{}" {
		t.Error("expected default permissions to be set")
	}
	if agent.DesiredSkills != "[]" {
		t.Errorf("desired_skills = %q, want []", agent.DesiredSkills)
	}
}

func TestCreateAgent_NameRequired(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("", models.AgentRoleWorker)
	err := svc.CreateAgentInstance(ctx, agent)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreateAgent_InvalidRole(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("bad-role", models.AgentRole("manager"))
	err := svc.CreateAgentInstance(ctx, agent)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestCreateAgent_SingleCEO(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	ceo1 := makeAgent("ceo-1", models.AgentRoleCEO)
	if err := svc.CreateAgentInstance(ctx, ceo1); err != nil {
		t.Fatalf("create first CEO: %v", err)
	}

	ceo2 := makeAgent("ceo-2", models.AgentRoleCEO)
	err := svc.CreateAgentInstance(ctx, ceo2)
	if err == nil {
		t.Fatal("expected error creating second CEO")
	}
}

func TestCreateAgent_CEOAllowedInDifferentWorkspace(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	ceo1 := makeAgent("ceo-ws1", models.AgentRoleCEO)
	ceo1.WorkspaceID = "ws-1"
	if err := svc.CreateAgentInstance(ctx, ceo1); err != nil {
		t.Fatalf("create CEO in ws-1: %v", err)
	}

	ceo2 := makeAgent("ceo-ws2", models.AgentRoleCEO)
	ceo2.WorkspaceID = "ws-2"
	if err := svc.CreateAgentInstance(ctx, ceo2); err != nil {
		t.Fatalf("should allow CEO in different workspace: %v", err)
	}
}

func TestCreateAgent_ValidReportsTo(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	ceo := makeAgent("ceo", models.AgentRoleCEO)
	if err := svc.CreateAgentInstance(ctx, ceo); err != nil {
		t.Fatalf("create CEO: %v", err)
	}

	worker := makeAgent("worker", models.AgentRoleWorker)
	worker.ReportsTo = ceo.ID
	if err := svc.CreateAgentInstance(ctx, worker); err != nil {
		t.Fatalf("create worker with valid reports_to: %v", err)
	}
}

func TestCreateAgent_InvalidReportsTo(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	worker := makeAgent("worker", models.AgentRoleWorker)
	worker.ReportsTo = "nonexistent-id"
	err := svc.CreateAgentInstance(ctx, worker)
	if err == nil {
		t.Fatal("expected error for invalid reports_to")
	}
}

func TestCreateAgent_ReportsToSelf(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create an agent first to get an ID, then try to update it to report to itself
	agent := makeAgent("self-ref", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}
	agent.ReportsTo = agent.ID
	err := svc.UpdateAgentInstance(ctx, agent)
	if err == nil {
		t.Fatal("expected error for self-referencing reports_to")
	}
}

func TestCreateAgent_UniqueName(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a1 := makeAgent("same-name", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, a1); err != nil {
		t.Fatalf("create first: %v", err)
	}
	a2 := makeAgent("same-name", models.AgentRoleWorker)
	err := svc.CreateAgentInstance(ctx, a2)
	if err == nil {
		t.Fatal("expected error for duplicate name in same workspace")
	}
}

func TestDefaultPermissions_CEO(t *testing.T) {
	perms := service.DefaultPermissions(models.AgentRoleCEO)
	if perms == "{}" || perms == "" {
		t.Fatal("expected non-empty permissions for CEO")
	}
}

func TestDefaultPermissions_Worker(t *testing.T) {
	perms := service.DefaultPermissions(models.AgentRoleWorker)
	if perms == "{}" || perms == "" {
		t.Fatal("expected non-empty permissions for worker")
	}
}

func TestStatusTransition_Valid(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("status-test", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	// idle -> working
	updated, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusWorking, "")
	if err != nil {
		t.Fatalf("idle->working: %v", err)
	}
	if updated.Status != models.AgentStatusWorking {
		t.Errorf("status = %q, want working", updated.Status)
	}

	// working -> paused
	updated, err = svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "manual pause")
	if err != nil {
		t.Fatalf("working->paused: %v", err)
	}
	if updated.PauseReason != "manual pause" {
		t.Errorf("pause_reason = %q, want %q", updated.PauseReason, "manual pause")
	}

	// paused -> idle
	_, err = svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusIdle, "")
	if err != nil {
		t.Fatalf("paused->idle: %v", err)
	}
}

func TestStatusTransition_Invalid(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("bad-transition", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	// idle -> idle (same status, should be no-op / allowed)
	_, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusIdle, "")
	if err != nil {
		t.Fatalf("same status should be allowed: %v", err)
	}

	// Set to stopped, then try stopped -> working (not allowed)
	_, err = svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusStopped, "")
	if err != nil {
		t.Fatalf("idle->stopped: %v", err)
	}
	_, err = svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusWorking, "")
	if err == nil {
		t.Fatal("expected error for stopped->working transition")
	}
}

func TestUpdateAgent_CEOEnforcement(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	ceo := makeAgent("the-ceo", models.AgentRoleCEO)
	if err := svc.CreateAgentInstance(ctx, ceo); err != nil {
		t.Fatalf("create CEO: %v", err)
	}

	worker := makeAgent("worker", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, worker); err != nil {
		t.Fatalf("create worker: %v", err)
	}

	// Try to promote worker to CEO while one exists
	worker.Role = models.AgentRoleCEO
	err := svc.UpdateAgentInstance(ctx, worker)
	if err == nil {
		t.Fatal("expected error promoting to CEO when one already exists")
	}
}
