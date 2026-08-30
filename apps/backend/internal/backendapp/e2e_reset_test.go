package backendapp

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestE2EResetDeletesWorkspaceGitHubAuthentication(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "e2e-reset.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		CREATE TABLE github_workspace_connections (workspace_id TEXT PRIMARY KEY);
		CREATE TABLE github_user_connections (workspace_id TEXT, user_id TEXT);
		CREATE TABLE github_auth_flows (state_hash TEXT PRIMARY KEY, workspace_id TEXT);
		CREATE TABLE secrets (id TEXT PRIMARY KEY);
		INSERT INTO github_workspace_connections VALUES ('ws-1'), ('ws-2');
		INSERT INTO github_user_connections VALUES ('ws-1', 'user-1'), ('ws-2', 'user-2');
		INSERT INTO github_auth_flows VALUES ('state-1', 'ws-1'), ('state-2', 'ws-2');
		INSERT INTO secrets VALUES
			('github:workspace:ws-1:pat'),
			('github:user:ws-1:user-1:access'),
			('github:user:ws-1:user-1:refresh'),
			('github:workspace:ws-2:pat')
	`); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	if err := deleteGitHubAuthForReset(context.Background(), database.DB, "ws-1"); err != nil {
		t.Fatalf("delete GitHub auth: %v", err)
	}
	for _, table := range []string{
		"github_workspace_connections", "github_user_connections", "github_auth_flows",
	} {
		assertWorkspaceRows(t, database, table, "ws-1", 0)
		assertWorkspaceRows(t, database, table, "ws-2", 1)
	}
	var ws1Secrets, ws2Secrets int
	if err := database.Get(&ws1Secrets,
		`SELECT COUNT(*) FROM secrets WHERE id = ? OR substr(id, 1, length(?)) = ?`,
		"github:workspace:ws-1:pat", "github:user:ws-1:", "github:user:ws-1:"); err != nil {
		t.Fatalf("count deleted secrets: %v", err)
	}
	if err := database.Get(&ws2Secrets,
		`SELECT COUNT(*) FROM secrets WHERE id = ? OR substr(id, 1, length(?)) = ?`,
		"github:workspace:ws-2:pat", "github:user:ws-2:", "github:user:ws-2:"); err != nil {
		t.Fatalf("count preserved secrets: %v", err)
	}
	if ws1Secrets != 0 || ws2Secrets != 1 {
		t.Fatalf("secret counts = ws-1:%d ws-2:%d, want 0 and 1", ws1Secrets, ws2Secrets)
	}
}

func TestWaitForE2ETaskCleanupWithReader(t *testing.T) {
	t.Run("waits for a running job to finish", func(t *testing.T) {
		firstRead := make(chan struct{})
		releaseFirstRead := make(chan struct{})
		result := make(chan error, 1)
		done := make(chan struct{})
		var releaseOnce sync.Once
		t.Cleanup(func() {
			releaseOnce.Do(func() { close(releaseFirstRead) })
			<-done
		})
		readCount := 0
		go func() {
			defer close(done)
			result <- waitForE2ETaskCleanupWithReader(
				context.Background(),
				[]string{"task-1"},
				0,
				func(context.Context, []string) ([]e2eTaskCleanupStatus, error) {
					readCount++
					if readCount == 1 {
						close(firstRead)
						<-releaseFirstRead
						return []e2eTaskCleanupStatus{{
							taskID: "task-1",
							state:  taskmodels.TaskResourceCleanupStateRunning,
						}}, nil
					}
					return nil, nil
				},
			)
		}()

		<-firstRead
		releaseOnce.Do(func() { close(releaseFirstRead) })
		if err := <-result; err != nil {
			t.Fatalf("waitForE2ETaskCleanupWithReader: %v", err)
		}
		if readCount != 2 {
			t.Fatalf("status reads = %d, want 2", readCount)
		}
	})

	t.Run("returns failed job error", func(t *testing.T) {
		err := waitForE2ETaskCleanupWithReader(
			context.Background(),
			[]string{"task-1"},
			0,
			func(context.Context, []string) ([]e2eTaskCleanupStatus, error) {
				return []e2eTaskCleanupStatus{{
					taskID:    "task-1",
					state:     taskmodels.TaskResourceCleanupStateFailed,
					lastError: "worktree is busy",
				}}, nil
			},
		)
		if err == nil || !strings.Contains(err.Error(), "task cleanup failed for task-1: worktree is busy") {
			t.Fatalf("failed cleanup error = %v", err)
		}
	})

	t.Run("returns context deadline when cleanup does not finish", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		err := waitForE2ETaskCleanupWithReader(
			ctx,
			[]string{"task-1"},
			time.Hour,
			func(context.Context, []string) ([]e2eTaskCleanupStatus, error) {
				return []e2eTaskCleanupStatus{{
					taskID: "task-1",
					state:  taskmodels.TaskResourceCleanupStateRunning,
				}}, nil
			},
		)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cleanup timeout error = %v, want context deadline exceeded", err)
		}
	})
}

func TestQueryE2ETaskCleanupStatusesBatchesTaskIDs(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "e2e-reset-cleanup.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		CREATE TABLE task_resource_cleanup_jobs (
			task_id TEXT NOT NULL,
			state TEXT NOT NULL,
			last_error TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO task_resource_cleanup_jobs(task_id, state) VALUES ('task-204', 'running');
	`); err != nil {
		t.Fatalf("seed cleanup job: %v", err)
	}

	taskIDs := make([]string, 205)
	for i := range taskIDs {
		taskIDs[i] = "task-" + strconv.Itoa(i)
	}
	statuses, err := queryE2ETaskCleanupStatuses(context.Background(), database.DB, taskIDs)
	if err != nil {
		t.Fatalf("queryE2ETaskCleanupStatuses: %v", err)
	}
	if len(statuses) != 1 || statuses[0].taskID != "task-204" || statuses[0].state != taskmodels.TaskResourceCleanupStateRunning {
		t.Fatalf("cleanup statuses = %+v, want task-204 running", statuses)
	}
}

func TestListE2ETaskIDsIncludesAutomationOwnedTasks(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "e2e-reset-tasks.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`
		CREATE TABLE tasks (id TEXT NOT NULL, workspace_id TEXT NOT NULL, origin TEXT NOT NULL);
		INSERT INTO tasks VALUES ('ordinary-task', 'ws-1', ''),
			('automation-task', 'ws-1', 'automation_run'),
			('other-workspace-task', 'ws-2', '');
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	ids, err := listE2ETaskIDs(context.Background(), database.DB, "ws-1")
	if err != nil {
		t.Fatalf("listE2ETaskIDs: %v", err)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	if len(ids) != 2 {
		t.Fatalf("task IDs = %v, want both ws-1 tasks", ids)
	}
	for _, id := range []string{"ordinary-task", "automation-task"} {
		if _, ok := seen[id]; !ok {
			t.Fatalf("task IDs = %v, missing %s", ids, id)
		}
	}
}

func assertWorkspaceRows(t *testing.T, database *sqlx.DB, table, workspaceID string, want int) {
	t.Helper()
	var got int
	if err := database.Get(&got, `SELECT COUNT(*) FROM `+table+` WHERE workspace_id = ?`, workspaceID); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s rows for %s = %d, want %d", table, workspaceID, got, want)
	}
}
