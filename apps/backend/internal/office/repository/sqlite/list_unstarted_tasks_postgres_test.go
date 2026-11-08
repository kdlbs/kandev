package sqlite_test

import (
	"context"
	"testing"
	"time"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// TestPostgresListUnstartedTasks is the PostgreSQL twin of the
// ListUnstartedTasks SQLite tests. The scheduler recovery sweep binds the
// integer lookback window; the SQLite-only datetime('now', '-' || ? || ' hours')
// form makes Postgres type that parameter as text, so pgx refused to encode
// the int and the sweep failed on every tick (#4741). This runs the real
// query, including the lookback cutoff, the in-flight run exclusion, and the
// excludedTaskIDs path, against a Postgres backend.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListUnstartedTasks(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("init settings store: %v", err)
	}
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, err := workflowrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init workflow repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	now := time.Now().UTC()
	seed := func(id string, createdAt time.Time) {
		t.Helper()
		execPostgres(t, ctx, repo, `
			INSERT INTO tasks (id, workspace_id, project_id, state, title, description, identifier, created_at, updated_at)
			VALUES (?, 'pg-unstarted-ws', 'pg-unstarted-project', 'TODO', 'Task', '', '', ?, ?)
		`, id, createdAt, createdAt)
		seedPostgresRunner(t, ctx, repo, id)
	}
	seed("pg-unstarted-fresh-a", now.Add(-2*time.Hour))
	seed("pg-unstarted-fresh-b", now.Add(-time.Hour))
	seed("pg-unstarted-stale", now.Add(-48*time.Hour))
	seed("pg-unstarted-queued", now.Add(-time.Hour))
	seedPostgresRun(t, ctx, repo, "pg-unstarted-run", "pg-unstarted-queued-agent", "queued", now,
		`{"task_id":"pg-unstarted-queued"}`)

	for _, runCase := range []struct {
		status string
		age    time.Duration
	}{
		{status: "claimed", age: 90 * time.Minute},
		{status: "finished", age: 45 * time.Minute},
		{status: "failed", age: 30 * time.Minute},
		{status: "cancelled", age: 15 * time.Minute},
	} {
		taskID := "pg-unstarted-" + runCase.status
		createdAt := now.Add(-runCase.age)
		seed(taskID, createdAt)
		seedPostgresRun(t, ctx, repo, taskID+"-run", taskID+"-agent", runCase.status, createdAt.Add(time.Minute),
			`{"task_id":"`+taskID+`"}`)
	}

	rows, err := repo.ListUnstartedTasks(ctx, 24, 10)
	if err != nil {
		t.Fatalf("ListUnstartedTasks: %v", err)
	}
	if got := unstartedIDs(rows); !equalIDs(got, []string{
		"pg-unstarted-fresh-a", "pg-unstarted-fresh-b", "pg-unstarted-failed", "pg-unstarted-cancelled",
	}) {
		t.Fatalf("ListUnstartedTasks = %v, want recoverable tasks oldest first", got)
	}
	if rows[0].AssigneeAgentProfileID != "pg-unstarted-fresh-a-agent" {
		t.Errorf("assignee = %q, want runner agent", rows[0].AssigneeAgentProfileID)
	}

	rows, err = repo.ListUnstartedTasks(ctx, 24, 10, "pg-unstarted-fresh-a")
	if err != nil {
		t.Fatalf("ListUnstartedTasks with exclusions: %v", err)
	}
	if got := unstartedIDs(rows); !equalIDs(got, []string{
		"pg-unstarted-fresh-b", "pg-unstarted-failed", "pg-unstarted-cancelled",
	}) {
		t.Fatalf("ListUnstartedTasks(excluded) = %v, want remaining recoverable tasks", got)
	}

	rows, err = repo.ListUnstartedTasks(ctx, 72, 10)
	if err != nil {
		t.Fatalf("ListUnstartedTasks wide window: %v", err)
	}
	if got := unstartedIDs(rows); !equalIDs(got, []string{
		"pg-unstarted-stale", "pg-unstarted-fresh-a", "pg-unstarted-fresh-b",
		"pg-unstarted-failed", "pg-unstarted-cancelled",
	}) {
		t.Fatalf("ListUnstartedTasks(72h) = %v, want all recoverable tasks oldest first", got)
	}
}

func unstartedIDs(rows []*sqlite.UnstartedTaskRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestPostgresListUnstartedTasksNonUTCSession checks the lookback cutoff when
// the PostgreSQL session time zone is not UTC. tasks.created_at is a naive
// TIMESTAMP holding UTC wall-clock values, so comparing it against NOW()
// without normalizing it reads it in the session zone and shifts the window
// by the zone offset. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListUnstartedTasksNonUTCSession(t *testing.T) {
	for _, tz := range []string{"Asia/Shanghai", "America/Los_Angeles"} {
		t.Run(tz, func(t *testing.T) {
			db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			ctx := context.Background()

			if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
				t.Fatalf("init settings store: %v", err)
			}
			if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
				t.Fatalf("init task repo: %v", err)
			}
			if _, err := workflowrepo.NewWithDB(db, db, nil); err != nil {
				t.Fatalf("init workflow repo: %v", err)
			}
			repo, err := sqlite.NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("init office repo: %v", err)
			}
			// OpenIsolatedPostgres pins the pool to one connection, so the
			// session setting applies to every query below.
			if _, err := db.ExecContext(ctx, "SET TIME ZONE '"+tz+"'"); err != nil {
				t.Fatalf("set time zone: %v", err)
			}

			now := time.Now().UTC()
			seed := func(id string, createdAt time.Time) {
				t.Helper()
				execPostgres(t, ctx, repo, `
					INSERT INTO tasks (id, workspace_id, project_id, state, title, description, identifier, created_at, updated_at)
					VALUES (?, 'pg-unstarted-ws', 'pg-unstarted-project', 'TODO', 'Task', '', '', ?, ?)
				`, id, createdAt, createdAt)
				seedPostgresRunner(t, ctx, repo, id)
			}
			// 20h old is inside a 24h window; 30h old is outside it. Both are
			// more than the zone offset away from the cutoff.
			seed("pg-tz-inside", now.Add(-20*time.Hour))
			seed("pg-tz-outside", now.Add(-30*time.Hour))

			rows, err := repo.ListUnstartedTasks(ctx, 24, 10)
			if err != nil {
				t.Fatalf("ListUnstartedTasks: %v", err)
			}
			if got := unstartedIDs(rows); !equalIDs(got, []string{"pg-tz-inside"}) {
				t.Fatalf("ListUnstartedTasks with session time zone %s = %v, want only the 20h-old task", tz, got)
			}
		})
	}
}
