package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestResolveExecutor_TaskOverrideWins(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	agent.ExecutorPreference = `{"type":"local_docker","image":"node:20"}`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	taskPolicy := `{"executor_config":{"type":"sprites","image":"sandbox:latest"}}`
	cfg, err := svc.ResolveExecutor(ctx, taskPolicy, agent.ID, "", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cfg.Type != "sprites" {
		t.Errorf("type = %q, want sprites (task override wins)", cfg.Type)
	}
}

func TestResolveExecutor_AgentPreferenceWins(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	agent.ExecutorPreference = `{"type":"local_docker","image":"node:20"}`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	project := &models.Project{
		WorkspaceID:    "ws-1",
		Name:           "Backend",
		ExecutorConfig: `{"type":"sprites","image":"go:1.22"}`,
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	cfg, err := svc.ResolveExecutor(ctx, "{}", agent.ID, project.ID, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cfg.Type != "local_docker" {
		t.Errorf("type = %q, want local_docker (agent preference wins over project)", cfg.Type)
	}
}

func TestResolveExecutor_ProjectConfigWins(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	// No executor preference on agent.
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	project := &models.Project{
		WorkspaceID:    "ws-1",
		Name:           "Backend",
		ExecutorConfig: `{"type":"sprites","image":"go:1.22"}`,
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	cfg, err := svc.ResolveExecutor(ctx, "", agent.ID, project.ID, `{"type":"local_pc"}`)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cfg.Type != "sprites" {
		t.Errorf("type = %q, want sprites (project wins over workspace default)", cfg.Type)
	}
}

func TestResolveExecutor_WorkspaceDefault(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	cfg, err := svc.ResolveExecutor(ctx, "", agent.ID, "", `{"type":"local_pc"}`)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cfg.Type != "local_pc" {
		t.Errorf("type = %q, want local_pc (workspace default)", cfg.Type)
	}
}

func TestResolveExecutor_NoConfig(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	_, err := svc.ResolveExecutor(ctx, "", agent.ID, "", "")
	if err == nil {
		t.Fatal("expected error when no executor config found")
	}
}
