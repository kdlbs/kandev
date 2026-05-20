package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestCreateAgent_WritesToDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "fs-agent",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := svc.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "fs-agent" {
		t.Errorf("name = %q, want fs-agent", got.Name)
	}
}

func TestUpdateAgent_WritesToDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "updatable",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}

	agent.BudgetMonthlyCents = 9999
	if err := svc.UpdateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := svc.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.BudgetMonthlyCents != 9999 {
		t.Errorf("budget = %d, want 9999", got.BudgetMonthlyCents)
	}
}

func TestDeleteAgent_RemovesFromDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "deletable",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.DeleteAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetAgentInstance(ctx, agent.ID); err == nil {
		t.Fatal("agent should have been deleted")
	}
}

func TestCreateProject_WritesToDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "fs-project",
		Description: "test project",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description != "test project" {
		t.Errorf("description = %q, want test project", got.Description)
	}
}

func TestUpdateProject_WritesToDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "updatable-proj",
		Description: "original",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}
	project.Description = "updated description"
	if err := svc.UpdateProject(ctx, project); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := svc.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description != "updated description" {
		t.Errorf("description = %q, want updated description", got.Description)
	}
}

func TestDeleteProject_RemovesFromDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	project := &models.Project{
		WorkspaceID: "ws-1",
		Name:        "deletable-proj",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.DeleteProject(ctx, project.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetProject(ctx, project.ID); err == nil {
		t.Fatal("project should have been deleted")
	}
}

func TestCreateRoutine_WritesToDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID: "ws-1",
		Name:        "fs-routine",
		Description: "test routine",
		Status:      "active",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Description != "test routine" {
		t.Errorf("description = %q, want test routine", got.Description)
	}
}

func TestDeleteRoutine_RemovesFromDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID: "ws-1",
		Name:        "deletable-routine",
		Status:      "active",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.DeleteRoutine(ctx, routine.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetRoutine(ctx, routine.ID); err == nil {
		t.Fatal("routine should have been deleted")
	}
}

func TestUpdateSkill_WritesToDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "My Skill",
		Slug:        "my-skill",
		SourceType:  "inline",
		Content:     "# Original",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}
	skill.Content = "# Updated content"
	if err := svc.UpdateSkill(ctx, skill); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := svc.GetSkill(ctx, skill.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "# Updated content" {
		t.Errorf("content = %q, want # Updated content", got.Content)
	}
}

func TestDeleteSkill_RemovesFromDB(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	skill := &models.Skill{
		WorkspaceID: "ws-1",
		Name:        "Removable",
		Slug:        "removable",
		SourceType:  "inline",
		Content:     "# Remove me",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.DeleteSkill(ctx, skill.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetSkill(ctx, skill.ID); err == nil {
		t.Fatal("skill should have been deleted")
	}
}
