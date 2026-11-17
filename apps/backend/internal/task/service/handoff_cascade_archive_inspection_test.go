package service

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-DIRTY-WORKTREE-ARCHIVE-005.1
func TestArchiveInspectionFailureAutoArchive(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	workspace, err := repo.GetWorkspace(t.Context(), "ws-1")
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = repo.DB().ExecContext(t.Context(), `INSERT INTO workflow_steps (id,workflow_id,name,position,auto_archive_after_hours,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`, "inspection-auto", workspace.OfficeWorkflowID, "Auto archive", 0, 1, now, now)
	require.NoError(t, err)
	task := &models.Task{ID: "damaged-auto", WorkspaceID: workspace.ID, WorkflowID: workspace.OfficeWorkflowID, WorkflowStepID: "inspection-auto", Title: "Damaged auto archive"}
	require.NoError(t, repo.CreateTask(t.Context(), task))
	_, err = repo.DB().ExecContext(t.Context(), `UPDATE tasks SET updated_at=? WHERE id=?`, now.Add(-48*time.Hour), task.ID)
	require.NoError(t, err)
	svc.SetWorktreeCleanup(&unavailableIdentityCleanup{taskDeletePreflightCleanup: taskDeletePreflightCleanup{worktreesByTaskID: map[string][]*worktree.Worktree{task.ID: {{ID: "broken", TaskID: task.ID}}}}})
	h := NewHandoffService(repo, repo, nil, nil, nil, nil)
	h.SetTaskResourceCleaner(svc)
	svc.SetAutoArchiveCoordinator(h)
	svc.runAutoArchive(t.Context())
	saved, err := repo.GetTask(t.Context(), task.ID)
	require.NoError(t, err)
	require.NotNil(t, saved.ArchivedAt)
	require.NotEmpty(t, saved.ArchivedByCascadeID)
}
