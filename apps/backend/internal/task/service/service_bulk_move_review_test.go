package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type bulkMoveReviewRepository struct {
	*sqliterepo.Repository
	failWriteTaskID string
	writeErr        error
	failRefreshTask string
	refreshErr      error
	failNextGet     bool
	getTaskCalls    int
}

func (r *bulkMoveReviewRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	r.getTaskCalls++
	if r.failNextGet && id == r.failRefreshTask {
		r.failNextGet = false
		return nil, r.refreshErr
	}
	return r.Repository.GetTask(ctx, id)
}

func (r *bulkMoveReviewRepository) UpdateTaskWithWorkflowStepAdmissionAndStateWithCompletionAdmission(
	ctx context.Context,
	task *models.Task,
	sourceStepID, targetStepID string,
	limit int,
	admittedState *v1.TaskState,
	queueExitPending bool,
	expectedWorkflowID string,
	admission *completion.Admission,
) (bool, error) {
	if task.ID == r.failWriteTaskID {
		return false, r.writeErr
	}
	admitted, err := r.Repository.UpdateTaskWithWorkflowStepAdmissionAndStateWithCompletionAdmission(
		ctx, task, sourceStepID, targetStepID, limit, admittedState, queueExitPending,
		expectedWorkflowID, admission,
	)
	if err == nil && task.ID == r.failRefreshTask {
		r.failNextGet = true
	}
	return admitted, err
}

func TestBulkMovePartialErrorRetainsUnexpectedCauseAfterEarlierCommit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedBulkCompletingTarget(t, ctx, svc, repo, func(string) bool { return false })
	createMoveTask(t, ctx, repo, "task-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-b", "wf-source", "step-source", nil)
	setMoveTaskOrder(t, ctx, repo, "task-a", 0, "medium")
	setMoveTaskOrder(t, ctx, repo, "task-b", 1, "medium")

	writeErr := errors.New("database secret: postgres://private")
	faultRepo := &bulkMoveReviewRepository{Repository: repo, failWriteTaskID: "task-b", writeErr: writeErr}
	svc.tasks = faultRepo
	_, err := svc.BulkMoveSelectedTasks(ctx, []string{"task-a", "task-b"}, "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
	var partial *BulkMovePartialError
	if !errors.As(err, &partial) {
		t.Fatalf("bulk move error = %T (%v), want BulkMovePartialError", err, err)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("partial error %v does not retain the write cause", err)
	}
	if len(partial.MovedTaskIDs) != 1 || partial.MovedTaskIDs[0] != "task-a" {
		t.Fatalf("moved IDs = %v, want [task-a]", partial.MovedTaskIDs)
	}
	if partial.Refusal.Code != "task_move_failed" || strings.Contains(partial.Refusal.Message, "postgres://private") {
		t.Fatalf("partial refusal = %+v, want safe task_move_failed refusal", partial.Refusal)
	}
	storedA, err := repo.GetTask(ctx, "task-a")
	if err != nil {
		t.Fatal(err)
	}
	storedB, err := repo.GetTask(ctx, "task-b")
	if err != nil {
		t.Fatal(err)
	}
	if storedA.WorkflowStepID != "step-target" || storedB.WorkflowStepID != "step-source" {
		t.Fatalf("stored steps: task-a=%s task-b=%s", storedA.WorkflowStepID, storedB.WorkflowStepID)
	}
}

func TestBulkMovePartialErrorIncludesCommitWhenRefreshFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedBulkCompletingTarget(t, ctx, svc, repo, func(string) bool { return false })
	createMoveTask(t, ctx, repo, "task-committed", "wf-source", "step-source", nil)

	refreshErr := errors.New("refresh failed: database secret")
	svc.tasks = &bulkMoveReviewRepository{
		Repository: repo, failRefreshTask: "task-committed", refreshErr: refreshErr,
	}
	_, err := svc.BulkMoveSelectedTasks(ctx, []string{"task-committed"}, "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
	var partial *BulkMovePartialError
	if !errors.As(err, &partial) {
		t.Fatalf("bulk move error = %T (%v), want committed partial result", err, err)
	}
	if !errors.Is(err, refreshErr) {
		t.Fatalf("partial error %v does not retain refresh error", err)
	}
	if len(partial.MovedTaskIDs) != 1 || partial.MovedTaskIDs[0] != "task-committed" {
		t.Fatalf("moved IDs = %v, want [task-committed]", partial.MovedTaskIDs)
	}
	stored, err := repo.GetTask(ctx, "task-committed")
	if err != nil {
		t.Fatal(err)
	}
	if stored.WorkflowStepID != "step-target" {
		t.Fatalf("stored step = %s, want committed target step", stored.WorkflowStepID)
	}
}

func TestBulkMoveAuthorizesEveryTaskBeforeCollectingRefusals(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	if err := repo.UpdateTaskSessionState(context.Background(), "sess-b", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	probe := &bulkMoveReviewRepository{Repository: repo}
	svc.tasks = probe

	_, err := svc.BulkMoveSelectedTasks(ctxAs("user-a"), []string{"task-b"}, "missing-workflow", "missing-step", wfmodels.StepTransitionActorHuman)
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign task bulk move error = %v, want not found before move refusal", err)
	}
	if probe.getTaskCalls != 1 {
		t.Fatalf("foreign task reads = %d, want only the authorization lookup", probe.getTaskCalls)
	}
	var refused *BulkMoveRefusedError
	if errors.As(err, &refused) {
		t.Fatalf("foreign task refusal details leaked: %+v", refused.Refusals)
	}
}
