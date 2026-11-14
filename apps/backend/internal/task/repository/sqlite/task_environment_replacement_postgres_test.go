package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

const (
	pgReplacementTask    = "task-pg-replacement"
	pgReplacementOwner   = "session-pg-replacement-owner"
	pgReplacementSibling = "session-pg-replacement-sibling"
)

type pgReplacementResult struct {
	sessionID string
	env       *models.TaskEnvironment
	err       error
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
func TestPostgresReplacementElectsOneReservationAndRetainsInventory(t *testing.T) {
	db, repo, ctx := openPostgresReplacementRepo(t)
	retained := seedPostgresRetainedFailedEnvironment(t, ctx, repo)
	holder := holdPostgresMaterializationLock(t, ctx, db, retained.ID)

	results := make(chan pgReplacementResult, 2)
	for _, session := range []*models.TaskSession{
		{ID: pgReplacementOwner, TaskID: pgReplacementTask, TaskEnvironmentID: retained.ID, State: models.TaskSessionStateFailed},
		{ID: pgReplacementSibling, TaskID: pgReplacementTask, TaskEnvironmentID: retained.ID, State: models.TaskSessionStateCompleted},
	} {
		go func(session *models.TaskSession) {
			env, err := repo.ReserveTaskEnvironmentReplacement(ctx, retained, session, &models.TaskEnvironment{
				TaskID: pgReplacementTask, ExecutorType: string(models.ExecutorTypeLocal), TaskDirName: "pg-replacement",
			})
			results <- pgReplacementResult{sessionID: session.ID, env: env, err: err}
		}(session)
	}
	// One request waits for the environment lock, the other for the task row.
	waitForPostgresLockWaiters(t, ctx, db, 2)
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}

	var winner *pgReplacementResult
	refused := 0
	for range 2 {
		select {
		case result := <-results:
			var preparing *models.WorkspacePreparingError
			switch {
			case result.err == nil:
				if winner != nil {
					t.Fatalf("two replacements were reserved: %s and %s", winner.env.ID, result.env.ID)
				}
				winner = &result
			case errors.As(result.err, &preparing):
				refused++
			default:
				t.Fatalf("replacement for %s: %v", result.sessionID, result.err)
			}
		case <-ctx.Done():
			t.Fatal("replacement did not finish after lock release")
		}
	}
	if winner == nil || refused != 1 {
		t.Fatalf("winner=%v refused=%d, want one reservation and one preparing refusal", winner, refused)
	}
	if winner.env.ID == retained.ID || winner.env.Status != models.TaskEnvironmentStatusCreating ||
		winner.env.MaterializationSessionID != winner.sessionID {
		t.Fatalf("reservation = %s status %q owner %q", winner.env.ID, winner.env.Status, winner.env.MaterializationSessionID)
	}
	var active int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_environments WHERE task_id = $1 AND retired_at IS NULL`,
		pgReplacementTask).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active environments = %d, want 1", active)
	}

	// The retired owner keeps its identity, inventory and the loser's binding.
	history, err := repo.GetTaskEnvironment(ctx, retained.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !history.IsRetired() || len(history.Repos) != 1 || history.Repos[0].WorktreePath != "/retained" {
		t.Fatalf("retained environment = retired %v repos %+v", history.IsRetired(), history.Repos)
	}
	loser := pgReplacementOwner
	if winner.sessionID == pgReplacementOwner {
		loser = pgReplacementSibling
	}
	assertPostgresSessionBinding(t, ctx, db, winner.sessionID, winner.env.ID)
	assertPostgresSessionBinding(t, ctx, db, loser, retained.ID)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.11
func TestPostgresReplacementRefusesConsumerAfterLockWait(t *testing.T) {
	db, repo, ctx := openPostgresReplacementRepo(t)
	retained := seedPostgresRetainedFailedEnvironment(t, ctx, repo)
	holder := holdPostgresMaterializationLock(t, ctx, db, retained.ID)

	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, err := repo.ReserveTaskEnvironmentReplacement(ctx, retained, &models.TaskSession{
			ID: pgReplacementOwner, TaskID: pgReplacementTask, TaskEnvironmentID: retained.ID, State: models.TaskSessionStateFailed,
		}, &models.TaskEnvironment{TaskID: pgReplacementTask, ExecutorType: string(models.ExecutorTypeLocal), TaskDirName: "pg-replacement"})
		done <- err
	}()
	waitForMaterializationAdvisoryLock(t, ctx, db, retained.ID, finished)
	// A sibling resumes on the failed environment while the replacement waits.
	if _, err := holder.ExecContext(ctx, `UPDATE task_sessions SET state = $1 WHERE id = $2`,
		string(models.TaskSessionStateRunning), pgReplacementSibling); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
			t.Fatalf("replacement error = %v, want reset required", err)
		}
	case <-ctx.Done():
		t.Fatal("replacement did not finish after lock release")
	}
	current, err := repo.GetTaskEnvironment(ctx, retained.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.IsRetired() || current.Status != models.TaskEnvironmentStatusFailed {
		t.Fatalf("consumed environment = retired %v status %q, want unchanged failed owner", current.IsRetired(), current.Status)
	}
	var environments int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_environments WHERE task_id = $1`, pgReplacementTask).Scan(&environments); err != nil {
		t.Fatal(err)
	}
	if environments != 1 {
		t.Fatalf("environments = %d, want no replacement reservation", environments)
	}
	assertPostgresSessionBinding(t, ctx, db, pgReplacementOwner, retained.ID)
}

func openPostgresReplacementRepo(t *testing.T) (*sqlx.DB, *tasksqlite.Repository, context.Context) {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(6)
	db.SetMaxIdleConns(6)
	repo, err := tasksqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return db, repo, ctx
}

// seedPostgresRetainedFailedEnvironment leaves a failed environment with
// physical inventory and two terminal sessions bound to it.
func seedPostgresRetainedFailedEnvironment(t *testing.T, ctx context.Context, repo *tasksqlite.Repository) *models.TaskEnvironment {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-pg-replacement", Name: "PG replacement"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: pgReplacementTask, WorkspaceID: "ws-pg-replacement", Title: pgReplacementTask}); err != nil {
		t.Fatal(err)
	}
	owner := &models.TaskSession{ID: pgReplacementOwner, TaskID: pgReplacementTask}
	if err := repo.CreateTaskSessionWithWorkspaceBinding(ctx, owner, &models.TaskEnvironment{
		TaskID: pgReplacementTask, ExecutorType: string(models.ExecutorTypeLocal),
	}); err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.FailTaskEnvironmentMaterialization(ctx, owner.ID); err != nil || !changed {
		t.Fatalf("fail seeded claim: changed=%v err=%v", changed, err)
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{ID: "pg-replacement-inventory",
		TaskEnvironmentID: owner.TaskEnvironmentID, RepositoryID: "pg-repository", WorktreePath: "/retained"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskSessionState(ctx, owner.ID, models.TaskSessionStateFailed, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: pgReplacementSibling, TaskID: pgReplacementTask}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET task_environment_id = $1, state = $2 WHERE id = $3`,
		owner.TaskEnvironmentID, string(models.TaskSessionStateCompleted), pgReplacementSibling); err != nil {
		t.Fatal(err)
	}
	retained, err := repo.GetTaskEnvironment(ctx, owner.TaskEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	return retained
}

func holdPostgresMaterializationLock(t *testing.T, ctx context.Context, db *sqlx.DB, environmentID string) *sqlx.Tx {
	t.Helper()
	holder, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Rollback() })
	if _, err := holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"task-environment-materialization:"+environmentID); err != nil {
		t.Fatal(err)
	}
	return holder
}

func waitForPostgresLockWaiters(t *testing.T, ctx context.Context, db *sqlx.DB, want int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_stat_activity
 WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("observed fewer than %d lock waiters", want)
		case <-ticker.C:
		}
	}
}

func assertPostgresSessionBinding(t *testing.T, ctx context.Context, db *sqlx.DB, sessionID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(task_environment_id, '') FROM task_sessions WHERE id = $1`, sessionID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("session %s binding = %q, want %q", sessionID, got, want)
	}
}
