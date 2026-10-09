package backendapp

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type e2eResetTaskDeleterStub struct {
	taskID  string
	options taskservice.DeleteTaskOptions
}

type e2eAttachTaskAuthorizerStub struct {
	err    error
	taskID string
}

func (s *e2eAttachTaskAuthorizerStub) AuthorizeTaskAccess(_ context.Context, taskID string) error {
	s.taskID = taskID
	return s.err
}

func (s *e2eResetTaskDeleterStub) DeleteTaskWithOptions(
	_ context.Context,
	taskID string,
	options taskservice.DeleteTaskOptions,
) error {
	s.taskID = taskID
	s.options = options
	return nil
}

func TestDeleteTaskForE2EResetDiscardsWorktreeChanges(t *testing.T) {
	deleter := &e2eResetTaskDeleterStub{}

	if err := deleteTaskForE2EReset(context.Background(), deleter, "task-1"); err != nil {
		t.Fatalf("deleteTaskForE2EReset: %v", err)
	}
	if deleter.taskID != "task-1" {
		t.Fatalf("deleted task ID = %q, want task-1", deleter.taskID)
	}
	if !deleter.options.DiscardWorktreeChanges {
		t.Fatal("E2E reset must discard disposable worktree changes")
	}
}

func TestOrderE2ETasksForDeletionPlacesChildrenFirst(t *testing.T) {
	tasks := []*taskmodels.Task{
		{ID: "root"},
		{ID: "sibling"},
		{ID: "child", ParentID: "root"},
		{ID: "grandchild", ParentID: "child"},
		{ID: "sibling-child", ParentID: "sibling"},
	}

	ordered, err := orderE2ETasksForDeletion(tasks)
	if err != nil {
		t.Fatalf("orderE2ETasksForDeletion(): %v", err)
	}
	if len(ordered) != len(tasks) {
		t.Fatalf("ordered task count = %d, want %d", len(ordered), len(tasks))
	}

	positions := make(map[string]int, len(ordered))
	for index, task := range ordered {
		positions[task.ID] = index
	}
	for _, task := range tasks {
		if task.ParentID == "" {
			continue
		}
		parentPosition, parentInList := positions[task.ParentID]
		if parentInList && positions[task.ID] >= parentPosition {
			t.Errorf(
				"task %q at %d must precede parent %q at %d",
				task.ID,
				positions[task.ID],
				task.ParentID,
				parentPosition,
			)
		}
	}
}

func TestOrderE2ETasksForDeletionRejectsParentCycles(t *testing.T) {
	tasks := []*taskmodels.Task{
		{ID: "first", ParentID: "second"},
		{ID: "second", ParentID: "first"},
	}

	if _, err := orderE2ETasksForDeletion(tasks); err == nil {
		t.Fatal("orderE2ETasksForDeletion() error = nil, want a hierarchy cycle error")
	}
}

func TestE2EAttachGitHubContributionRejectsUnauthorizedTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authorizer := &e2eAttachTaskAuthorizerStub{err: errors.New("task is not visible")}
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Params = gin.Params{{Key: "id", Value: "other-task"}}
	context.Request = httptest.NewRequest(
		"POST",
		"/api/v1/e2e/tasks/other-task/remote-contribution",
		strings.NewReader(`{"pr_url":"https://github.com/testorg/testrepo/pull/1"}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	handleE2EAttachGitHubContribution(nil, authorizer, nil, nil)(context)

	if response.Code != 404 {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if authorizer.taskID != "other-task" {
		t.Fatalf("authorized task ID = %q, want other-task", authorizer.taskID)
	}
}

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

func TestDeleteCoordinatorStateForReset_NilServiceIsNoop(t *testing.T) {
	if err := deleteCoordinatorStateForReset(context.Background(), nil, "ws-1"); err != nil {
		t.Fatalf("deleteCoordinatorStateForReset(nil service) = %v, want nil", err)
	}
}

func TestDeleteCoordinatorStateForReset_DeletesOnlyTargetWorkspace(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "e2e-reset-coordinator.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })

	store, err := coordinator.NewStore(database, database)
	if err != nil {
		t.Fatalf("new coordinator store: %v", err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	svc := coordinator.NewService(store, nil, nil, log)

	seedCoordinatorWorkspaceRows(t, database, "ws-1")
	seedCoordinatorWorkspaceRows(t, database, "ws-2")

	if err := deleteCoordinatorStateForReset(context.Background(), svc, "ws-1"); err != nil {
		t.Fatalf("deleteCoordinatorStateForReset: %v", err)
	}

	for _, table := range []string{"coordinators", "coordinator_proposals", "coordinator_stalls"} {
		assertWorkspaceRows(t, database, table, "ws-1", 0)
		assertWorkspaceRows(t, database, table, "ws-2", 1)
	}
}

func seedCoordinatorWorkspaceRows(t *testing.T, database *sqlx.DB, workspaceID string) {
	t.Helper()
	coordinatorID := "coordinator-" + workspaceID
	if _, err := database.Exec(
		`INSERT INTO coordinators (id, workspace_id, name, agent_profile_id, executor_profile_id, context, created_at, updated_at)
		 VALUES (?, ?, 'Planner', 'agent-1', 'executor-1', '', datetime('now'), datetime('now'))`,
		coordinatorID, workspaceID,
	); err != nil {
		t.Fatalf("seed coordinators: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at)
		 VALUES (?, ?, ?, 'open', '{}', datetime('now'), datetime('now'))`,
		"proposal-"+workspaceID, coordinatorID, workspaceID,
	); err != nil {
		t.Fatalf("seed coordinator_proposals: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO coordinator_stalls (task_id, workspace_id, stalled_for_ms, last_event_at, detected_at)
		 VALUES (?, ?, 1000, datetime('now'), datetime('now'))`,
		"task-"+workspaceID, workspaceID,
	); err != nil {
		t.Fatalf("seed coordinator_stalls: %v", err)
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

func TestE2EResetDeletionOrdersChildrenBeforeParents(t *testing.T) {
	tasks := []*taskmodels.Task{
		{ID: "root"}, {ID: "other"}, {ID: "child", ParentID: "root"},
		{ID: "grandchild", ParentID: "child"}, {ID: "sibling", ParentID: "root"},
		{ID: "external-child", ParentID: "outside-workspace"},
	}
	for _, reverse := range []bool{false, true} {
		input := append([]*taskmodels.Task(nil), tasks...)
		if reverse {
			for i, j := 0, len(input)-1; i < j; i, j = i+1, j-1 {
				input[i], input[j] = input[j], input[i]
			}
		}
		ordered, err := orderE2ETasksForDeletion(input)
		if err != nil {
			t.Fatal(err)
		}
		positions := make(map[string]int)
		for i, task := range ordered {
			if _, duplicate := positions[task.ID]; duplicate {
				t.Fatalf("duplicate deletion of %s", task.ID)
			}
			positions[task.ID] = i
		}
		if len(positions) != len(tasks) {
			t.Fatalf("deletion count = %d, want %d", len(positions), len(tasks))
		}
		for _, task := range tasks {
			if parentIndex, exists := positions[task.ParentID]; exists && positions[task.ID] >= parentIndex {
				t.Fatalf("parent %s would be deleted before child %s", task.ParentID, task.ID)
			}
		}
	}
}

func TestE2EResetDeletionRejectsHierarchyCycleBeforeDeleting(t *testing.T) {
	ordered, err := orderE2ETasksForDeletion([]*taskmodels.Task{
		{ID: "a", ParentID: "b"}, {ID: "b", ParentID: "a"},
	})
	if err == nil || len(ordered) != 0 {
		t.Fatalf("cyclic deletion plan = %v, %v; want no deletions and an error", ordered, err)
	}
}
