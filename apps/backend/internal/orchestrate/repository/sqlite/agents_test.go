package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestAgentInstance_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Create
	agent := &models.AgentInstance{
		WorkspaceID:           "ws-1",
		Name:                  "test-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		Permissions:           "{}",
		DesiredSkills:         "[]",
		ExecutorPreference:    "{}",
		MaxConcurrentSessions: 1,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}
	if agent.ID == "" {
		t.Fatal("expected ID to be set")
	}

	// Get
	got, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "test-agent" {
		t.Errorf("name = %q, want %q", got.Name, "test-agent")
	}
	if got.Role != models.AgentRoleWorker {
		t.Errorf("role = %q, want %q", got.Role, models.AgentRoleWorker)
	}

	// List
	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("list count = %d, want 1", len(agents))
	}

	// Update
	agent.Name = "renamed-agent"
	agent.Status = models.AgentStatusWorking
	if err := repo.UpdateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = repo.GetAgentInstance(ctx, agent.ID)
	if got.Name != "renamed-agent" {
		t.Errorf("updated name = %q, want %q", got.Name, "renamed-agent")
	}
	if got.Status != models.AgentStatusWorking {
		t.Errorf("updated status = %q, want %q", got.Status, models.AgentStatusWorking)
	}

	// Delete
	if err := repo.DeleteAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	agents, _ = repo.ListAgentInstances(ctx, "ws-1")
	if len(agents) != 0 {
		t.Errorf("list after delete = %d, want 0", len(agents))
	}
}

func TestAgentInstance_UniqueConstraint(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "unique-agent",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		Permissions:        "{}",
		DesiredSkills:      "[]",
		ExecutorPreference: "{}",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("first create: %v", err)
	}

	dup := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "unique-agent",
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		Permissions:        "{}",
		DesiredSkills:      "[]",
		ExecutorPreference: "{}",
	}
	if err := repo.CreateAgentInstance(ctx, dup); err == nil {
		t.Fatal("expected unique constraint error on duplicate name+workspace")
	}
}
