package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestUpdateRoutineConfigFields_UpdatesOnlyOwnedColumns proves the
// column-scoped writer touches only description, task_template, and
// concurrency_policy - leaving status and every other column, which a
// config import does not own, exactly as they were.
func TestUpdateRoutineConfigFields_UpdatesOnlyOwnedColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		ID:                     "routine-1",
		WorkspaceID:            "ws-1",
		Name:                   "standup",
		Description:            "daily standup",
		TaskTemplate:           "summarise yesterday",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      models.ConcurrencyPolicySkipIfActive,
		CatchUpPolicy:          "enqueue_missed_with_cap",
		CatchUpMax:             25,
		Variables:              `{"team":"platform"}`,
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}

	fields := sqlite.RoutineConfigFields{
		Description:       "weekly",
		TaskTemplate:      "summarise the week",
		ConcurrencyPolicy: models.ConcurrencyPolicyAlwaysCreate,
	}
	if err := repo.UpdateRoutineConfigFields(ctx, routine.ID, fields); err != nil {
		t.Fatalf("UpdateRoutineConfigFields: %v", err)
	}

	got, err := repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}

	// Owned columns landed.
	if got.Description != fields.Description {
		t.Errorf("Description = %q, want %q", got.Description, fields.Description)
	}
	if got.TaskTemplate != fields.TaskTemplate {
		t.Errorf("TaskTemplate = %q, want %q", got.TaskTemplate, fields.TaskTemplate)
	}
	if got.ConcurrencyPolicy != fields.ConcurrencyPolicy {
		t.Errorf("ConcurrencyPolicy = %q, want %q", got.ConcurrencyPolicy, fields.ConcurrencyPolicy)
	}

	// Everything else stayed exactly as seeded.
	if got.AssigneeAgentProfileID != routine.AssigneeAgentProfileID {
		t.Errorf("AssigneeAgentProfileID changed: got %q, want %q",
			got.AssigneeAgentProfileID, routine.AssigneeAgentProfileID)
	}
	if got.Status != routine.Status {
		t.Errorf("Status changed: got %q, want %q", got.Status, routine.Status)
	}
	if got.CatchUpPolicy != routine.CatchUpPolicy {
		t.Errorf("CatchUpPolicy changed: got %q, want %q", got.CatchUpPolicy, routine.CatchUpPolicy)
	}
	if got.CatchUpMax != routine.CatchUpMax {
		t.Errorf("CatchUpMax changed: got %d, want %d", got.CatchUpMax, routine.CatchUpMax)
	}
	if got.Variables != routine.Variables {
		t.Errorf("Variables changed: got %q, want %q", got.Variables, routine.Variables)
	}
}
