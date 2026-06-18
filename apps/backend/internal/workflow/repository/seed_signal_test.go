package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// TestSeedDefaultWorkflowSteps_InProgressGatesOnSignal verifies that the kanban
// steps seeded into a step-less workflow carry auto_advance_requires_signal=true
// on In Progress, so the default workflow advances to Review on an explicit
// completion signal rather than the agent's first bare turn-end. Guards the
// hand-rolled seed INSERT against silently dropping the column.
func TestSeedDefaultWorkflowSteps_InProgressGatesOnSignal(t *testing.T) {
	repo := setupTestRepo(t)

	steps, err := repo.ListStepsByWorkflow(context.Background(), "wf-test")
	if err != nil {
		t.Fatalf("ListStepsByWorkflow: %v", err)
	}

	var inProgress *models.WorkflowStep
	for _, s := range steps {
		if s.Name == "In Progress" {
			inProgress = s
			break
		}
	}
	if inProgress == nil {
		t.Fatal("seeded kanban workflow has no In Progress step")
	}
	if !inProgress.AutoAdvanceRequiresSignal {
		t.Error("seeded In Progress must have auto_advance_requires_signal=true so it advances on the completion signal, not the first bare turn-end")
	}
}
