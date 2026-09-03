package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// TestIsOfficeTaskSessionUniqueViolation_ClassifiesPostgresConstraint
// exercises isOfficeTaskSessionUniqueViolation directly against a real
// PostgreSQL unique-constraint violation. The classifier's two production
// call sites (createTaskSession's INSERT and updateTaskSessionWithStateGuard's
// UPDATE) were deleted per the system design's "Baseline" instruction —
// office session-uniqueness enforcement now lives only in
// CreateOfficeTaskSession's in-transaction guard (session.go), which never
// inspects driver error types — so this function is intentionally
// referenced only by this test and its SQLite sibling
// (TestIsOfficeTaskSessionUniqueViolation_ClassifiesSQLiteMessage).
// uniq_office_task_session is not wired into base_schema.go, so this test
// builds and drops its own scoped index rather than relying on production
// schema, and drives the raw insert directly instead of through
// CreateTaskSession/UpdateTaskSessionIfCurrentState, since neither
// production call path translates this error anymore.
func TestIsOfficeTaskSessionUniqueViolation_ClassifiesPostgresConstraint(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	const taskID = "task-classify-pg"
	seedPostgresTask(t, repo, taskID)
	agent := "agent-classify-pg"

	if _, err := repo.db.Exec(`
		CREATE UNIQUE INDEX uniq_office_task_session
		ON task_sessions(task_id, agent_profile_id)
		WHERE agent_profile_id IS NOT NULL AND state IN (
			'CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT'
		)
	`); err != nil {
		t.Fatalf("create scoped index: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.db.Exec(`DROP INDEX IF EXISTS uniq_office_task_session`)
	})

	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at)
		VALUES (?, ?, ?, 'CREATED', ?, ?)
	`), "sess-classify-pg-1", taskID, agent, now, now); err != nil {
		t.Fatalf("seed first office session: %v", err)
	}

	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, agent_profile_id, state, started_at, updated_at)
		VALUES (?, ?, ?, 'CREATED', ?, ?)
	`), "sess-classify-pg-2", taskID, agent, now, now)
	if err == nil {
		t.Fatal("expected inserting a second live row for the same pair to violate the scoped index")
	}
	if !isOfficeTaskSessionUniqueViolation(err) {
		t.Fatalf("expected err to classify as an office task session unique violation, got: %v", err)
	}
}

// TestIsOfficeTaskSessionUniqueViolation_IgnoresUnrelatedPostgresViolation
// asserts the classifier does not fire on an unrelated PostgreSQL
// unique-constraint violation (the tasks primary key here), mirroring the
// SQLite sibling's negative case.
func TestIsOfficeTaskSessionUniqueViolation_IgnoresUnrelatedPostgresViolation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	const taskID = "task-classify-pg-2"
	seedPostgresTask(t, repo, taskID)

	now := time.Now().UTC()
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), taskID, "ws-"+taskID, taskID, now, now)
	if err == nil {
		t.Fatal("expected duplicate task id to violate the tasks primary key")
	}
	if isOfficeTaskSessionUniqueViolation(err) {
		t.Fatalf("expected primary-key violation not to be misclassified as an office task session unique violation: %v", err)
	}
}
