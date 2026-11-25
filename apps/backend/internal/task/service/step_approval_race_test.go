package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const approvalMovedAwayStep = "step-source"

// A task that leaves the observed step between the status read and the write
// must not be advanced from the step the guard never saw.
func TestApproveAwaitingStepRejectsATaskThatMovedAfterTheObservation(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedApprovalGatedReview(t, ctx, svc, repo)
	createMoveTask(t, ctx, repo, "task-race", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-race", "task-race", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	completeStampedTurn(t, ctx, repo, "session-race", "task-race")

	moveAway := func(*StepApprovalStatus) error {
		if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, approvalMovedAwayStep, "task-race"); err != nil {
			t.Fatalf("move task away: %v", err)
		}
		return nil
	}
	_, result, err := svc.ApproveAwaitingStep(ctx, "session-race", moveAway)
	if !errors.Is(err, ErrStepApprovalChanged) || result != nil {
		t.Fatalf("approve = result %+v err %v, want ErrStepApprovalChanged", result, err)
	}
	task, err := repo.GetTask(ctx, "task-race")
	if err != nil || task.WorkflowStepID != approvalMovedAwayStep {
		t.Fatalf("task = %+v err=%v, want it left at %s", task, err, approvalMovedAwayStep)
	}
	session, err := repo.GetTaskSession(ctx, "session-race")
	if err != nil || session.ReviewStatus == models.ReviewStatusApproved {
		t.Fatalf("session review status = %+v err=%v, want it not approved", session, err)
	}
}

// The write itself re-checks the step inside its transaction, so a race that
// lands after every earlier read still leaves the task row untouched.
func TestStepGuardedMoveLeavesTheTaskWhenItAlreadyLeftTheExpectedStep(t *testing.T) {
	_, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	createMoveTask(t, ctx, repo, "task-cas", "wf-source", "step-review-target", nil)

	task, err := repo.GetTask(ctx, "task-cas")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkflowStepID = "step-done"
	_, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, task, "step-done", 0, nil, true, approvalMovedAwayStep, "", nil)
	if err != nil || applied {
		t.Fatalf("applied = %v err = %v, want a no-op for a stale expected step", applied, err)
	}
	stored, err := repo.GetTask(ctx, "task-cas")
	if err != nil || stored.WorkflowStepID != "step-review-target" {
		t.Fatalf("stored task = %+v err=%v, want it unchanged", stored, err)
	}
}

type failingReviewStatusSessions struct {
	repository.SessionRepository
}

func (failingReviewStatusSessions) UpdateSessionReviewStatus(context.Context, string, string) error {
	return errors.New("review status write failed")
}

// A committed move followed by a failed review-status write is reported as an
// applied approval, so a replayed idempotency key never stores a failure for a
// move that already happened.
func TestApproveAwaitingStepReportsAppliedWhenOnlyTheReviewStatusWriteFails(t *testing.T) {
	svc, _, repo := createTestServiceWithSessionsRepo(t, func(repo *sqliterepo.Repository) repository.SessionRepository {
		return failingReviewStatusSessions{SessionRepository: repo}
	})
	ctx := context.Background()
	seedApprovalGatedReview(t, ctx, svc, repo)
	createMoveTask(t, ctx, repo, "task-partial", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-partial", "task-partial", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	completeStampedTurn(t, ctx, repo, "session-partial", "task-partial")

	_, result, err := svc.ApproveAwaitingStep(ctx, "session-partial", nil)
	if err != nil || result == nil || result.Task == nil || result.Task.WorkflowStepID != "step-done" {
		t.Fatalf("approve = result %+v err %v, want an applied move to step-done", result, err)
	}
}
