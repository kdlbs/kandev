package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.1
func TestOrdinaryUpdateMissingWorkflowStepReturnsTypedNotFound(t *testing.T) {
	repo, _ := setupTestRepoWithDB(t)
	step := &models.WorkflowStep{
		ID:         "missing-step",
		WorkflowID: "wf-test",
		Name:       "Ready",
	}

	err := repo.UpdateStep(context.Background(), step)
	if !errors.Is(err, models.ErrWorkflowStepNotFound) {
		t.Fatalf("UpdateStep error = %v, want ErrWorkflowStepNotFound", err)
	}
}

// @covers AC-TASKS-WORKFLOW-STEP-MUTATION-ERRORS-001.1
func TestDeleteMissingWorkflowStepReturnsTypedNotFound(t *testing.T) {
	repo, _ := setupTestRepoWithDB(t)

	err := repo.DeleteStep(context.Background(), "missing-step")
	if !errors.Is(err, models.ErrWorkflowStepNotFound) {
		t.Fatalf("DeleteStep error = %v, want ErrWorkflowStepNotFound", err)
	}
}
