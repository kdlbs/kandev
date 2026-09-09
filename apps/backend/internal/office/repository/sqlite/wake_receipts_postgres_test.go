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

// TestPostgresListStuckParents is the PostgreSQL twin of the SQLite
// ListStuckParents suite. The query carried several SQLite-only constructs —
// GROUP_CONCAT, json_extract(...), and IS NOT with a column right-hand side
// are the three fixed here; a fourth, wsp.rowid, was already fixed in #3459 —
// each a parse error on Postgres, so ParentWakeReconciler's backing query
// could not run there at all and the reconciler recovered nothing. Postgres
// rejects the whole statement at parse time, so this fails on the first
// construct rather than returning wrong rows.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListStuckParents(t *testing.T) {
	repo, ctx := newPostgresWakeRepo(t)

	wantKey := seedPostgresStuckParent(t, ctx, repo, "pg-parent-1", "pg-ws-1")

	rows, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ParentTaskID != "pg-parent-1" {
		t.Errorf("ParentTaskID = %q, want %q", rows[0].ParentTaskID, "pg-parent-1")
	}
	if rows[0].AssigneeAgentProfileID != "pg-parent-1-agent" {
		t.Errorf("AssigneeAgentProfileID = %q, want %q", rows[0].AssigneeAgentProfileID, "pg-parent-1-agent")
	}
	if rows[0].ChildSetKey != wantKey {
		t.Errorf("ChildSetKey = %q, want %q", rows[0].ChildSetKey, wantKey)
	}
}

// TestPostgresListStuckParentsChildSetKeyMatchesGo pins the aggregate against
// GetChildSetKey, which builds the same key in Go. The two must agree byte for
// byte across engines: a receipt is recorded with the Go form and compared
// against the SQL form, so a separator or ordering difference would make every
// receipt look stale and wake each parent forever. Postgres does not guarantee
// aggregate input order without an explicit ORDER BY inside the aggregate,
// which is what makes this worth asserting on more than one child.
func TestPostgresListStuckParentsChildSetKeyMatchesGo(t *testing.T) {
	repo, ctx := newPostgresWakeRepo(t)

	// Insert the children out of id order so an unordered aggregate produces a
	// different string than GetChildSetKey's ORDER BY id read.
	seedPostgresTask(t, ctx, repo, "pg-parent-2", "pg-ws-2")
	execPostgres(t, ctx, repo,
		`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, "pg-parent-2")
	for _, childID := range []string{"pg-parent-2-child-c", "pg-parent-2-child-a", "pg-parent-2-child-b"} {
		seedPostgresTask(t, ctx, repo, childID, "pg-ws-2")
		execPostgres(t, ctx, repo,
			`UPDATE tasks SET parent_id = ?, state = 'COMPLETED' WHERE id = ?`, "pg-parent-2", childID)
	}
	seedPostgresRunner(t, ctx, repo, "pg-parent-2")

	goKey, err := repo.GetChildSetKey(ctx, "pg-parent-2")
	if err != nil {
		t.Fatalf("GetChildSetKey: %v", err)
	}

	rows, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ChildSetKey != goKey {
		t.Errorf("ChildSetKey = %q, want %q (GetChildSetKey)", rows[0].ChildSetKey, goKey)
	}
}

// TestPostgresListStuckParentsExcludesCoveredParent drives the two predicates
// that the remaining dialect-sensitive constructs sit inside: the receipt
// comparison (IS DISTINCT FROM) and the in-flight-run check (JSON extraction).
// A parent whose receipt already matches its child set, and which has a queued
// wake run, must not come back.
func TestPostgresListStuckParentsExcludesCoveredParent(t *testing.T) {
	repo, ctx := newPostgresWakeRepo(t)

	key := seedPostgresStuckParent(t, ctx, repo, "pg-parent-3", "pg-ws-3")
	execPostgres(t, ctx, repo, `
		INSERT INTO parent_child_wake_receipts (parent_task_id, child_set_key, delivery_operation_id, delivered_at)
		VALUES (?, ?, 'op-1', ?)
	`, "pg-parent-3", key, time.Now().UTC())

	rows, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("len(rows) = %d, want 0 — receipt already covers this child set", len(rows))
	}

	// A queued wake run must also exclude the parent once the receipt no longer
	// matches, which is the json_extract/->> arm.
	execPostgres(t, ctx, repo,
		`UPDATE parent_child_wake_receipts SET child_set_key = 'stale' WHERE parent_task_id = ?`, "pg-parent-3")
	execPostgres(t, ctx, repo, `
		INSERT INTO runs (id, agent_profile_id, reason, payload, status, requested_at)
		VALUES ('pg-run-3', 'agent-x', 'task_children_completed', ?, 'queued', ?)
	`, `{"task_id":"pg-parent-3"}`, time.Now().UTC())

	rows, err = repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (queued run): %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("len(rows) = %d, want 0 — a queued wake run already covers this parent", len(rows))
	}
}

// newPostgresWakeRepo opens an isolated Postgres schema and initialises the
// settings, task, and workflow repositories before the office one, mirroring
// production boot order: agent_profiles belongs to the settings store schema,
// tasks and runs belong to the task repository's schema init, and
// workflow_step_participants.created_at (which RunnerProjection's third
// COALESCE arm orders by) is added by the workflow repository's migration —
// see participant_claim_postgres_test.go's newPostgresWakeRepo-equivalent
// setup for the same ordering contract.
func newPostgresWakeRepo(t *testing.T) (*sqlite.Repository, context.Context) {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
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
	return repo, context.Background()
}

// seedPostgresStuckParent builds the minimal parent that satisfies every
// ListStuckParents predicate — an Office-owned parent, one non-archived
// COMPLETED child, and a runner resolvable through an agent_profiles row —
// and returns the child_set_key the query should compute for it.
func seedPostgresStuckParent(
	t *testing.T, ctx context.Context, repo *sqlite.Repository, parentID, wsID string,
) string {
	t.Helper()
	seedPostgresTask(t, ctx, repo, parentID, wsID)
	execPostgres(t, ctx, repo,
		`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID)

	childID := parentID + "-child-0"
	seedPostgresTask(t, ctx, repo, childID, wsID)
	execPostgres(t, ctx, repo,
		`UPDATE tasks SET parent_id = ?, state = 'COMPLETED' WHERE id = ?`, parentID, childID)

	seedPostgresRunner(t, ctx, repo, parentID)
	return childID + ":COMPLETED"
}

// seedPostgresRunner inserts the backing agents row, the agent_profiles row,
// and the workflow_step_participants runner row that RunnerProjection
// resolves through. The agents row is required because Postgres enforces
// agent_profiles.agent_id's foreign key, unlike the SQLite test harness's
// default connection.
func seedPostgresRunner(t *testing.T, ctx context.Context, repo *sqlite.Repository, parentID string) {
	t.Helper()
	agentID := parentID + "-agent"
	now := time.Now().UTC()
	execPostgres(t, ctx, repo, `
		INSERT INTO agents (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, agentID, agentID, now, now)
	execPostgres(t, ctx, repo, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'idle', ?, ?)
	`, agentID, agentID, agentID, agentID, now, now)
	execPostgres(t, ctx, repo, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES (?, '', ?, 'runner', ?)
	`, "p-runner-"+parentID, parentID, agentID)
}

// seedPostgresTask inserts a tasks row with bound timestamps, since the SQLite
// helpers in this package use datetime('now').
func seedPostgresTask(t *testing.T, ctx context.Context, repo *sqlite.Repository, id, wsID string) {
	t.Helper()
	now := time.Now().UTC()
	execPostgres(t, ctx, repo, `
		INSERT INTO tasks (id, workspace_id, title, description, identifier, created_at, updated_at)
		VALUES (?, ?, 'Task', '', '', ?, ?)
	`, id, wsID, now, now)
}

func execPostgres(
	t *testing.T, ctx context.Context, repo *sqlite.Repository, query string, args ...any,
) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
