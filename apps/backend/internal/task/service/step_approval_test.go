package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// seedApprovalGatedReview makes step-review-target an approval-gated step
// whose on_turn_complete transition targets step-done.
func seedApprovalGatedReview(t *testing.T, ctx context.Context, svc *Service, repo *sqliterepo.Repository) {
	t.Helper()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-review-target"].Events.OnTurnComplete = []wfmodels.OnTurnCompleteAction{{
		Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]any{"step_id": "step-done", "requires_approval": true},
	}}
	getter.steps["step-done"] = &wfmodels.WorkflowStep{ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2}
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, ?)`,
		"step-done", "wf-source", "Done", 2); err != nil {
		t.Fatalf("seed step-done: %v", err)
	}
}

// completeStampedTurn starts a turn stamped with the task's current step entry
// and completes it.
func completeStampedTurn(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, sessionID, taskID string) {
	t.Helper()
	turn := &models.Turn{TaskSessionID: sessionID, TaskID: taskID}
	if _, err := repo.CreateTurnWithStepStamp(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if err := repo.CompleteTurn(ctx, turn.ID); err != nil {
		t.Fatalf("complete turn: %v", err)
	}
}

func recordStepEntry(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID, toStepID string) {
	t.Helper()
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO task_step_transitions
		(task_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, taskID, "wf-source", "step-source", "wf-source", toStepID, "manual", "human", 1, time.Now().UTC()); err != nil {
		t.Fatalf("record step entry: %v", err)
	}
}

func TestStepApprovalAwaitsATurnCompletedInTheGatedStepEntry(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedApprovalGatedReview(t, ctx, svc, repo)
	createMoveTask(t, ctx, repo, "task-gated", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-gated", "task-gated", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)

	status, err := svc.GetStepApprovalStatus(ctx, "session-gated")
	if err != nil || status.Awaiting {
		t.Fatalf("before any turn: status=%+v err=%v, want not awaiting", status, err)
	}

	completeStampedTurn(t, ctx, repo, "session-gated", "task-gated")
	status, err = svc.GetStepApprovalStatus(ctx, "session-gated")
	if err != nil || !status.Awaiting || status.Step.ID != "step-review-target" || status.NextStep == nil || status.NextStep.ID != "step-done" {
		t.Fatalf("after a completed turn: status=%+v err=%v, want awaiting review -> done", status, err)
	}

	// Re-entering the step starts a new entry; the earlier turn no longer counts.
	recordStepEntry(t, ctx, repo, "task-gated", "step-review-target")
	if status, err = svc.GetStepApprovalStatus(ctx, "session-gated"); err != nil || status.Awaiting {
		t.Fatalf("after re-entry: status=%+v err=%v, want not awaiting", status, err)
	}
}

func TestStepApprovalRules(t *testing.T) {
	cases := []struct {
		name     string
		state    models.TaskSessionState
		review   models.ReviewStatus
		withTurn bool
		want     bool
	}{
		{name: "pending review awaits without a turn", state: models.TaskSessionStateWaitingForInput, review: models.ReviewStatusPending, want: true},
		{name: "approved review does not await", state: models.TaskSessionStateWaitingForInput, review: models.ReviewStatusApproved, withTurn: true},
		{name: "running session does not await", state: models.TaskSessionStateRunning, review: models.ReviewStatusPending, withTurn: true},
		{name: "starting session does not await", state: models.TaskSessionStateStarting, withTurn: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedApprovalGatedReview(t, ctx, svc, repo)
			createMoveTask(t, ctx, repo, "task-rule", "wf-source", "step-review-target", nil)
			createMoveSession(t, ctx, repo, "session-rule", "task-rule", tc.state, tc.review)
			if tc.withTurn {
				completeStampedTurn(t, ctx, repo, "session-rule", "task-rule")
			}
			status, err := svc.GetStepApprovalStatus(ctx, "session-rule")
			if err != nil || status.Awaiting != tc.want {
				t.Fatalf("status=%+v err=%v, want awaiting=%v", status, err, tc.want)
			}
		})
	}
}

func TestStepApprovalOfficeStageIsNotSessionApproval(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedApprovalGatedReview(t, ctx, svc, repo)
	svc.workflowStepGetter.(*fakeWorkflowStepGetter).steps["step-review-target"].StageType = wfmodels.StageTypeApproval
	createMoveTask(t, ctx, repo, "task-office", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-office", "task-office", models.TaskSessionStateWaitingForInput, models.ReviewStatusPending)

	if _, err := svc.GetStepApprovalStatus(ctx, "session-office"); !errors.Is(err, ErrOfficeApprovalStage) {
		t.Fatalf("office stage err = %v, want ErrOfficeApprovalStage", err)
	}
}

func TestApproveAwaitingStepMovesOnceThroughApproveSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedApprovalGatedReview(t, ctx, svc, repo)
	createMoveTask(t, ctx, repo, "task-approve", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-approve", "task-approve", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	completeStampedTurn(t, ctx, repo, "session-approve", "task-approve")

	guardErr := errors.New("stale")
	if _, _, err := svc.ApproveAwaitingStep(ctx, "session-approve", func(*StepApprovalStatus) error { return guardErr }); !errors.Is(err, guardErr) {
		t.Fatalf("guarded approval err = %v, want the guard's error", err)
	}
	status, result, err := svc.ApproveAwaitingStep(ctx, "session-approve", nil)
	if err != nil || status.NextStep.ID != "step-done" || result.Task == nil || result.Task.WorkflowStepID != "step-done" {
		t.Fatalf("approve = status %+v result %+v err %v, want a move to step-done", status, result, err)
	}
	session, err := repo.GetTaskSession(ctx, "session-approve")
	if err != nil || session.ReviewStatus != models.ReviewStatusApproved {
		t.Fatalf("session review status = %+v err=%v, want approved", session, err)
	}
	if _, _, err := svc.ApproveAwaitingStep(ctx, "session-approve", nil); !errors.Is(err, ErrStepNotAwaitingApproval) {
		t.Fatalf("second approval err = %v, want ErrStepNotAwaitingApproval", err)
	}
}
