package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskAgentProjectIDRoundTrips(t *testing.T) {
	repo, _ := newDesktopDiscoveryTestRepo(t)
	ctx := context.Background()
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO agent_projects (id, workspace_id, name)
		VALUES (?, ?, ?)
	`, "project-agent-project", "workspace-agent-project", "Agent Project"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	created := &models.Task{
		ID:                    "task-agent-project",
		WorkspaceID:           "workspace-agent-project",
		AgentProjectID:        "project-agent-project",
		AgentProjectTier:      "coordinator",
		AgentProjectProfileID: "profile-coordinator",
		Title:                 "Project coordinator",
	}
	if err := repo.CreateTask(ctx, created); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	got, err := repo.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.AgentProjectID != created.AgentProjectID {
		t.Fatalf("AgentProjectID = %q, want %q", got.AgentProjectID, created.AgentProjectID)
	}
	if got.AgentProjectTier != created.AgentProjectTier || got.AgentProjectProfileID != created.AgentProjectProfileID {
		t.Fatalf("project launch policy = %q/%q, want %q/%q", got.AgentProjectTier, got.AgentProjectProfileID, created.AgentProjectTier, created.AgentProjectProfileID)
	}
}
