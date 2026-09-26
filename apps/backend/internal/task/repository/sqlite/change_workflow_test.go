package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func seedWorkflowChangeRows(t *testing.T, repo *Repository, taskID string) *models.Task {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-workflow-change")
	for _, workflow := range []*models.Workflow{
		{ID: "workflow-change-source", WorkspaceID: "workspace-workflow-change", Name: "Source"},
		{ID: "workflow-change-target", WorkspaceID: "workspace-workflow-change", Name: "Target"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow(%s): %v", workflow.ID, err)
		}
	}
	seedCASWorkflowStep(t, repo, "workflow-change-source", "workflow-change-source-step", 0)
	seedCASWorkflowStep(t, repo, "workflow-change-target", "workflow-change-target-step", 0)
	oldOverrides, err := models.NewWorkflowAgentOverrides("workflow-change-source", []models.WorkflowAgentOverrideBinding{{
		StepID: "workflow-change-source-step", SourceProfileID: "source-old", ReplacementProfileID: "replacement-old",
	}})
	if err != nil {
		t.Fatalf("NewWorkflowAgentOverrides: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "workspace-workflow-change",
		WorkflowID: "workflow-change-source", WorkflowStepID: "workflow-change-source-step",
		Title: "Original title", Description: "Original description", Priority: "medium",
		Metadata: map[string]interface{}{"sentinel": "keep"}, WorkflowAgentOverrides: oldOverrides,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return task
}

func workflowChangeCandidate(t *testing.T, task *models.Task) *models.Task {
	t.Helper()
	candidate := *task
	candidate.WorkflowID = "workflow-change-target"
	candidate.WorkflowStepID = "workflow-change-target-step"
	candidate.WorkflowAgentOverrides, _ = models.NewWorkflowAgentOverrides("workflow-change-target", []models.WorkflowAgentOverrideBinding{{
		StepID: "workflow-change-target-step", SourceProfileID: "source-new", ReplacementProfileID: "replacement-new",
	}})
	return &candidate
}

func TestWorkflowChangeAtomicAdmissionPersistsMembershipAndOverridesTogether(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	source := seedWorkflowChangeRows(t, repo, "task-workflow-change-atomic")
	guard := &models.WorkflowChangeSource{WorkflowID: source.WorkflowID, StepID: source.WorkflowStepID, UpdatedAt: source.UpdatedAt}
	candidate := workflowChangeCandidate(t, source)
	if _, err := repo.UpdateTaskWithWorkflowChangeAdmissionAndState(
		ctx, candidate, source.WorkflowStepID, candidate.WorkflowStepID, 0, nil, true, guard,
	); err != nil {
		t.Fatalf("UpdateTaskWithWorkflowChangeAdmissionAndState: %v", err)
	}
	stored, err := repo.GetTask(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetTask after change: %v", err)
	}
	if stored.WorkflowID != candidate.WorkflowID || stored.WorkflowStepID != candidate.WorkflowStepID {
		t.Fatalf("stored destination = %s/%s", stored.WorkflowID, stored.WorkflowStepID)
	}
	if stored.WorkflowAgentOverrides == nil || stored.WorkflowAgentOverrides.WorkflowID != candidate.WorkflowID {
		t.Fatalf("stored overrides = %+v", stored.WorkflowAgentOverrides)
	}
	if replacement, ok := stored.WorkflowAgentOverrides.ReplacementFor(candidate.WorkflowID, candidate.WorkflowStepID); !ok || replacement != "replacement-new" {
		t.Fatalf("stored destination replacement = %q, %v", replacement, ok)
	}
	if stored.Title != "Original title" || stored.Description != "Original description" || stored.Metadata["sentinel"] != "keep" {
		t.Fatalf("task context changed: %+v", stored)
	}
}

func TestWorkflowChangeAtomicAdmissionRejectsConcurrentTaskEdit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	source := seedWorkflowChangeRows(t, repo, "task-workflow-change-stale")
	staleGuard := &models.WorkflowChangeSource{WorkflowID: source.WorkflowID, StepID: source.WorkflowStepID, UpdatedAt: source.UpdatedAt}
	candidate := workflowChangeCandidate(t, source)

	current, err := repo.GetTask(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetTask before concurrent edit: %v", err)
	}
	current.Description = "Concurrent edit"
	if err := repo.UpdateTask(ctx, current); err != nil {
		t.Fatalf("UpdateTask concurrent edit: %v", err)
	}
	if _, err := repo.UpdateTaskWithWorkflowChangeAdmissionAndState(
		ctx, candidate, source.WorkflowStepID, candidate.WorkflowStepID, 0, nil, true, staleGuard,
	); !errors.Is(err, repoerrors.ErrWorkflowChangeConflict) {
		t.Fatalf("stale guarded write error = %v, want ErrWorkflowChangeConflict", err)
	}
	stored, err := repo.GetTask(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetTask after conflict: %v", err)
	}
	if stored.WorkflowID != source.WorkflowID || stored.WorkflowStepID != source.WorkflowStepID || stored.Description != "Concurrent edit" {
		t.Fatalf("stale change overwrote current task: %+v", stored)
	}
	if stored.WorkflowAgentOverrides == nil || stored.WorkflowAgentOverrides.WorkflowID != source.WorkflowID {
		t.Fatalf("stale change replaced source overrides: %+v", stored.WorkflowAgentOverrides)
	}
}

func TestWorkflowChangeAtomicAdmissionRollsBackOnTaskWriteFailure(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	source := seedWorkflowChangeRows(t, repo, "task-workflow-change-write-failure")
	guard := &models.WorkflowChangeSource{WorkflowID: source.WorkflowID, StepID: source.WorkflowStepID, UpdatedAt: source.UpdatedAt}
	candidate := workflowChangeCandidate(t, source)
	if _, err := repo.db.Exec(`CREATE TRIGGER fail_workflow_change BEFORE UPDATE ON tasks
		WHEN OLD.id = 'task-workflow-change-write-failure'
		BEGIN SELECT RAISE(ABORT, 'injected workflow change failure'); END`); err != nil {
		t.Fatalf("create injected failure trigger: %v", err)
	}
	_, err := repo.UpdateTaskWithWorkflowChangeAdmissionAndState(
		ctx, candidate, source.WorkflowStepID, candidate.WorkflowStepID, 0, nil, true, guard,
	)
	if err == nil {
		t.Fatal("guarded write succeeded despite injected task update failure")
	}
	if _, err := repo.db.Exec(`DROP TRIGGER fail_workflow_change`); err != nil {
		t.Fatalf("drop injected failure trigger: %v", err)
	}
	stored, err := repo.GetTask(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetTask after rollback: %v", err)
	}
	if stored.WorkflowID != source.WorkflowID || stored.WorkflowStepID != source.WorkflowStepID ||
		stored.WorkflowAgentOverrides == nil || stored.WorkflowAgentOverrides.WorkflowID != source.WorkflowID {
		t.Fatalf("failed change partially committed: %+v", stored)
	}
}
