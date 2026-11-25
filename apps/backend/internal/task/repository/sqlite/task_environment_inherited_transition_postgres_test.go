package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func seedPostgresInheritedTransition(t *testing.T, repo *Repository, suffix string) (*models.TaskEnvironment, []*models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	workspaceID := "workspace-inherited-transition-pg-" + suffix
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	parentID := "a-parent-inherited-transition-pg-" + suffix
	childID := "z-child-inherited-transition-pg-" + suffix
	for _, task := range []*models.Task{
		{ID: parentID, WorkspaceID: workspaceID, Title: parentID},
		{ID: childID, WorkspaceID: workspaceID, ParentID: parentID, Title: childID},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	source := &models.TaskEnvironment{
		ID: "environment-inherited-transition-pg-" + suffix, TaskID: parentID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: "/parent/" + suffix,
	}
	if err := repo.CreateTaskEnvironment(ctx, source); err != nil {
		t.Fatalf("create source environment: %v", err)
	}
	sessions := []*models.TaskSession{
		{ID: "session-inherited-transition-pg-owner-" + suffix, TaskID: childID,
			TaskEnvironmentID: source.ID, WorkspacePath: source.WorkspacePath, State: models.TaskSessionStateCreated},
		{ID: "session-inherited-transition-pg-successor-" + suffix, TaskID: childID,
			TaskEnvironmentID: source.ID, WorkspacePath: source.WorkspacePath, State: models.TaskSessionStateCreated},
	}
	for _, session := range sessions {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("create inherited session: %v", err)
		}
	}
	return source, sessions
}

func runPostgresInheritedTransitionClaim(
	ctx context.Context,
	repo *Repository,
	source *models.TaskEnvironment,
	session *models.TaskSession,
) error {
	_, _, _, err := repo.ClaimInheritedTaskEnvironmentTransition(
		ctx, session.TaskID, session.ID, source.ID,
		string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
	)
	return err
}

func startPostgresInheritedTransitionClaim(
	ctx context.Context,
	repo *Repository,
	source *models.TaskEnvironment,
	session *models.TaskSession,
) (<-chan error, <-chan struct{}) {
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- runPostgresInheritedTransitionClaim(ctx, repo, source, session)
	}()
	return done, finished
}

func inheritedTransitionPostgresRepositories(t *testing.T, dsn string, db *sqlx.DB) (*Repository, *sqlx.DB) {
	t.Helper()
	workerDB := openSecondPostgresConnection(t, dsn, db)
	observerDB := openSecondPostgresConnection(t, dsn, db)
	worker, err := NewWithDB(workerDB, workerDB, nil)
	if err != nil {
		t.Fatalf("initialize worker repository: %v", err)
	}
	return worker, observerDB
}

// PostgreSQL-only evidence for the transaction's task-row lock before the
// inherited-environment advisory lock. The worker must re-read the source
// executor after it obtains the row lock rather than use its earlier identity.
func TestPostgresInheritedTransitionRechecksSourceAfterTaskRowLock(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres repository: %v", err)
	}
	worker, observer := inheritedTransitionPostgresRepositories(t, dsn, db)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	source, sessions := seedPostgresInheritedTransition(t, repo, "task-row-lock")

	holder, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Rollback() })
	var lockedTask string
	parentTaskID := "a-parent-inherited-transition-pg-task-row-lock"
	if err := holder.QueryRowContext(ctx, `SELECT id FROM tasks WHERE id = $1 FOR UPDATE`, parentTaskID).Scan(&lockedTask); err != nil {
		t.Fatalf("lock inherited owner task: %v", err)
	}
	if lockedTask != parentTaskID {
		t.Fatalf("locked task = %q, want %q", lockedTask, parentTaskID)
	}
	var workerPID int
	if err := worker.db.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&workerPID); err != nil {
		t.Fatalf("read worker backend PID: %v", err)
	}
	done, finished := startPostgresInheritedTransitionClaim(ctx, worker, source, sessions[0])
	t.Cleanup(func() {
		cancel()
		_ = holder.Rollback()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("task-row-locked inherited transition worker did not stop")
		}
	})
	if err := waitForPostgresLock(ctx, observer, workerPID, finished); err != nil {
		t.Fatalf("worker did not wait for the inherited owner task row: %v", err)
	}
	if _, err := holder.ExecContext(ctx, db.Rebind(`UPDATE task_environments SET executor_type = ? WHERE id = ?`),
		string(models.ExecutorTypeLocalDocker), source.ID); err != nil {
		t.Fatalf("change source executor while claim waits: %v", err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("release task-row lock: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("claim after task-row lock release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("claim did not finish after releasing the task-row lock")
	}
	childEnvironment, err := repo.GetTaskEnvironmentByTaskID(ctx, sessions[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if childEnvironment != nil {
		t.Fatalf("stale transition created child environment %+v after source became compatible", childEnvironment)
	}
	bound, err := repo.GetTaskSession(ctx, sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if bound.TaskEnvironmentID != source.ID {
		t.Fatalf("session binding = %q, want unchanged inherited environment %q", bound.TaskEnvironmentID, source.ID)
	}
}

// PostgreSQL-only evidence that the inherited source is re-read after waiting
// for the environment materialization advisory lock.
func TestPostgresInheritedTransitionRechecksSourceAfterAdvisoryLock(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres repository: %v", err)
	}
	worker, observer := inheritedTransitionPostgresRepositories(t, dsn, db)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	source, sessions := seedPostgresInheritedTransition(t, repo, "advisory-lock")

	holder, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Rollback() })
	if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		taskEnvironmentMaterializationLockNamespace+source.ID); err != nil {
		t.Fatalf("lock inherited environment materialization: %v", err)
	}
	var workerPID int
	if err := worker.db.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&workerPID); err != nil {
		t.Fatalf("read worker backend PID: %v", err)
	}
	done, finished := startPostgresInheritedTransitionClaim(ctx, worker, source, sessions[0])
	t.Cleanup(func() {
		cancel()
		_ = holder.Rollback()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("advisory-locked inherited transition worker did not stop")
		}
	})
	if err := waitForPostgresLock(ctx, observer, workerPID, finished); err != nil {
		t.Fatalf("worker did not wait for inherited materialization advisory lock: %v", err)
	}
	if _, err := holder.ExecContext(ctx, db.Rebind(`UPDATE task_environments SET executor_type = ? WHERE id = ?`),
		string(models.ExecutorTypeLocalDocker), source.ID); err != nil {
		t.Fatalf("change source executor while claim waits: %v", err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("release materialization advisory lock: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("claim after advisory lock release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("claim did not finish after releasing the materialization advisory lock")
	}
	childEnvironment, err := repo.GetTaskEnvironmentByTaskID(ctx, sessions[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if childEnvironment != nil {
		t.Fatalf("stale transition created child environment %+v after source became compatible", childEnvironment)
	}
	bound, err := repo.GetTaskSession(ctx, sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if bound.TaskEnvironmentID != source.ID {
		t.Fatalf("session binding = %q, want unchanged inherited environment %q", bound.TaskEnvironmentID, source.ID)
	}
}

func TestPostgresInheritedTransitionSuccessorClaimFencesOldFinalizer(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres repository: %v", err)
	}
	worker, _ := inheritedTransitionPostgresRepositories(t, dsn, db)
	ctx := context.Background()
	source, sessions := seedPostgresInheritedTransition(t, repo, "successor-cas")
	owner, successor := sessions[0], sessions[1]
	first, handled, _, err := repo.ClaimInheritedTaskEnvironmentTransition(
		ctx, owner.TaskID, owner.ID, source.ID,
		string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
	)
	if err != nil || !handled || first == nil {
		t.Fatalf("initial claim = %+v, handled=%v, error=%v", first, handled, err)
	}
	if err := repo.UpdateTaskSessionState(ctx, owner.ID, models.TaskSessionStateCancelled, "cancelled during workspace preparation"); err != nil {
		t.Fatalf("cancel former materializer: %v", err)
	}
	second, handled, _, err := worker.ClaimInheritedTaskEnvironmentTransition(
		ctx, successor.TaskID, successor.ID, source.ID,
		string(models.ExecutorTypeLocalDocker), "docker-child", "profile-child",
	)
	if err != nil || !handled || second == nil {
		t.Fatalf("successor claim = %+v, handled=%v, error=%v", second, handled, err)
	}
	if second.ID != first.ID || second.Status != models.TaskEnvironmentStatusCreating || second.MaterializationSessionID != successor.ID {
		t.Fatalf("successor environment = %+v, want same creating row owned by %s", second, successor.ID)
	}
	stale := *first
	stale.Status = models.TaskEnvironmentStatusReady
	stale.WorkspacePath = "/stale-owner/workspace"
	if err := repo.FinalizeTaskEnvironmentMaterialization(ctx, &stale, nil, owner.ID); err == nil {
		t.Fatal("former owner finalized the successor's PostgreSQL materialization claim")
	}
	current, err := repo.GetTaskEnvironment(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != models.TaskEnvironmentStatusCreating || current.MaterializationSessionID != successor.ID || current.WorkspacePath != "" {
		t.Fatalf("environment after stale publication = %+v, want successor claim unchanged", current)
	}
}
