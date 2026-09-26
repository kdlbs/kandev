package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestUpdateProjectConfigFields_UpdatesOnlyOwnedColumns proves the
// column-scoped writer touches only description, color, budget_cents,
// repositories, and executor_config - leaving status and
// lead_agent_profile_id, which a config import does not own, exactly as
// they were.
func TestUpdateProjectConfigFields_UpdatesOnlyOwnedColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := &models.Project{
		ID:                 "project-1",
		WorkspaceID:        "ws-1",
		Name:               "apollo",
		Description:        "moon shot",
		Status:             models.ProjectStatusActive,
		LeadAgentProfileID: "agent-1",
		Color:              "#ff00ff",
		BudgetCents:        99000,
		Repositories:       `["kdlbs/kandev"]`,
		ExecutorConfig:     `{"type":"local_docker"}`,
	}
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	fields := sqlite.ProjectConfigFields{
		Description:    "mars shot",
		Color:          "#00ff00",
		BudgetCents:    1234,
		Repositories:   `["kdlbs/other"]`,
		ExecutorConfig: `{"type":"local_pc"}`,
	}
	if err := repo.UpdateProjectConfigFields(ctx, project.ID, fields); err != nil {
		t.Fatalf("UpdateProjectConfigFields: %v", err)
	}

	got, err := repo.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	// Owned columns landed.
	if got.Description != fields.Description {
		t.Errorf("Description = %q, want %q", got.Description, fields.Description)
	}
	if got.Color != fields.Color {
		t.Errorf("Color = %q, want %q", got.Color, fields.Color)
	}
	if got.BudgetCents != fields.BudgetCents {
		t.Errorf("BudgetCents = %d, want %d", got.BudgetCents, fields.BudgetCents)
	}
	if got.Repositories != fields.Repositories {
		t.Errorf("Repositories = %q, want %q", got.Repositories, fields.Repositories)
	}
	if got.ExecutorConfig != fields.ExecutorConfig {
		t.Errorf("ExecutorConfig = %q, want %q", got.ExecutorConfig, fields.ExecutorConfig)
	}

	// Everything else stayed exactly as seeded.
	if got.Status != project.Status {
		t.Errorf("Status changed: got %q, want %q", got.Status, project.Status)
	}
	if got.LeadAgentProfileID != project.LeadAgentProfileID {
		t.Errorf("LeadAgentProfileID changed: got %q, want %q", got.LeadAgentProfileID, project.LeadAgentProfileID)
	}
}
