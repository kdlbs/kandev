package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func newBarrierTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "barrier.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func seedBarrierTask(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'barrier task', ?, ?)`), taskID, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func reserveBarrier(t *testing.T, repo *Repository, taskID, operationID string) {
	t.Helper()
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: taskID,
		Trigger: models.TaskResourceCleanupTriggerArchive,
		State:   models.TaskResourceCleanupStatePrepared,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("reserve barrier: %v", err)
	}
}

func TestTaskCleanupBarrier_RejectsSessionCreation(t *testing.T) {
	repo := newBarrierTestRepo(t)
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-session")
	reserveBarrier(t, repo, "task-barrier-session", "op-session")

	err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-barrier", TaskID: "task-barrier-session",
		State: models.TaskSessionStateCreated,
	})
	if !errors.Is(err, repoerrors.ErrTaskCleanupInProgress) {
		t.Fatalf("CreateTaskSession error = %v, want ErrTaskCleanupInProgress", err)
	}
}

func TestTaskCleanupBarrier_RejectsEnvironmentCreation(t *testing.T) {
	repo := newBarrierTestRepo(t)
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-env")
	reserveBarrier(t, repo, "task-barrier-env", "op-env")

	err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-barrier", TaskID: "task-barrier-env", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	})
	if !errors.Is(err, repoerrors.ErrTaskCleanupInProgress) {
		t.Fatalf("CreateTaskEnvironment error = %v, want ErrTaskCleanupInProgress", err)
	}
}

func TestTaskCleanupBarrier_RejectsEnvRepoCreation(t *testing.T) {
	repo := newBarrierTestRepo(t)
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-repo")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-barrier-repo", TaskID: "task-barrier-repo", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("seed env: %v", err)
	}
	reserveBarrier(t, repo, "task-barrier-repo", "op-repo")

	err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		TaskEnvironmentID: "env-barrier-repo", RepositoryID: "repo-1",
		WorktreeID: "wt-1", WorktreePath: "/tmp/wt-1",
	})
	if !errors.Is(err, repoerrors.ErrTaskCleanupInProgress) {
		t.Fatalf("CreateTaskEnvironmentRepo error = %v, want ErrTaskCleanupInProgress", err)
	}
}

func TestTaskCleanupBarrier_AllowsCreationWithoutBarrier(t *testing.T) {
	repo := newBarrierTestRepo(t)
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-open")

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-open", TaskID: "task-barrier-open", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession without barrier: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-open", TaskID: "task-barrier-open", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment without barrier: %v", err)
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		TaskEnvironmentID: "env-open", RepositoryID: "repo-1",
		WorktreeID: "wt-1", WorktreePath: "/tmp/wt-1",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo without barrier: %v", err)
	}
}

// TestTaskCleanupBarrier_CommittedCreationIsIncludedInInventory proves the
// inverse race ordering: a session/worktree that commits before the barrier
// is reserved remains visible to the later inventory.
func TestTaskCleanupBarrier_CommittedCreationIsIncludedInInventory(t *testing.T) {
	repo := newBarrierTestRepo(t)
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-order")

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-order", TaskID: "task-barrier-order", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	reserveBarrier(t, repo, "task-barrier-order", "op-order")

	sessions, err := repo.ListTaskSessions(ctx, "task-barrier-order")
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "session-order" {
		t.Fatalf("inventory = %+v, want the committed session", sessions)
	}
}

// TestTaskCleanupBarrier_ReleasedBarrierAllowsCreation proves a completed or
// cancelled barrier no longer blocks session creation.
func TestTaskCleanupBarrier_ReleasedBarrierAllowsCreation(t *testing.T) {
	repo := newBarrierTestRepo(t)
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-released")
	reserveBarrier(t, repo, "task-barrier-released", "op-released")
	job, err := repo.GetTaskResourceCleanupJobByOperationID(ctx, "op-released")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if err := repo.CompleteTaskResourceCleanupJob(ctx, job.ID, models.TaskResourceCleanupStateCancelled, "", nil); err != nil {
		t.Fatalf("cancel barrier: %v", err)
	}

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-released", TaskID: "task-barrier-released", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("CreateTaskSession after barrier release: %v", err)
	}
}

func TestTaskCleanupBarrierPostgres_RejectsSessionCreation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres: %v", err)
	}
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-pg")
	reserveBarrier(t, repo, "task-barrier-pg", "op-pg")

	err = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-pg", TaskID: "task-barrier-pg", State: models.TaskSessionStateCreated,
	})
	if !errors.Is(err, repoerrors.ErrTaskCleanupInProgress) {
		t.Fatalf("postgres CreateTaskSession error = %v, want ErrTaskCleanupInProgress", err)
	}
}

func TestTaskCleanupBarrierPostgres_AllowsCreationWithoutBarrier(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres: %v", err)
	}
	ctx := context.Background()
	seedBarrierTask(t, repo, "task-barrier-pg-open")

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-pg-open", TaskID: "task-barrier-pg-open", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("postgres CreateTaskSession without barrier: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-pg-open", TaskID: "task-barrier-pg-open", ExecutorType: "worktree",
		WorkspacePath: "/tmp", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("postgres CreateTaskEnvironment without barrier: %v", err)
	}
}
