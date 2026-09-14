package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresGetTaskAssignee is the PostgreSQL twin of
// TestGetTaskAssignee_MostRecentlyAssignedRunnerWins: RunnerProjection's
// task-scoped fallback arm (base.go) used to order by `wsp.rowid`, a SQLite
// pseudo-column with no Postgres equivalent, so every call to
// GetTaskAssignee failed unconditionally on Postgres with
// "column wsp.rowid does not exist" — even for a task with zero participant
// rows, since Postgres validates the whole COALESCE expression at plan
// time. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresGetTaskAssignee(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	// tasks/workflow_steps/workflow_step_participants are created by the
	// task repository's schema init, mirroring production boot order (see
	// failure_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	// A task with zero participant rows must not error — the fix must not
	// merely avoid the error when the fallback arm actually fires.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES ('pg-task-empty', 'Empty task', now(), now())
	`); err != nil {
		t.Fatalf("seed empty task: %v", err)
	}
	got, err := repo.GetTaskAssignee(ctx, "pg-task-empty")
	if err != nil {
		t.Fatalf("GetTaskAssignee (no participants): %v", err)
	}
	if got != "" {
		t.Fatalf("GetTaskAssignee (no participants) = %q, want \"\"", got)
	}

	// The task-scoped fallback arm: two runner rows at different steps,
	// neither matching the task's current (participant-less) step, so
	// resolution falls through to "most recently assigned across any
	// step" — ordered by created_at, not insertion order.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES ('pg-task-x', 'Task X', now(), now())
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	recent := time.Now().UTC()
	stale := recent.Add(-time.Hour)
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES ('pg-p-recent', 'step-old-a', 'pg-task-x', 'runner', 'agent-recent', 0, 0, ?)
	`, recent); err != nil {
		t.Fatalf("insert recent runner: %v", err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position, created_at)
		VALUES ('pg-p-stale', 'step-old-b', 'pg-task-x', 'runner', 'agent-stale', 0, 0, ?)
	`, stale); err != nil {
		t.Fatalf("insert stale runner: %v", err)
	}

	got, err = repo.GetTaskAssignee(ctx, "pg-task-x")
	if err != nil {
		t.Fatalf("GetTaskAssignee: %v", err)
	}
	if got != "agent-recent" {
		t.Fatalf("GetTaskAssignee = %q, want agent-recent (highest created_at)", got)
	}
}
