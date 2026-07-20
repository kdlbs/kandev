package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type storageInventory struct {
	reader    *sqlx.DB
	worktrees *worktree.Manager
	lifecycle *lifecycle.Manager
}

func (i *storageInventory) LoadWorkspaceInventory(ctx context.Context) (workspaces.Inventory, error) {
	if i.worktrees == nil || i.lifecycle == nil {
		return workspaces.Inventory{}, workspaces.ErrInventoryIncomplete
	}
	paths, err := i.activeWorktreePaths(ctx)
	if err != nil {
		return workspaces.Inventory{}, err
	}
	inventory := workspaces.Inventory{Complete: true, WorktreePaths: paths}
	for _, execution := range i.lifecycle.ListExecutions() {
		if execution.WorkspacePath != "" && filepath.IsAbs(execution.WorkspacePath) {
			inventory.ExecutionPaths = append(inventory.ExecutionPaths, execution.WorkspacePath)
		}
	}
	rows, err := i.activeWorkspaceRows(ctx)
	if err != nil {
		return workspaces.Inventory{}, err
	}
	for _, row := range rows {
		inventory.EnvironmentPaths = append(inventory.EnvironmentPaths, row.WorkspacePath)
		if row.TaskID != "" && row.WorkspaceID != "" {
			inventory.ScratchRoots = append(inventory.ScratchRoots, workspaces.ScratchRoot{
				TaskID: row.TaskID, WorkspaceID: row.WorkspaceID, Path: row.WorkspacePath,
			})
		}
	}
	return inventory, nil
}

func (i *storageInventory) activeWorktreePaths(ctx context.Context) ([]string, error) {
	paths := make([]string, 0)
	query := "SELECT w.worktree_path FROM task_session_worktrees w " +
		"INNER JOIN task_sessions s ON s.id = w.session_id " +
		"INNER JOIN tasks t ON t.id = s.task_id " +
		"WHERE t.archived_at IS NULL AND w.status = 'active' " +
		"AND w.deleted_at IS NULL AND w.worktree_path <> ''"
	if err := i.reader.SelectContext(ctx, &paths, query); err != nil {
		return nil, err
	}
	return paths, nil
}

func (i *storageInventory) activeWorkspaceRows(ctx context.Context) ([]activeWorkspaceRow, error) {
	rows := make([]activeWorkspaceRow, 0)
	query := "SELECT te.task_id AS taskid, COALESCE(t.workspace_id, '') AS workspaceid, " +
		"te.workspace_path AS workspacepath FROM task_environments te " +
		"LEFT JOIN tasks t ON t.id = te.task_id " +
		"WHERE te.status IN ('creating', 'ready') AND te.workspace_path <> '' AND (" +
		"(t.id IS NOT NULL AND t.archived_at IS NULL) OR EXISTS (" +
		"SELECT 1 FROM task_sessions borrower " +
		"INNER JOIN tasks borrower_task ON borrower_task.id = borrower.task_id " +
		"WHERE borrower.task_environment_id = te.id AND borrower_task.archived_at IS NULL " +
		"AND borrower.state IN ('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')))"
	if err := i.reader.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	return rows, nil
}

type activeWorkspaceRow struct {
	TaskID        string
	WorkspaceID   string
	WorkspacePath string
}

type containerInventory struct{ reader *sqlx.DB }

func (i *containerInventory) ContainerTaskRemovable(ctx context.Context, taskID string) (bool, error) {
	task, err := i.loadContainerTask(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if task.ArchivedAt == nil && !models.IsTerminalTaskState(task.State) {
		return false, nil
	}
	hasEnvironment, err := i.hasLiveTaskEnvironment(ctx, taskID)
	if err != nil || hasEnvironment {
		return false, err
	}
	hasExecutor, err := i.hasLiveExecutor(ctx, taskID)
	if err != nil || hasExecutor {
		return false, err
	}
	return true, nil
}

type containerTask struct {
	State      v1.TaskState `db:"state"`
	ArchivedAt *time.Time   `db:"archived_at"`
}

func (i *containerInventory) loadContainerTask(ctx context.Context, taskID string) (containerTask, error) {
	var task containerTask
	query := i.reader.Rebind("SELECT state, archived_at FROM tasks WHERE id = ?")
	err := i.reader.GetContext(ctx, &task, query, taskID)
	return task, err
}

func (i *containerInventory) hasLiveTaskEnvironment(ctx context.Context, taskID string) (bool, error) {
	var count int
	query := i.reader.Rebind(
		"SELECT COUNT(*) FROM task_environments " +
			"WHERE task_id = ? AND status NOT IN (?, ?)",
	)
	if err := i.reader.GetContext(
		ctx, &count, query, taskID,
		models.TaskEnvironmentStatusStopped, models.TaskEnvironmentStatusFailed,
	); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (i *containerInventory) hasLiveExecutor(ctx context.Context, taskID string) (bool, error) {
	var count int
	query := i.reader.Rebind(
		"SELECT COUNT(*) FROM executors_running " +
			"WHERE task_id = ? AND status NOT IN (?, ?, ?)",
	)
	if err := i.reader.GetContext(
		ctx, &count, query, taskID,
		models.ExecutorRunningStatusFailed,
		models.ExecutorRunningStatusStopped,
		models.ExecutorRunningStatusComplete,
	); err != nil {
		return false, err
	}
	return count > 0, nil
}

var _ workspaces.InventorySource = (*storageInventory)(nil)
var _ storagepkg.CleanupProvider = namedCleanupProvider{}
