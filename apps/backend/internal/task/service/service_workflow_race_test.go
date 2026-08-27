package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// workflowMoveRaceRepo wraps the real sqlite repository and, on the first
// call to UpdateTaskWithWorkflowStepAdmissionAndState, runs an injected
// callback before delegating to the real implementation. This simulates a
// concurrent, legitimate reassignment landing between MoveTaskWithOptions'
// own GetTask (service_workflow.go's pre-write fast-fail check) and its own
// write — a race landing INSIDE a single MoveTaskWithOptions call, as
// distinct from TestConcurrentReassignmentSurvivesMatchingStepStaleMove
// (backendapp package), which only covers a race landing entirely before the
// call starts. The injected callback runs on the same wrapped repository
// (guarded by injected, so its own recursive write goes straight to the real
// embedded method rather than re-triggering the injection).
type workflowMoveRaceRepo struct {
	*sqliterepo.Repository
	inject   func()
	injected bool
}

func (r *workflowMoveRaceRepo) UpdateTaskWithWorkflowStepAdmissionAndState(
	ctx context.Context,
	task *models.Task,
	targetStepID string,
	limit int,
	admittedState *v1.TaskState,
	queueExitPending bool,
	expectedWorkflowID string,
) (bool, error) {
	if !r.injected {
		r.injected = true
		r.inject()
	}
	return r.Repository.UpdateTaskWithWorkflowStepAdmissionAndState(
		ctx, task, targetStepID, limit, admittedState, queueExitPending, expectedWorkflowID,
	)
}

// TestMoveTaskWithOptions_RaceLandingInsideCallIsRejectedByWriteTimeGuard
// closes the gap Review round 3 flagged: every existing CAS test (including
// TestConcurrentReassignmentSurvivesMatchingStepStaleMove) only proves a
// stale ExpectedWorkflowID is caught when the concurrent reassignment lands
// BEFORE MoveTaskWithOptions starts. None of them prove that MoveTaskWithOptions'
// own unlocked pre-write check (service_workflow.go, comparing
// opts.ExpectedWorkflowID against a plain GetTask) cannot itself close: a
// reassignment landing AFTER that check has already passed but BEFORE this
// call's own write commits.
//
// Here the concurrent, legitimate move is injected via workflowMoveRaceRepo
// at the exact point MoveTaskWithOptions reaches the repository write —
// strictly after its own pre-write check already passed (the check compares
// against wf-source, which is still true when it runs) and strictly before
// its own write executes. Only the atomic in-transaction recheck inside
// UpdateTaskWithWorkflowStepAdmissionAndState (updateTaskTx's
// expectedWorkflowID comparison, sqlite/task.go) can catch this — proving
// that check, not the outer fast-fail, is what closes the race.
func TestMoveTaskWithOptions_RaceLandingInsideCallIsRejectedByWriteTimeGuard(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-race-in-call", "wf-source", "step-source", nil)

	raceRepo := &workflowMoveRaceRepo{Repository: repo}
	raceRepo.inject = func() {
		if _, err := svc.MoveTaskWithOptions(ctx, "task-race-in-call", "wf-target", "step-target", 0, MoveTaskOptions{}); err != nil {
			t.Fatalf("concurrent legitimate reassignment (injected mid-call) failed: %v", err)
		}
	}
	svc.tasks = raceRepo

	staleWorkflowID := "wf-source"
	_, err := svc.MoveTaskWithOptions(ctx, "task-race-in-call", "wf-source", "step-review-target", 0, MoveTaskOptions{
		ExpectedWorkflowID: &staleWorkflowID,
	})
	if !errors.Is(err, ErrWorkflowResolutionConflict) {
		t.Fatalf("stale move: got err %v, want ErrWorkflowResolutionConflict", err)
	}

	final, err := repo.GetTask(ctx, "task-race-in-call")
	if err != nil {
		t.Fatalf("GetTask after race: %v", err)
	}
	if final.WorkflowID != "wf-target" || final.WorkflowStepID != "step-target" {
		t.Fatalf("concurrent reassignment must survive the rejected stale move: got (%s, %s), want (wf-target, step-target)",
			final.WorkflowID, final.WorkflowStepID)
	}
}
