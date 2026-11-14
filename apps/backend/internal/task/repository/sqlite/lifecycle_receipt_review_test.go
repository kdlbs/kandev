package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	"github.com/stretchr/testify/require"
)

func TestTaskFieldStepMoveUpdatesLifecycleReceipt(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "field-move-ws")
	task := &models.Task{ID: "field-move-task", WorkspaceID: "field-move-ws", WorkflowID: "field-wf", WorkflowStepID: "before"}
	require.NoError(t, repo.CreateTask(ctx, task))
	step := "after"
	_, err := repo.UpdateTaskFieldsWithParentAdmission(ctx, task.ID, models.TaskFieldUpdate{WorkflowStepID: &step}, hierarchy.ValidateParent)
	require.NoError(t, err)
	receipt, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.Equal(t, step, receipt.WorkflowStepID)
	require.Equal(t, "move", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)
}

func TestProfileCleanupRecordsDeletedLifecycleReceipt(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "profile-task", "profile-session", "profile-turn")
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET is_ephemeral = 1 WHERE id = ?`), "profile-task")
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET agent_profile_id = ? WHERE id = ?`), "profile-cleanup", "profile-session")
	require.NoError(t, err)
	task, err := repo.GetTask(ctx, "profile-task")
	require.NoError(t, err)
	require.NoError(t, repo.recordTaskLifecycleReceipt(ctx, repo.db, task, "active", "create", time.Now().UTC()))
	count, err := repo.DeleteEphemeralTasksByAgentProfile(ctx, "profile-cleanup")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	receipt, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, "deleted", receipt.Disposition)
	require.Equal(t, "delete", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)
}

func TestWorkspaceCascadeRemovesLiveAndDeletedTaskReceipts(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "receipt-cascade-ws")
	for _, id := range []string{"live", "deleted"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "receipt-cascade-ws", Title: id}))
	}
	require.NoError(t, repo.DeleteTask(ctx, "deleted"))
	_, _, err := repo.DeleteWorkspaceCascade(ctx, "receipt-cascade-ws")
	require.NoError(t, err)
	for _, id := range []string{"live", "deleted"} {
		receipt, err := repo.GetTaskLifecycleReceipt(ctx, "receipt-cascade-ws", id)
		require.NoError(t, err)
		require.Nil(t, receipt, id)
	}
}

func TestRepeatedWorkflowPlacementDoesNotAdvanceLifecycleReceipt(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "repeat-ws")
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "repeat-wf", WorkspaceID: "repeat-ws", Name: "Repeat"}))
	_, err := repo.db.ExecContext(ctx, `INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, ?, 0)`, "repeat-step", "repeat-wf", "Work")
	require.NoError(t, err)
	task := &models.Task{ID: "repeat-task", WorkspaceID: "repeat-ws", Title: "Repeat"}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.AddTaskToWorkflow(ctx, task.ID, "repeat-wf", "repeat-step", 0))
	before, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.NoError(t, repo.AddTaskToWorkflow(ctx, task.ID, "repeat-wf", "repeat-step", 1))
	after, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.Equal(t, before.Revision, after.Revision)
	require.Equal(t, before.ChangedAt, after.ChangedAt)
}

func TestAutoArchiveWritesLifecycleReceipt(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "auto-receipt-ws")
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "auto-receipt-wf", WorkspaceID: "auto-receipt-ws", Name: "Auto"}))
	_, err := repo.db.ExecContext(ctx, `INSERT INTO workflow_steps (id, workflow_id, name, position, auto_archive_after_hours) VALUES (?, ?, ?, 0, 1)`, "auto-receipt-step", "auto-receipt-wf", "Done")
	require.NoError(t, err)
	task := &models.Task{ID: "auto-receipt-task", WorkspaceID: "auto-receipt-ws", WorkflowID: "auto-receipt-wf", WorkflowStepID: "auto-receipt-step"}
	require.NoError(t, repo.CreateTask(ctx, task))
	old := time.Now().UTC().Add(-48 * time.Hour)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), old, task.ID)
	require.NoError(t, err)
	changed, err := repo.ArchiveTaskIfAutoArchiveEligible(ctx, task.ID, old, "")
	require.NoError(t, err)
	require.True(t, changed)
	receipt, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.Equal(t, "archived", receipt.Disposition)
	require.Equal(t, "archive", receipt.Provenance)
	require.EqualValues(t, 2, receipt.Revision)
}

func TestArchiveTaskAcceptsLegacyNullState(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "null-state-ws")
	task := &models.Task{ID: "null-state-task", WorkspaceID: "null-state-ws"}
	require.NoError(t, repo.CreateTask(ctx, task))
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET state = NULL WHERE id = ?`), task.ID)
	require.NoError(t, err)
	require.NoError(t, repo.ArchiveTask(ctx, task.ID))
	receipt, err := repo.GetTaskLifecycleReceipt(ctx, task.WorkspaceID, task.ID)
	require.NoError(t, err)
	require.Equal(t, "archived", receipt.Disposition)
}
