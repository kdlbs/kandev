package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestListAgentsFromConfig(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		Name: "test-agent", Role: models.AgentRoleWorker, WorkspaceID: "default",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	agents, err := svc.ListAgentsFromConfig(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != "test-agent" {
		t.Errorf("got %d agents, want 1 named test-agent", len(agents))
	}
}

func TestListSkillsFromConfig(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateSkill(ctx, &models.Skill{
		Name: "My Skill", Slug: "my-skill", WorkspaceID: "default", Content: "# My Skill",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	skills, err := svc.ListSkillsFromConfig(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(skills) != 1 || skills[0].Slug != "my-skill" {
		t.Errorf("got %d skills, want 1 with slug my-skill", len(skills))
	}
}

func TestListRoutinesFromConfig(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateRoutine(ctx, &models.Routine{
		ID: "routine-daily", Name: "daily", WorkspaceID: "default",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	routines, err := svc.ListRoutinesFromConfig(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(routines) != 1 || routines[0].Name != "daily" {
		t.Errorf("got %d routines, want 1 named daily", len(routines))
	}
}

func TestGetAgentFromConfig_ByName(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateAgentInstance(ctx, &models.AgentInstance{
		Name: "lookup", Role: models.AgentRoleWorker, WorkspaceID: "default",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	agent, err := svc.GetAgentFromConfig(ctx, "lookup")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if agent.Name != "lookup" {
		t.Errorf("name = %q, want lookup", agent.Name)
	}
}
