package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestExactTaskUpdateRecovery(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)

	current, err := repo.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	title := "Updated once"
	request := ExactTaskUpdateRequest{
		WorkspaceID:             current.WorkspaceID,
		ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID:             "operation-exact-recovery",
		PayloadDigest:           "sha256:payload-v1",
		Title:                   &title,
	}
	first, err := svc.UpdateTaskExact(ctx, current.ID, request)
	if err != nil {
		t.Fatalf("first UpdateTaskExact: %v", err)
	}
	if first.AlreadyApplied || first.Task.Title != title {
		t.Fatalf("first result = %#v, want applied title %q", first, title)
	}

	// A fresh service over the same repository models a backend restart after
	// the task transaction committed but before the plugin receipt was saved.
	restarted := newServiceOverRepository(t, repo)
	replayed, err := restarted.UpdateTaskExact(ctx, current.ID, request)
	if err != nil {
		t.Fatalf("replayed UpdateTaskExact: %v", err)
	}
	if !replayed.AlreadyApplied || replayed.Task.Title != title {
		t.Fatalf("replay result = %#v, want already-applied title %q", replayed, title)
	}

	description := "must not apply"
	if _, err := restarted.UpdateTaskExact(ctx, current.ID, ExactTaskUpdateRequest{
		WorkspaceID:             current.WorkspaceID,
		ExpectedResourceVersion: request.ExpectedResourceVersion,
		OperationID:             request.OperationID,
		PayloadDigest:           "sha256:different-payload",
		Description:             &description,
	}); err == nil || !errors.Is(err, repoerrors.ErrTaskOperationConflict) {
		t.Fatalf("operation payload mismatch error = %v, want ErrTaskOperationConflict", err)
	}

	final, err := repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("final GetTask: %v", err)
	}
	if final.Title != title || final.Description == "must not apply" {
		t.Fatalf("final task = %#v, unexpected replay mutation", final)
	}
	if !final.UpdatedAt.Equal(first.Task.UpdatedAt) {
		t.Fatalf("replay changed resource version from %s to %s", first.Task.UpdatedAt, final.UpdatedAt)
	}
	if final.UpdatedAt.Equal(current.UpdatedAt) {
		t.Fatal("task update did not advance its resource version")
	}
	var operationCount int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM exact_task_command_operations WHERE operation_id = ?`, request.OperationID).Scan(&operationCount); err != nil {
		t.Fatalf("count exact task operation: %v", err)
	}
	if operationCount != 1 {
		t.Fatalf("persisted operation count = %d, want one", operationCount)
	}
}

func TestExactTaskUpdateRejectsStaleResourceVersion(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	current, err := repo.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	firstTitle := "Concurrent update"
	if _, err := svc.UpdateTaskExact(ctx, current.ID, ExactTaskUpdateRequest{
		WorkspaceID: current.WorkspaceID, ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID: "operation-first", PayloadDigest: "sha256:first", Title: &firstTitle,
	}); err != nil {
		t.Fatalf("first UpdateTaskExact: %v", err)
	}
	secondTitle := "Stale overwrite"
	if _, err := svc.UpdateTaskExact(ctx, current.ID, ExactTaskUpdateRequest{
		WorkspaceID: current.WorkspaceID, ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID: "operation-stale", PayloadDigest: "sha256:stale", Title: &secondTitle,
	}); err == nil || !errors.Is(err, repoerrors.ErrTaskVersionConflict) {
		t.Fatalf("stale UpdateTaskExact error = %v, want ErrTaskVersionConflict", err)
	}
	final, err := repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("final GetTask: %v", err)
	}
	if final.Title != firstTitle {
		t.Fatalf("title = %q, want %q", final.Title, firstTitle)
	}
}

func TestExactTaskUpdateRecoversLabels(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	current, err := repo.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	labels := []string{"urgent", "coordination"}
	request := ExactTaskUpdateRequest{
		WorkspaceID: current.WorkspaceID, ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID: "operation-exact-labels", PayloadDigest: "sha256:labels-v1", Labels: &labels,
	}
	first, err := svc.UpdateTaskExact(ctx, current.ID, request)
	if err != nil {
		t.Fatalf("UpdateTaskExact labels: %v", err)
	}
	if first.Task.Labels != `["urgent","coordination"]` {
		t.Fatalf("labels = %q, want %q", first.Task.Labels, `["urgent","coordination"]`)
	}
	restarted := newServiceOverRepository(t, repo)
	replayed, err := restarted.UpdateTaskExact(ctx, current.ID, request)
	if err != nil {
		t.Fatalf("replay UpdateTaskExact labels: %v", err)
	}
	if !replayed.AlreadyApplied || replayed.Task.Labels != `["urgent","coordination"]` {
		t.Fatalf("replayed result = %+v, want already-applied labels", replayed)
	}
	empty := []string{}
	cleared, err := restarted.UpdateTaskExact(ctx, current.ID, ExactTaskUpdateRequest{
		WorkspaceID: current.WorkspaceID, ExpectedResourceVersion: first.Task.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID: "operation-exact-labels-clear", PayloadDigest: "sha256:labels-clear", Labels: &empty,
	})
	if err != nil {
		t.Fatalf("clear exact task labels: %v", err)
	}
	if cleared.Task.Labels != "[]" {
		t.Fatalf("cleared labels = %q, want []", cleared.Task.Labels)
	}
}

func TestExactTaskUpdateClearsHumanAssignee(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	current, err := repo.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	current.AssigneeUserID = "user-old"
	if err := repo.UpdateTask(ctx, current); err != nil {
		t.Fatalf("seed assignee: %v", err)
	}
	current, err = repo.GetTask(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	unassigned := ""
	result, err := svc.UpdateTaskExact(ctx, current.ID, ExactTaskUpdateRequest{
		WorkspaceID: current.WorkspaceID, ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID: "operation-exact-unassign", PayloadDigest: "sha256:unassign", AssigneeUserID: &unassigned,
	})
	if err != nil {
		t.Fatalf("UpdateTaskExact clear assignee: %v", err)
	}
	if result.Task.AssigneeUserID != "" {
		t.Fatalf("assignee after exact clear = %q, want empty", result.Task.AssigneeUserID)
	}
}

func TestExactTaskMoveRecoversWorkflowAdmissionOperation(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-exact-move", "wf-source", "step-source", nil)
	current, err := repo.GetTask(ctx, "task-exact-move")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	operation := &ExactTaskMoveOperation{
		WorkspaceID:             current.WorkspaceID,
		ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID:             "operation-exact-move", PayloadDigest: "sha256:move-v1",
	}
	first, err := svc.MoveTaskWithOptions(ctx, current.ID, "wf-source", "step-review-target", 0,
		MoveTaskOptions{ExactOperation: operation})
	if err != nil {
		t.Fatalf("first exact MoveTaskWithOptions: %v", err)
	}
	if first.AlreadyApplied || first.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("first exact move result = %+v, want newly applied target move", first)
	}

	second, err := svc.MoveTaskWithOptions(ctx, current.ID, "wf-source", "step-review-target", 0,
		MoveTaskOptions{ExactOperation: operation})
	if err != nil {
		t.Fatalf("replayed exact MoveTaskWithOptions: %v", err)
	}
	if !second.AlreadyApplied || second.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("replayed exact move result = %+v, want already-applied target move", second)
	}
	if !second.Task.UpdatedAt.Equal(first.Task.UpdatedAt) {
		t.Fatalf("replay changed task resource version from %s to %s", first.Task.UpdatedAt, second.Task.UpdatedAt)
	}
	var operationCount int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM exact_task_command_operations WHERE operation_id = ?`, operation.OperationID).Scan(&operationCount); err != nil {
		t.Fatalf("count exact move operation: %v", err)
	}
	if operationCount != 1 {
		t.Fatalf("exact move operation count = %d, want one", operationCount)
	}
}

func TestExactTaskMoveSameStepRecoversOperation(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-exact-same-step", "wf-source", "step-source", nil)
	current, err := repo.GetTask(ctx, "task-exact-same-step")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	operation := &ExactTaskMoveOperation{
		WorkspaceID:             current.WorkspaceID,
		ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID:             "operation-exact-same-step", PayloadDigest: "sha256:same-step-v1",
	}

	first, err := svc.MoveTaskWithOptions(ctx, current.ID, "wf-source", "step-source", 0,
		MoveTaskOptions{ExactOperation: operation})
	if err != nil {
		t.Fatalf("first same-step exact move: %v", err)
	}
	if first.AlreadyApplied || first.Task.WorkflowStepID != "step-source" {
		t.Fatalf("first same-step exact result = %+v, want a newly applied no-transition move", first)
	}

	second, err := svc.MoveTaskWithOptions(ctx, current.ID, "wf-source", "step-source", 0,
		MoveTaskOptions{ExactOperation: operation})
	if err != nil {
		t.Fatalf("replayed same-step exact move: %v", err)
	}
	if !second.AlreadyApplied || second.Task.WorkflowStepID != "step-source" {
		t.Fatalf("replayed same-step exact result = %+v, want an already-applied no-transition move", second)
	}
	if !second.Task.UpdatedAt.Equal(first.Task.UpdatedAt) {
		t.Fatalf("replay changed task resource version from %s to %s", first.Task.UpdatedAt, second.Task.UpdatedAt)
	}
	var operationCount int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM exact_task_command_operations WHERE operation_id = ?`, operation.OperationID).Scan(&operationCount); err != nil {
		t.Fatalf("count exact same-step operation: %v", err)
	}
	if operationCount != 1 {
		t.Fatalf("exact same-step operation count = %d, want one", operationCount)
	}
}

func TestExactTaskArchiveRecoversAfterCommit(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	setupTestTask(t, repo)
	current, err := repo.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	request := ExactTaskArchiveRequest{
		WorkspaceID:             current.WorkspaceID,
		ExpectedResourceVersion: current.UpdatedAt.UTC().Format(time.RFC3339Nano),
		OperationID:             "operation-exact-archive", PayloadDigest: "sha256:archive-v1",
	}
	first, err := svc.ArchiveTaskExact(ctx, current.ID, request)
	if err != nil {
		t.Fatalf("first ArchiveTaskExact: %v", err)
	}
	if first.AlreadyApplied || first.Task.ArchivedAt == nil {
		t.Fatalf("first exact archive = %+v, want a newly archived task", first)
	}

	restarted := newServiceOverRepository(t, repo)
	replayed, err := restarted.ArchiveTaskExact(ctx, current.ID, request)
	if err != nil {
		t.Fatalf("replayed ArchiveTaskExact: %v", err)
	}
	if !replayed.AlreadyApplied || replayed.Task.ArchivedAt == nil {
		t.Fatalf("replayed exact archive = %+v, want already-applied archived task", replayed)
	}
	if !replayed.Task.UpdatedAt.Equal(first.Task.UpdatedAt) {
		t.Fatalf("archive replay changed resource version from %s to %s", first.Task.UpdatedAt, replayed.Task.UpdatedAt)
	}
	var operationCount int
	if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM exact_task_command_operations WHERE operation_id = ?`, request.OperationID).Scan(&operationCount); err != nil {
		t.Fatalf("count exact archive operation: %v", err)
	}
	if operationCount != 1 {
		t.Fatalf("exact archive operation count = %d, want one", operationCount)
	}
}
