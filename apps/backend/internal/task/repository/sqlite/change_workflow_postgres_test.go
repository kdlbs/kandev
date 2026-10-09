package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestPostgresWorkflowChangeAtomicAdmissionGuardsSourceVersion(t *testing.T) {
	repoA, repoB, _ := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	source := seedWorkflowChangeRows(t, repoA, "task-workflow-change-postgres")
	guard := &models.WorkflowChangeSource{WorkflowID: source.WorkflowID, StepID: source.WorkflowStepID, UpdatedAt: source.UpdatedAt}
	candidate := workflowChangeCandidate(t, source)

	concurrent, err := repoB.GetTask(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetTask before concurrent edit: %v", err)
	}
	concurrent.Description = "Concurrent PostgreSQL edit"
	if err := repoB.UpdateTask(ctx, concurrent); err != nil {
		t.Fatalf("UpdateTask concurrent edit: %v", err)
	}
	if _, err := repoA.UpdateTaskWithWorkflowChangeAdmissionAndState(
		ctx, candidate, source.WorkflowStepID, candidate.WorkflowStepID, 0, nil, true, guard,
	); !errors.Is(err, repoerrors.ErrWorkflowChangeConflict) {
		t.Fatalf("stale guarded write error = %v, want ErrWorkflowChangeConflict", err)
	}

	current, err := repoA.GetTask(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetTask after stale change: %v", err)
	}
	if current.WorkflowID != source.WorkflowID || current.WorkflowStepID != source.WorkflowStepID || current.Description != "Concurrent PostgreSQL edit" {
		t.Fatalf("stale change overwrote current task: %+v", current)
	}

	currentGuard := &models.WorkflowChangeSource{WorkflowID: current.WorkflowID, StepID: current.WorkflowStepID, UpdatedAt: current.UpdatedAt}
	currentCandidate := workflowChangeCandidate(t, current)
	if _, err := repoA.UpdateTaskWithWorkflowChangeAdmissionAndState(
		ctx, currentCandidate, current.WorkflowStepID, currentCandidate.WorkflowStepID, 0, nil, true, currentGuard,
	); err != nil {
		t.Fatalf("current guarded write: %v", err)
	}
	stored, err := repoB.GetTask(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetTask after successful change: %v", err)
	}
	if stored.WorkflowID != currentCandidate.WorkflowID || stored.WorkflowStepID != currentCandidate.WorkflowStepID {
		t.Fatalf("stored destination = %s/%s", stored.WorkflowID, stored.WorkflowStepID)
	}
	if stored.WorkflowAgentOverrides == nil || stored.WorkflowAgentOverrides.WorkflowID != currentCandidate.WorkflowID {
		t.Fatalf("stored destination overrides = %+v", stored.WorkflowAgentOverrides)
	}
}
