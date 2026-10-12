package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestLifecycleReceiptExactMoveRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptExactMoveRollsBackOnReceiptFailure(t, newRepoForEntityTests(t), "sqlite-exact")
}

func testLifecycleReceiptExactMoveRollsBackOnReceiptFailure(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	move, version := prepareExactReceiptMove(t, repo, prefix)
	before := lifecycleMutationRows(t, repo)
	installLifecycleReceiptFailure(t, repo)
	replayed, err := repo.UpdateTaskExactOperation(
		context.Background(), move, move.WorkspaceID, version, prefix+"-operation", prefix+"-digest",
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "receipt write failure")
	require.False(t, replayed)
	require.Equal(t, before, lifecycleMutationRows(t, repo))
}

func TestLifecycleReceiptExactMoveSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptExactMoveSuccessAndReplayAdvanceOnce(t, newRepoForEntityTests(t), "sqlite-exact-success")
}

func testLifecycleReceiptExactMoveSuccessAndReplayAdvanceOnce(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	move, version := prepareExactReceiptMove(t, repo, prefix)
	replayed, err := repo.UpdateTaskExactOperation(
		context.Background(), move, move.WorkspaceID, version, prefix+"-operation", prefix+"-digest",
	)
	require.NoError(t, err)
	require.False(t, replayed)
	receipt, err := repo.GetTaskLifecycleReceipt(context.Background(), move.WorkspaceID, move.ID)
	require.NoError(t, err)
	require.Equal(t, "move", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)
	beforeReplay := lifecycleMutationRows(t, repo)
	replayed, err = repo.UpdateTaskExactOperation(
		context.Background(), move, move.WorkspaceID, version, prefix+"-operation", prefix+"-digest",
	)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, beforeReplay, lifecycleMutationRows(t, repo))
}

func TestLifecycleReceiptPromotionRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptPromotionRollsBackOnReceiptFailure(t, newRepoForEntityTests(t), "sqlite-promotion")
}

func testLifecycleReceiptPromotionRollsBackOnReceiptFailure(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	queued, occupantID := prepareQueuedReceiptPromotion(t, repo, prefix)
	require.NoError(t, repo.DeleteTask(context.Background(), occupantID))
	before := lifecycleMutationRows(t, repo)
	installLifecycleReceiptFailure(t, repo)
	promoted, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		context.Background(), queued, prefix+"-target", prefix+"-target", 1,
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "receipt write failure")
	require.False(t, promoted)
	require.Equal(t, before, lifecycleMutationRows(t, repo))
}

func TestLifecycleReceiptPromotionSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptPromotionSuccessAndReplayAdvanceOnce(t, newRepoForEntityTests(t), "sqlite-promotion-success")
}

func TestLifecycleReceiptCascadeArchiveRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptCascadeArchiveRollsBackOnReceiptFailure(t, newRepoForEntityTests(t), "sqlite-cascade-archive")
}

func testLifecycleReceiptCascadeArchiveRollsBackOnReceiptFailure(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	task := prepareCascadeReceiptTask(t, repo, prefix)
	before := lifecycleMutationRows(t, repo)
	installLifecycleReceiptFailure(t, repo)
	changed, err := repo.ArchiveTaskIfActive(context.Background(), task.ID, prefix+"-cascade")
	require.Error(t, err)
	require.ErrorContains(t, err, "receipt write failure")
	require.False(t, changed)
	require.Equal(t, before, lifecycleMutationRows(t, repo))
}

func TestLifecycleReceiptCascadeArchiveSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptCascadeArchiveSuccessAndReplayAdvanceOnce(t, newRepoForEntityTests(t), "sqlite-cascade-archive-success")
}

func testLifecycleReceiptCascadeArchiveSuccessAndReplayAdvanceOnce(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	task := prepareCascadeReceiptTask(t, repo, prefix)
	changed, err := repo.ArchiveTaskIfActive(context.Background(), task.ID, prefix+"-cascade")
	require.NoError(t, err)
	require.True(t, changed)
	requireCascadeReceipt(t, repo, task, "archived", "archive", 2)
	beforeReplay := lifecycleMutationRows(t, repo)
	changed, err = repo.ArchiveTaskIfActive(context.Background(), task.ID, prefix+"-cascade")
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, beforeReplay, lifecycleMutationRows(t, repo))
}

func TestLifecycleReceiptCascadeUnarchiveRollsBackOnReceiptFailure(t *testing.T) {
	testLifecycleReceiptCascadeUnarchiveRollsBackOnReceiptFailure(t, newRepoForEntityTests(t), "sqlite-cascade-unarchive")
}

func testLifecycleReceiptCascadeUnarchiveRollsBackOnReceiptFailure(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	task := prepareCascadeReceiptTask(t, repo, prefix)
	require.NoError(t, archiveCascadeReceiptTask(repo, task, prefix))
	before := lifecycleMutationRows(t, repo)
	installLifecycleReceiptFailure(t, repo)
	changed, err := repo.UnarchiveTaskByCascade(context.Background(), task.ID, prefix+"-cascade")
	require.Error(t, err)
	require.ErrorContains(t, err, "receipt write failure")
	require.False(t, changed)
	require.Equal(t, before, lifecycleMutationRows(t, repo))
}

func TestLifecycleReceiptCascadeUnarchiveSuccessAndReplayAdvanceOnce(t *testing.T) {
	testLifecycleReceiptCascadeUnarchiveSuccessAndReplayAdvanceOnce(t, newRepoForEntityTests(t), "sqlite-cascade-unarchive-success")
}

func testLifecycleReceiptCascadeUnarchiveSuccessAndReplayAdvanceOnce(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	task := prepareCascadeReceiptTask(t, repo, prefix)
	require.NoError(t, archiveCascadeReceiptTask(repo, task, prefix))
	changed, err := repo.UnarchiveTaskByCascade(context.Background(), task.ID, prefix+"-cascade")
	require.NoError(t, err)
	require.True(t, changed)
	requireCascadeReceipt(t, repo, task, "active", "unarchive", 3)
	beforeReplay := lifecycleMutationRows(t, repo)
	changed, err = repo.UnarchiveTaskByCascade(context.Background(), task.ID, prefix+"-cascade")
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, beforeReplay, lifecycleMutationRows(t, repo))
}

func testLifecycleReceiptPromotionSuccessAndReplayAdvanceOnce(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	queued, occupantID := prepareQueuedReceiptPromotion(t, repo, prefix)
	require.NoError(t, repo.DeleteTask(context.Background(), occupantID))
	promoted, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		context.Background(), queued, prefix+"-target", prefix+"-target", 1,
	)
	require.NoError(t, err)
	require.True(t, promoted)
	receipt, err := repo.GetTaskLifecycleReceipt(context.Background(), queued.WorkspaceID, queued.ID)
	require.NoError(t, err)
	require.Equal(t, "move", receipt.Provenance)
	require.EqualValues(t, 3, receipt.Revision)
	beforeReplay := lifecycleMutationRows(t, repo)
	promoted, err = repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		context.Background(), queued, prefix+"-target", prefix+"-target", 1,
	)
	require.NoError(t, err)
	require.False(t, promoted)
	require.Equal(t, beforeReplay, lifecycleMutationRows(t, repo))
}

func prepareExactReceiptMove(t *testing.T, repo *Repository, prefix string) (*models.Task, string) {
	t.Helper()
	ctx := context.Background()
	workspaceID, workflowID := prefix+"-workspace", prefix+"-workflow"
	seedWorkspace(t, repo, workspaceID)
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Exact"}))
	seedCASWorkflowStep(t, repo, workflowID, prefix+"-source", 0)
	seedCASWorkflowStep(t, repo, workflowID, prefix+"-target", 1)
	task := &models.Task{ID: prefix + "-task", WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: prefix + "-source", Title: "Exact move", WIPAdmitted: true}
	require.NoError(t, repo.CreateTask(ctx, task))
	move, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	version := move.UpdatedAt.UTC().Format(time.RFC3339Nano)
	move.WorkflowStepID = prefix + "-target"
	return move, version
}

func prepareQueuedReceiptPromotion(t *testing.T, repo *Repository, prefix string) (*models.Task, string) {
	t.Helper()
	ctx := context.Background()
	workspaceID, workflowID := prefix+"-workspace", prefix+"-workflow"
	seedWorkspace(t, repo, workspaceID)
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Promotion"}))
	seedCASWorkflowStep(t, repo, workflowID, prefix+"-source", 0)
	seedCASWorkflowStep(t, repo, workflowID, prefix+"-target", 1)
	occupant := &models.Task{ID: prefix + "-occupant", WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: prefix + "-target", Title: "Occupant", WIPAdmitted: true}
	moving := &models.Task{ID: prefix + "-moving", WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: prefix + "-source", Title: "Moving", WIPAdmitted: true}
	require.NoError(t, repo.CreateTask(ctx, occupant))
	require.NoError(t, repo.CreateTask(ctx, moving))
	admitted, err := repo.UpdateTaskWithWorkflowStepAdmission(ctx, moving, prefix+"-source", prefix+"-target", 1)
	require.NoError(t, err)
	require.False(t, admitted)
	queued, err := repo.GetTask(ctx, moving.ID)
	require.NoError(t, err)
	queued.WIPAdmitted = true
	queued.QueuedForStepID = ""
	queued.QueuedAt = nil
	return queued, occupant.ID
}

func prepareCascadeReceiptTask(t *testing.T, repo *Repository, prefix string) *models.Task {
	t.Helper()
	workspaceID := prefix + "-workspace"
	seedWorkspace(t, repo, workspaceID)
	task := &models.Task{ID: prefix + "-task", WorkspaceID: workspaceID, Title: "Cascade receipt"}
	require.NoError(t, repo.CreateTask(context.Background(), task))
	return task
}

func archiveCascadeReceiptTask(repo *Repository, task *models.Task, prefix string) error {
	_, err := repo.ArchiveTaskIfActive(context.Background(), task.ID, prefix+"-cascade")
	return err
}

func requireCascadeReceipt(t *testing.T, repo *Repository, task *models.Task, disposition, provenance string, revision int64) {
	t.Helper()
	receipt, err := repo.GetTaskLifecycleReceipt(context.Background(), task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.Equal(t, disposition, receipt.Disposition)
	require.Equal(t, provenance, receipt.Provenance)
	require.EqualValues(t, revision, receipt.Revision)
}

func installLifecycleReceiptFailure(t *testing.T, repo *Repository) {
	t.Helper()
	if repo.db.DriverName() == "sqlite3" {
		_, err := repo.db.Exec(`CREATE TRIGGER fail_lifecycle_receipt BEFORE INSERT ON task_lifecycle_receipts BEGIN SELECT RAISE(ABORT, 'receipt write failure'); END`)
		require.NoError(t, err)
		return
	}
	_, err := repo.db.Exec(`CREATE FUNCTION fail_lifecycle_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'receipt write failure'; END; $$`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`CREATE TRIGGER fail_lifecycle_receipt BEFORE INSERT ON task_lifecycle_receipts FOR EACH ROW EXECUTE FUNCTION fail_lifecycle_receipt()`)
	require.NoError(t, err)
}

func lifecycleMutationRows(t *testing.T, repo *Repository) map[string][]string {
	t.Helper()
	return map[string][]string{
		"tasks":                         lifecycleTableRows(t, repo, "tasks", "id"),
		"task_lifecycle_receipts":       lifecycleTableRows(t, repo, "task_lifecycle_receipts", "task_id"),
		"task_step_transitions":         lifecycleTableRows(t, repo, "task_step_transitions", "id"),
		"exact_task_command_operations": lifecycleTableRows(t, repo, "exact_task_command_operations", "operation_id"),
	}
}
