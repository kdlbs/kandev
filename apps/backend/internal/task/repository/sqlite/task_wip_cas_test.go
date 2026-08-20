package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedCASWorkflowStep inserts a bare workflow step row for the compare-and-
// swap admission tests below, which only need CreateTask's WorkflowStepID
// foreign key to resolve, not a full workflow.
func seedCASWorkflowStep(t *testing.T, repo *Repository, workflowID, stepID string, position int) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`),
		stepID, workflowID, stepID, position, now, now); err != nil {
		t.Fatalf("seed workflow step %s: %v", stepID, err)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_AppliesOnlyWhenExpectedStepMatches
// is the AC-46/48 functional (non-concurrent) coverage for the new CAS
// repository method: applied=true and the admission/queue semantics of the
// unconditional UpdateTaskWithWorkflowStepAdmission are preserved when the
// precondition holds, and applied=false with the task row left untouched
// when it does not.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_AppliesOnlyWhenExpectedStepMatches(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas", WorkspaceID: "workspace-cas", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas", "step-target", 1)

	task := &models.Task{
		ID: "task-cas-1", WorkspaceID: "workspace-cas", WorkflowID: "workflow-cas",
		WorkflowStepID: "step-source", Title: "CAS candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Mismatched expectedStepID: the task has already left "step-source" from
	// this call's point of view (it's still there, but we assert a different
	// precondition) — applied must be false, no error, and the row untouched.
	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, "not-the-current-step", "step-target", 0)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep (mismatch): %v", err)
	}
	if applied {
		t.Fatalf("expected applied=false when expectedStepID does not match the persisted step")
	}
	reloaded, err := repo.GetTask(ctx, "task-cas-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-source" {
		t.Fatalf("expected task to remain at step-source after a lost CAS, got %q", reloaded.WorkflowStepID)
	}

	// Matching expectedStepID: applied=true and the move lands, same as the
	// unconditional variant.
	applied, err = repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, "step-source", "step-target", 0)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep (match): %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true when expectedStepID matches the persisted step")
	}
	reloaded, err = repo.GetTask(ctx, "task-cas-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-target" || !reloaded.WIPAdmitted {
		t.Fatalf("expected task admitted at step-target, got %+v", reloaded)
	}
}

func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_PreservesConcurrentTaskEdits(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas-preserve")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas-preserve", WorkspaceID: "workspace-cas-preserve", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas-preserve", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas-preserve", "step-target", 1)

	task := &models.Task{
		ID: "task-cas-preserve", WorkspaceID: "workspace-cas-preserve", WorkflowID: "workflow-cas-preserve",
		WorkflowStepID: "step-source", Title: "original", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	stale, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	current, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask current: %v", err)
	}
	current.Title = "edited concurrently"
	if err := repo.UpdateTask(ctx, current); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, stale, "step-source", "step-target", 0)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep: %v", err)
	}
	if !applied {
		t.Fatal("expected the step compare-and-swap to apply")
	}
	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after CAS: %v", err)
	}
	if reloaded.Title != "edited concurrently" {
		t.Fatalf("title = %q, want concurrent edit preserved", reloaded.Title)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_QueuesWhenStepIsFull mirrors
// the unconditional admission method's "limited full target queues instead
// of rejecting" behavior (it never rejects for WIP capacity, only for the
// CAS precondition), proving the CAS variant did not regress that contract.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_QueuesWhenStepIsFull(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas-full")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas-full", WorkspaceID: "workspace-cas-full", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas-full", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas-full", "step-target", 1)

	occupant := &models.Task{
		ID: "task-cas-occupant", WorkspaceID: "workspace-cas-full", WorkflowID: "workflow-cas-full",
		WorkflowStepID: "step-target", Title: "Occupant", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, occupant); err != nil {
		t.Fatalf("CreateTask(occupant): %v", err)
	}
	candidate := &models.Task{
		ID: "task-cas-candidate", WorkspaceID: "workspace-cas-full", WorkflowID: "workflow-cas-full",
		WorkflowStepID: "step-source", Title: "Candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, candidate); err != nil {
		t.Fatalf("CreateTask(candidate): %v", err)
	}

	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, candidate, "step-source", "step-target", 1)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true: the CAS precondition held even though the step is at capacity")
	}
	reloaded, err := repo.GetTask(ctx, "task-cas-candidate")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-target" {
		t.Fatalf("expected the task to move into step-target (queued, not rejected), got %q", reloaded.WorkflowStepID)
	}
	if reloaded.WIPAdmitted {
		t.Fatalf("expected WIPAdmitted=false for a queued task at a full step")
	}
	if reloaded.QueuedForStepID != "step-target" {
		t.Fatalf("expected QueuedForStepID=step-target, got %q", reloaded.QueuedForStepID)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_TaskNotFound preserves the
// ErrTaskNotFound sentinel the unconditional variant guarantees, for a task
// deleted concurrently with the CAS attempt.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_TaskNotFound(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	missing := &models.Task{ID: "task-cas-missing", WorkspaceID: "does-not-exist"}
	_, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, missing, "step-source", "step-target", 0)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace proves two
// concurrent CAS attempts against the same expected-step never both apply on
// SQLite. Unlike the PostgreSQL row-lock test
// (TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot), this
// does not exercise a dialect-specific row lock (SQLite's admission path
// takes no explicit lock — see updateTaskWithWorkflowStepAdmission) but
// SQLite's single-writer semantics still serialize the two write
// transactions, so at most one CAS precondition can observe "step-source"
// and win.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas-race")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas-race", WorkspaceID: "workspace-cas-race", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas-race", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas-race", "step-target", 1)

	task := &models.Task{
		ID: "task-cas-race", WorkspaceID: "workspace-cas-race", WorkflowID: "workflow-cas-race",
		WorkflowStepID: "step-source", Title: "Race candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	const attempts = 8
	start := make(chan struct{})
	results := make(chan struct {
		applied bool
		err     error
	}, attempts)
	done := make(chan struct{})
	for i := 0; i < attempts; i++ {
		go func() {
			<-start
			// Each goroutine loads its own copy so concurrent in-memory
			// mutation of a shared *models.Task is not itself the source of
			// any observed serialization.
			own, err := repo.GetTask(ctx, "task-cas-race")
			if err != nil {
				results <- struct {
					applied bool
					err     error
				}{false, err}
				return
			}
			applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, own, "step-source", "step-target", 0)
			results <- struct {
				applied bool
				err     error
			}{applied, err}
		}()
	}
	go func() {
		close(start)
		close(done)
	}()
	<-done

	appliedCount := 0
	for i := 0; i < attempts; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("concurrent CAS attempt: %v", r.err)
		}
		if r.applied {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("expected exactly one concurrent CAS attempt to apply, got %d", appliedCount)
	}
	reloaded, err := repo.GetTask(ctx, "task-cas-race")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-target" {
		t.Fatalf("expected the task to have moved to step-target exactly once, got %q", reloaded.WorkflowStepID)
	}
}
