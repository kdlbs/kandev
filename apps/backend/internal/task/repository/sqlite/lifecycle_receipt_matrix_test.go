package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestLifecycleReceiptAdmissionRefusalLeavesTaskAndReceiptUntouched(t *testing.T) {
	testLifecycleReceiptAdmissionRefusalLeavesTaskAndReceiptUntouched(t, newRepoForEntityTests(t))
}

func testLifecycleReceiptAdmissionRefusalLeavesTaskAndReceiptUntouched(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	workspaceID, workflowID := "receipt-admission-refusal-workspace", "receipt-admission-refusal-workflow"
	seedWorkspace(t, repo, workspaceID)
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Admission"}))
	seedCASWorkflowStep(t, repo, workflowID, "receipt-admission-source", 0)
	seedCASWorkflowStep(t, repo, workflowID, "receipt-admission-target", 1)
	task := &models.Task{
		ID: "receipt-admission-refusal-task", WorkspaceID: workspaceID, WorkflowID: workflowID,
		WorkflowStepID: "receipt-admission-source", Title: "Admission refusal", WIPAdmitted: true,
	}
	require.NoError(t, repo.CreateTask(ctx, task))

	before := lifecycleTaskAndReceiptRows(t, repo)
	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(
		ctx, task, "not-the-current-step", "receipt-admission-target", 0,
	)
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, before, lifecycleTaskAndReceiptRows(t, repo))
}

func TestLifecycleReceiptPromotionRefusalAndReplayDoNotAdvance(t *testing.T) {
	testLifecycleReceiptPromotionRefusalAndReplayDoNotAdvance(t, newRepoForEntityTests(t))
}

func testLifecycleReceiptPromotionRefusalAndReplayDoNotAdvance(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	workspaceID, workflowID := "receipt-promotion-workspace", "receipt-promotion-workflow"
	seedWorkspace(t, repo, workspaceID)
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Promotion"}))
	seedCASWorkflowStep(t, repo, workflowID, "receipt-promotion-source", 0)
	seedCASWorkflowStep(t, repo, workflowID, "receipt-promotion-target", 1)
	occupant := &models.Task{
		ID: "receipt-promotion-occupant", WorkspaceID: workspaceID, WorkflowID: workflowID,
		WorkflowStepID: "receipt-promotion-target", Title: "Occupant", WIPAdmitted: true,
	}
	moving := &models.Task{
		ID: "receipt-promotion-moving", WorkspaceID: workspaceID, WorkflowID: workflowID,
		WorkflowStepID: "receipt-promotion-source", Title: "Moving", WIPAdmitted: true,
	}
	require.NoError(t, repo.CreateTask(ctx, occupant))
	require.NoError(t, repo.CreateTask(ctx, moving))

	admitted, err := repo.UpdateTaskWithWorkflowStepAdmission(
		ctx, moving, "receipt-promotion-source", "receipt-promotion-target", 1,
	)
	require.NoError(t, err)
	require.False(t, admitted, "the full destination queues the move")
	queued, err := repo.GetTask(ctx, moving.ID)
	require.NoError(t, err)
	queued.WIPAdmitted = true
	queued.QueuedForStepID = ""
	queued.QueuedAt = nil
	beforeRefusal := lifecycleTaskAndReceiptRows(t, repo)
	promoted, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		ctx, queued, "receipt-promotion-target", "receipt-promotion-target", 1,
	)
	require.NoError(t, err)
	require.False(t, promoted)
	require.Equal(t, beforeRefusal, lifecycleTaskAndReceiptRows(t, repo))

	require.NoError(t, repo.DeleteTask(ctx, occupant.ID))
	promoted, err = repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		ctx, queued, "receipt-promotion-target", "receipt-promotion-target", 1,
	)
	require.NoError(t, err)
	require.True(t, promoted)
	beforeReplay := lifecycleTaskAndReceiptRows(t, repo)
	promoted, err = repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		ctx, queued, "receipt-promotion-target", "receipt-promotion-target", 1,
	)
	require.NoError(t, err)
	require.False(t, promoted)
	require.Equal(t, beforeReplay, lifecycleTaskAndReceiptRows(t, repo))
}

func lifecycleTaskAndReceiptRows(t *testing.T, repo *Repository) map[string][]string {
	t.Helper()
	return map[string][]string{
		"tasks":                   lifecycleTableRows(t, repo, "tasks", "id"),
		"task_lifecycle_receipts": lifecycleTableRows(t, repo, "task_lifecycle_receipts", "task_id"),
	}
}

func lifecycleTableRows(t *testing.T, repo *Repository, table, key string) []string {
	t.Helper()
	if repo.db.DriverName() == "sqlite3" {
		key = "rowid"
	}
	rows, err := repo.db.Queryx("SELECT * FROM " + table + " ORDER BY " + key)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var result []string
	for rows.Next() {
		values, scanErr := rows.SliceScan()
		require.NoError(t, scanErr)
		encoded, marshalErr := json.Marshal(values)
		require.NoError(t, marshalErr)
		result = append(result, string(encoded))
	}
	require.NoError(t, rows.Err())
	return result
}
