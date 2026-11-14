package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type bulkMoveTestCompletionPolicy struct {
	shouldRefuse func(taskID string) bool
}

func (p bulkMoveTestCompletionPolicy) AdmitCompletion(
	_ context.Context,
	_ completion.Querier,
	current, candidate *models.Task,
	_ completion.Admission,
) error {
	if candidate.State == v1.TaskStateCompleted && p.shouldRefuse(current.ID) {
		return &completion.RefusedError{Code: completion.CodeApprovalsPending, Subjects: []string{"approver-1"}}
	}
	return nil
}

func TestBulkMovePreflightRefusesWholeBatch(t *testing.T) {
	for _, bulkMove := range []struct {
		name string
		move func(*Service, context.Context) (*BulkMoveTasksResult, error)
	}{
		{
			name: "selected tasks",
			move: func(svc *Service, ctx context.Context) (*BulkMoveTasksResult, error) {
				return svc.BulkMoveSelectedTasks(ctx, []string{"task-a", "task-b", "task-c"}, "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
			},
		},
		{
			name: "source step",
			move: func(svc *Service, ctx context.Context) (*BulkMoveTasksResult, error) {
				return svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
			},
		},
	} {
		t.Run(bulkMove.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedBulkCompletingTarget(t, ctx, svc, repo, func(taskID string) bool { return taskID == "task-b" })
			for _, id := range []string{"task-a", "task-b", "task-c"} {
				createMoveTask(t, ctx, repo, id, "wf-source", "step-source", nil)
			}

			_, err := bulkMove.move(svc, ctx)
			var refused *BulkMoveRefusedError
			if !errors.As(err, &refused) {
				t.Fatalf("bulk move error = %v, want BulkMoveRefusedError", err)
			}
			if len(refused.Refusals) != 1 || refused.Refusals[0].TaskID != "task-b" || refused.Refusals[0].Code != bulkMoveApprovalsPendingCode {
				t.Fatalf("refusals = %+v, want task-b approval refusal", refused.Refusals)
			}

			for _, id := range []string{"task-a", "task-b", "task-c"} {
				task, readErr := repo.GetTask(ctx, id)
				if readErr != nil {
					t.Fatalf("GetTask(%s): %v", id, readErr)
				}
				if task.WorkflowID != "wf-source" || task.WorkflowStepID != "step-source" {
					t.Fatalf("%s moved despite preflight refusal: workflow=%s step=%s", id, task.WorkflowID, task.WorkflowStepID)
				}
			}
		})
	}
}

func TestBulkMoveReportsPartialResultAfterConcurrentRefusal(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	refuseTaskB := false
	seedBulkCompletingTarget(t, ctx, svc, repo, func(taskID string) bool { return refuseTaskB && taskID == "task-b" })
	createMoveTask(t, ctx, repo, "task-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-b", "wf-source", "step-source", nil)
	svc.bulkMoveAfterLockForTest = func() { refuseTaskB = true }
	t.Cleanup(func() { svc.bulkMoveAfterLockForTest = nil })

	_, err := svc.BulkMoveSelectedTasks(ctx, []string{"task-a", "task-b"}, "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
	var partial *BulkMovePartialError
	if !errors.As(err, &partial) {
		t.Fatalf("bulk move error = %v, want BulkMovePartialError", err)
	}
	if len(partial.MovedTaskIDs) != 1 || partial.MovedTaskIDs[0] != "task-a" {
		t.Fatalf("moved task IDs = %v, want [task-a]", partial.MovedTaskIDs)
	}
	if partial.Refusal.TaskID != "task-b" || partial.Refusal.Code != bulkMoveApprovalsPendingCode {
		t.Fatalf("refusal = %+v, want task-b approval refusal", partial.Refusal)
	}
	for id, wantStep := range map[string]string{"task-a": "step-target", "task-b": "step-source"} {
		task, readErr := repo.GetTask(ctx, id)
		if readErr != nil {
			t.Fatalf("GetTask(%s): %v", id, readErr)
		}
		if task.WorkflowStepID != wantStep {
			t.Fatalf("%s workflow step = %s, want %s", id, task.WorkflowStepID, wantStep)
		}
	}
}

func TestBulkMovePreflightSkipsCompletionAdmissionWhenTargetIsFull(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedBulkCompletingTarget(t, ctx, svc, repo, func(string) bool { return true })
	target, err := svc.workflowStepGetter.GetStep(ctx, "step-target")
	if err != nil {
		t.Fatal(err)
	}
	target.WIPLimit = 1
	if _, err := repo.DB().ExecContext(ctx, `UPDATE workflow_steps SET wip_limit = 1 WHERE id = 'step-target'`); err != nil {
		t.Fatal(err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source"},
		"step-target": target,
	}})
	createMoveTask(t, ctx, repo, "occupant", "wf-target", "step-target", nil)
	if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET wip_admitted = 1 WHERE id = 'occupant'`); err != nil {
		t.Fatal(err)
	}
	createMoveTask(t, ctx, repo, "queued", "wf-source", "step-source", nil)
	occupants, err := repo.CountAdmittedTasksByWorkflowStep(ctx, "step-target")
	if err != nil || occupants != 1 {
		t.Fatalf("target admitted occupants = %d, err=%v, want 1", occupants, err)
	}

	result, err := svc.BulkMoveSelectedTasks(ctx, []string{"queued"}, "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
	if err != nil {
		t.Fatalf("bulk move into full completing step: %v", err)
	}
	if result.MovedCount != 1 || len(result.MovedTaskIDs) != 1 || result.MovedTaskIDs[0] != "queued" {
		t.Fatalf("bulk result = %+v, want queued task moved", result)
	}
	stored, err := repo.GetTask(ctx, "queued")
	if err != nil {
		t.Fatal(err)
	}
	if stored.State == v1.TaskStateCompleted || stored.WIPAdmitted || stored.QueuedForStepID != "step-target" {
		t.Fatalf("full-step move = state %s, admitted %v, queued for %q", stored.State, stored.WIPAdmitted, stored.QueuedForStepID)
	}
}

func TestBulkMoveErrorCodeDoesNotClassifyByMessageText(t *testing.T) {
	if got := bulkMoveErrorCode(fmt.Errorf("repository detail: archived tasks cannot be moved")); got != bulkMoveGenericCode {
		t.Fatalf("untyped error code = %q, want generic", got)
	}
	for _, tc := range []struct {
		err  error
		code string
	}{
		{ErrTaskMoveArchived, "task_move_archived"},
		{ErrTaskMoveActiveSession, "task_move_active_session"},
		{ErrTaskMoveDifferentWorkspace, "task_move_different_workspace"},
		{ErrTaskMoveWorkflowStep, "task_move_workflow_step"},
	} {
		if got := bulkMoveErrorCode(fmt.Errorf("wrapped: %w", tc.err)); got != tc.code {
			t.Errorf("bulkMoveErrorCode(%v) = %q, want %q", tc.err, got, tc.code)
		}
	}
}

func TestBulkMoveWriteErrorPreservesPartialResultForUnexpectedFailure(t *testing.T) {
	svc, _, _ := createTestService(t)
	task := &models.Task{ID: "refused", Title: "Refused"}
	writeErr := errors.New("database secret: postgres://private")
	err := svc.bulkMoveWriteError(context.Background(), task, []string{"moved"}, true, writeErr)
	var partial *BulkMovePartialError
	if !errors.As(err, &partial) {
		t.Fatalf("write error = %T (%v), want BulkMovePartialError", err, err)
	}
	if len(partial.MovedTaskIDs) != 1 || partial.MovedTaskIDs[0] != "moved" {
		t.Fatalf("moved IDs = %v, want [moved]", partial.MovedTaskIDs)
	}
	if partial.Refusal.Code != "task_move_failed" || strings.Contains(partial.Refusal.Message, "postgres://private") {
		t.Fatalf("refusal = %+v, internal error detail leaked", partial.Refusal)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("partial error %v does not preserve the write error", err)
	}
}

func TestBulkMovePreflightChecksRequiredAssignmentsBeforeQueueWrites(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedBulkCompletingTarget(t, ctx, svc, repo, func(string) bool { return false })
	target, err := svc.workflowStepGetter.GetStep(ctx, "step-target")
	if err != nil {
		t.Fatal(err)
	}
	target.WIPLimit = 1
	if _, err := repo.DB().ExecContext(ctx, `UPDATE workflow_steps SET wip_limit = 1 WHERE id = 'step-target'`); err != nil {
		t.Fatal(err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source"},
		"step-target": target,
	}})
	createMoveTask(t, ctx, repo, "occupant", "wf-target", "step-target", nil)
	if _, err := repo.DB().ExecContext(ctx, `UPDATE tasks SET wip_admitted = 1 WHERE id = 'occupant'`); err != nil {
		t.Fatal(err)
	}
	createMoveTask(t, ctx, repo, "task-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-b", "wf-source", "step-source", nil)
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET metadata = ? WHERE id = 'task-b'`,
		`{"kandev.office_required_subtask_count":1}`,
	); err != nil {
		t.Fatal(err)
	}

	_, err = svc.BulkMoveSelectedTasks(ctx, []string{"task-a", "task-b"}, "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
	var refused *BulkMoveRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("bulk move error = %v, want whole-batch refusal before queued writes", err)
	}
	if len(refused.Refusals) != 1 || refused.Refusals[0].TaskID != "task-b" {
		t.Fatalf("refusals = %+v, want only task-b", refused.Refusals)
	}
	for _, id := range []string{"task-a", "task-b"} {
		stored, readErr := repo.GetTask(ctx, id)
		if readErr != nil {
			t.Fatalf("GetTask(%s): %v", id, readErr)
		}
		if stored.WorkflowStepID != "step-source" {
			t.Fatalf("%s moved or queued before preflight refusal: %+v", id, stored)
		}
	}
}

func TestBulkMovePreflightCostFor200Tasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedBulkCompletingTarget(t, ctx, svc, repo, func(string) bool { return false })
	for i := 0; i < 200; i++ {
		createMoveTask(t, ctx, repo, fmt.Sprintf("bulk-%03d", i), "wf-source", "step-source", nil)
	}

	started := time.Now()
	result, err := svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-target", "step-target", wfmodels.StepTransitionActorHuman)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("BulkMoveTasks(200 tasks): %v", err)
	}
	if result.MovedCount != 200 || len(result.MovedTaskIDs) != 200 {
		t.Fatalf("bulk move result = count %d, IDs %d; want 200 moved tasks", result.MovedCount, len(result.MovedTaskIDs))
	}
	t.Logf("200-task bulk move, including completion preflight and writes: %s", elapsed)
}

func seedBulkCompletingTarget(t *testing.T, ctx context.Context, svc *Service, repo *sqliterepo.Repository, shouldRefuse func(string) bool) {
	t.Helper()
	database := sqlx.NewDb(repo.DB(), "sqlite3")
	wfRepo, err := workflowrepo.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("create workflow repository: %v", err)
	}
	step := &wfmodels.WorkflowStep{ID: "step-target", WorkflowID: "wf-target", Name: "Destination", Position: 9999, CompleteTaskOnEnter: true}
	if err := wfRepo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create completing workflow step: %v", err)
	}
	var completeTaskOnEnter bool
	if err := repo.DB().QueryRowContext(ctx, `SELECT complete_task_on_enter FROM workflow_steps WHERE id = ? AND workflow_id = ?`, step.ID, step.WorkflowID).Scan(&completeTaskOnEnter); err != nil {
		t.Fatalf("read completing workflow step: %v", err)
	}
	if !completeTaskOnEnter {
		t.Fatal("completion target was not persisted as completing")
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-target": step,
	}})
	repo.SetCompletionPolicies(bulkMoveTestCompletionPolicy{shouldRefuse: shouldRefuse})
}
