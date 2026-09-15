package backendapp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/docknet"
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
	tasks, err := i.taskWorkspaceStates(ctx)
	if err != nil {
		return workspaces.Inventory{}, err
	}
	inventory.KnownTaskIDs, inventory.ArchivedTaskIDs = taskEligibilitySets(tasks)
	return inventory, nil
}

func (i *storageInventory) activeWorktreePaths(ctx context.Context) ([]string, error) {
	paths := make([]string, 0)
	// Worktrees are task-owned: the inventory reads task_environment_repos
	// through the owning environment's task row, so a zero-session task
	// workspace is still protected from storage maintenance.
	query := "SELECT ter.worktree_path FROM task_environment_repos ter " +
		"INNER JOIN task_environments te ON te.id = ter.task_environment_id " +
		"INNER JOIN tasks t ON t.id = te.task_id " +
		"WHERE t.archived_at IS NULL AND ter.status = 'active' " +
		"AND ter.deleted_at IS NULL AND ter.worktree_path <> ''"
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

type taskWorkspaceState struct {
	ID         string     `db:"id"`
	ArchivedAt *time.Time `db:"archived_at"`
}

func (i *storageInventory) taskWorkspaceStates(ctx context.Context) ([]taskWorkspaceState, error) {
	rows := make([]taskWorkspaceState, 0)
	query := i.reader.Rebind("SELECT id, archived_at FROM tasks")
	if err := i.reader.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}
	return rows, nil
}

func taskEligibilitySets(tasks []taskWorkspaceState) (map[string]struct{}, map[string]struct{}) {
	known := make(map[string]struct{}, len(tasks))
	archived := make(map[string]struct{})
	for _, task := range tasks {
		known[task.ID] = struct{}{}
		if task.ArchivedAt != nil {
			archived[task.ID] = struct{}{}
		}
	}
	return known, archived
}

type containerInventory struct{ reader *sqlx.DB }

// networkTaskOracle resolves network ownership keys to owning-task liveness
// for the docknet provider. Keys are kandev task IDs (from kandev.* labels)
// or compose project names (kd_<task-hash> on the governed deployment).
type networkTaskOracle struct{ reader *sqlx.DB }

// Ownership resolves the network's ownership key, then looks the key up as a
// task ID first and as a kd_-prefixed compose project name second: broker-
// launched projects hom via labels only.
func (o *networkTaskOracle) Ownership(network agentdocker.NetworkInfo) (string, docknet.TaskLookup, error) {
	key := docknet.OwnershipKeyFromLabels(network.Labels)
	if key == "" {
		return "", docknet.TaskLookupUnknown, nil
	}
	taskID := key
	if !isUUID(key) {
		resolved, ok, err := o.taskIDForProjectName(key)
		if err != nil {
			return key, docknet.TaskLookupUnknown, err
		}
		if !ok {
			return key, docknet.TaskLookupUnknown, nil
		}
		taskID = resolved
	}
	removable, err := o.taskRemovable(taskID)
	if err != nil {
		return key, docknet.TaskLookupUnknown, err
	}
	if removable {
		return key, docknet.TaskLookupInactive, nil
	}
	return key, docknet.TaskLookupActive, nil
}

// taskRemovable mirrors containerInventory's liveness composite: done means
// the task is archived or terminal AND has no live environment/executor.
// A missing task row is removable-by-unknown handled by the caller.
func (o *networkTaskOracle) taskRemovable(taskID string) (bool, error) {
	inventory := &containerInventory{reader: o.reader}
	removable, err := inventory.ContainerTaskRemovable(context.Background(), taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	return removable, nil
}

// taskIDForProjectName resolves a kd_<task-hash> compose project name to a
// task row. The guarded Compose broker hashes the absolute task root with
// SHA-256 and uses the first 16 hexadecimal characters after the kd_ prefix.
func (o *networkTaskOracle) taskIDForProjectName(name string) (string, bool, error) {
	if !strings.HasPrefix(name, "kd_") {
		return "", false, nil
	}
	fragment := strings.TrimPrefix(name, "kd_")
	if fragment == "" {
		return "", false, nil
	}
	var taskID string
	// Exact task-ID match first (the deployment's canonical form).
	query := o.reader.Rebind("SELECT id FROM tasks WHERE id = ?")
	if err := o.reader.GetContext(context.Background(), &taskID, query, fragment); err == nil {
		return taskID, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	var roots []networkTaskRoot
	query = o.reader.Rebind(`
		SELECT t.id AS task_id,
		       COALESCE(te.workspace_path, '') AS workspace_path,
		       COALESCE(te.task_dir_name, '') AS task_dir_name
		FROM tasks t
		LEFT JOIN task_environments te ON te.task_id = t.id`)
	if err := o.reader.SelectContext(context.Background(), &roots, query); err != nil {
		return "", false, err
	}
	var matchedTaskID string
	for _, row := range roots {
		for _, root := range row.candidateRoots() {
			if guardedComposeProjectName(root) != name {
				continue
			}
			if matchedTaskID != "" && matchedTaskID != row.TaskID {
				return "", false, errors.New("compose project hash matches multiple tasks")
			}
			matchedTaskID = row.TaskID
		}
	}
	return matchedTaskID, matchedTaskID != "", nil
}

type networkTaskRoot struct {
	TaskID        string `db:"task_id"`
	WorkspacePath string `db:"workspace_path"`
	TaskDirName   string `db:"task_dir_name"`
}

func (r networkTaskRoot) candidateRoots() []string {
	if r.WorkspacePath == "" || !filepath.IsAbs(r.WorkspacePath) {
		return nil
	}
	workspacePath := filepath.Clean(r.WorkspacePath)
	roots := []string{workspacePath}
	for current := workspacePath; r.TaskDirName != ""; current = filepath.Dir(current) {
		if filepath.Base(current) == r.TaskDirName {
			if current != workspacePath {
				roots = append(roots, current)
			}
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return roots
}

func guardedComposeProjectName(taskRoot string) string {
	digest := sha256.Sum256([]byte(taskRoot))
	return "kd_" + hex.EncodeToString(digest[:8])
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

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
