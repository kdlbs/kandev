package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

// newPostgresSearchTestRepo is newSearchTestRepo's (tasks_test.go)
// PostgreSQL twin: the same minimal fixture schema, built against a real
// Postgres connection. CURRENT_TIMESTAMP replaces SQLite's datetime('now')
// default — both dialects accept the bare CURRENT_TIMESTAMP keyword.
//
// Unlike newSearchTestRepo, the fixture tables are created BEFORE
// sqlite.NewWithDB, not after: the office schema's runs table has a foreign
// key onto tasks (see failure_postgres_test.go), and Postgres enforces that
// the referenced table exist at CREATE TABLE time, unlike SQLite.
func newPostgresSearchTestRepo(t *testing.T, db *sqlx.DB) *sqlite.Repository {
	t.Helper()
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			office_workflow_id TEXT DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			workflow_step_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			state TEXT DEFAULT 'TODO',
			priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('critical','high','medium','low')),
			parent_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			labels TEXT DEFAULT '[]',
			identifier TEXT DEFAULT '',
			is_ephemeral INTEGER DEFAULT 0,
			origin TEXT DEFAULT 'manual',
			metadata TEXT DEFAULT '{}',
			checkout_agent_id TEXT,
			checkout_at TIMESTAMP,
			checkout_run_id TEXT,
			archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			agent_profile_id TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create workflow_steps table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_step_participants (
			id TEXT PRIMARY KEY,
			step_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			agent_profile_id TEXT NOT NULL DEFAULT '',
			decision_required INTEGER NOT NULL DEFAULT 0,
			position INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
			provenance TEXT NOT NULL DEFAULT 'manual'
		)
	`); err != nil {
		t.Fatalf("create workflow_step_participants table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_step_decisions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL DEFAULT '',
			step_id TEXT NOT NULL DEFAULT '',
			participant_id TEXT NOT NULL DEFAULT '',
			decision TEXT NOT NULL DEFAULT '',
			decided_at TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00',
			superseded_at TIMESTAMP NULL
		)
	`); err != nil {
		t.Fatalf("create workflow_step_decisions table: %v", err)
	}

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	// agent_profiles.agent_id has a foreign key onto agents(id), enforced on
	// Postgres unlike SQLite. Every fixture agent profile in this file uses
	// the '' agent_id seedWakeAgentProfile's SQLite twin also uses, so seed
	// that one row once.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO agents (id, name, created_at, updated_at)
		VALUES ('', 'stub', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed stub agents row: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func insertPostgresTask(t *testing.T, repo *sqlite.Repository, ctx context.Context, id, wsID, title string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, id, wsID, title); err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

// seedPostgresWakeAgentProfile inserts an agent_profiles row backed by the
// empty-string agents row seeded once by newPostgresSearchTestRepo. Unlike SQLite (FK
// enforcement off by default), Postgres enforces agent_profiles' foreign key
// onto agents(id), so — unlike seedWakeAgentProfile's SQLite twin — the
// referenced agents row must actually exist first.
func seedPostgresWakeAgentProfile(t *testing.T, repo *sqlite.Repository, ctx context.Context, agentID, status string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, status, created_at, updated_at)
		VALUES (?, '', ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, agentID, agentID, agentID, status); err != nil {
		t.Fatalf("seed agent profile %s: %v", agentID, err)
	}
}

// seedPostgresRunnerForSearchRepo is newPostgresSearchTestRepo's runner
// fixture: it assumes the empty-string stub agents row that repo already
// seeded, and only adds the workflow_step_participants row RunnerProjection resolves
// through. Named distinctly from seedPostgresRunner below (newPostgresWakeRepo's
// fixture), which seeds its own dedicated agents row per parent instead of
// reusing the stub — the two fixtures are not interchangeable.
func seedPostgresRunnerForSearchRepo(t *testing.T, repo *sqlite.Repository, ctx context.Context, parentID string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id)
		VALUES (?, '', ?, 'runner', ?)
	`, "p-runner-"+parentID, parentID, parentID+"-agent"); err != nil {
		t.Fatalf("seed runner: %v", err)
	}
}

// TestPostgresWakeReceiptMigrations_ApplyAndReplay is the PostgreSQL twin
// required by apps/backend/AGENTS.md for a schema-changing repository
// change: this branch added parent_child_wake_receipts.child_generation
// (ALTER TABLE ... ADD COLUMN). Confirms it applies on a fresh database and
// is idempotent when the schema init runs again against the same database
// (mirroring production boot order). Skips unless KANDEV_TEST_POSTGRES_DSN
// is set.
func TestPostgresWakeReceiptMigrations_ApplyAndReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	// The office schema's runs table has a foreign key onto tasks, so the
	// tasks table must exist first — initialize via taskrepo, mirroring
	// production boot order (see workflow_test.go / failure_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("replay init office repo: %v", err)
	}

	ctx := context.Background()
	// Proves parent_child_wake_receipts.child_generation exists: this SELECT
	// names the column explicitly (GetWakeReceipt, wake_receipts.go).
	if _, err := repo.GetWakeReceipt(ctx, "pg-migration-parent"); err != nil {
		t.Fatalf("read wake receipt after replay: %v", err)
	}
}

// TestPostgresSecondPrecisionText_RenderedTextIsStableAndPortable proves the
// specific defect dialect.SecondPrecisionText fixes: without it,
// ListStuckParents' `newest_child_updated_at != child_generation` arm
// compares a native TIMESTAMP expression against parent_child_wake_receipts'
// TEXT column. SQLite accepts that comparison by implicit conversion;
// Postgres rejects it outright ("operator does not exist: timestamp without
// time zone <> text"), because Postgres has no implicit cast from a typed
// text column to timestamp. This seeds a real TIMESTAMP column written the
// same way tasks.updated_at is (bare CURRENT_TIMESTAMP), confirms the
// rendered text parses under the exact Go layout
// scheduler_wake_reconciler.go's childGenerationSecondLayout uses, and
// confirms two independent reads of the same underlying value render
// identical text — the property the generation equality check in
// ListStuckParents depends on. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresSecondPrecisionText_RenderedTextIsStableAndPortable(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE second_precision_probe (
			id TEXT PRIMARY KEY,
			ts TIMESTAMP NOT NULL
		)
	`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO second_precision_probe (id, ts) VALUES ('row-1', CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	query := "SELECT " + dialect.SecondPrecisionText(dialect.PGX, "ts") +
		" FROM second_precision_probe WHERE id = 'row-1'"

	var first, second string
	if err := db.GetContext(ctx, &first, query); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if err := db.GetContext(ctx, &second, query); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if first != second {
		t.Fatalf("two reads of the same row rendered different text: %q vs %q", first, second)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", first); err != nil {
		t.Fatalf("rendered text %q does not match childGenerationSecondLayout: %v", first, err)
	}
}

// TestPostgresListStuckParents_ReadmitsAfterChildReopenedAndRecompleted is
// the PostgreSQL twin of
// TestListStuckParents_ReadmitsAfterChildReopenedAndRecompleted
// (wake_receipts_test.go): the same production writers
// (UpdateTaskState/UpsertWakeReceiptTx), the same reopen-then-recomplete
// sequence, run against a real Postgres database end to end through
// ListStuckParents — not against child_generation in isolation — so a
// mistake anywhere in how the dialect-aware SQL was wired (not just in
// dialect.SecondPrecisionText itself) would show up here. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListStuckParents_ReadmitsAfterChildReopenedAndRecompleted(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo := newPostgresSearchTestRepo(t, db)
	ctx := context.Background()

	const (
		parentID = "pg-parent-1"
		wsID     = "pg-ws-1"
	)

	insertPostgresTask(t, repo, ctx, parentID, wsID, "Parent")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET project_id = 'office-project' WHERE id = ?`, parentID,
	); err != nil {
		t.Fatalf("mark parent as Office task: %v", err)
	}
	childID := parentID + "-child-0"
	insertPostgresTask(t, repo, ctx, childID, wsID, "Child")
	if _, err := repo.ExecRaw(ctx,
		`UPDATE tasks SET parent_id = ? WHERE id = ?`, parentID, childID,
	); err != nil {
		t.Fatalf("attach child to parent: %v", err)
	}
	seedPostgresWakeAgentProfile(t, repo, ctx, parentID+"-agent", "idle")
	seedPostgresRunnerForSearchRepo(t, repo, ctx, parentID)

	if err := repo.UpdateTaskState(ctx, childID, "COMPLETED"); err != nil {
		t.Fatalf("complete child: %v", err)
	}

	preDelivery, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (pre-delivery): %v", err)
	}
	if len(preDelivery) != 1 || preDelivery[0].ParentTaskID != parentID {
		t.Fatalf("ListStuckParents (pre-delivery) = %#v, want exactly [%s]", preDelivery, parentID)
	}

	// See waitForNextWholeSecond (wake_receipts_test.go): without a real gap,
	// the reopen below could land in the same wall-clock second as the
	// original completion, making the two generations indistinguishable by
	// test construction rather than by the fix under test.
	waitForNextWholeSecond(t)

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := repo.UpsertWakeReceiptTx(
		ctx, tx, parentID, preDelivery[0].ChildSetKey, "", "op-1",
		preDelivery[0].NewestChildUpdatedAt, time.Now().UTC(),
	); err != nil {
		t.Fatalf("upsert wake receipt: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit receipt tx: %v", err)
	}

	afterDelivery, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (after delivery): %v", err)
	}
	if len(afterDelivery) != 0 {
		t.Fatalf("after delivery: candidates = %#v, want none (receipt already covers this child set)", afterDelivery)
	}

	if err := repo.UpdateTaskState(ctx, childID, "IN_PROGRESS"); err != nil {
		t.Fatalf("reopen child: %v", err)
	}
	if err := repo.UpdateTaskState(ctx, childID, "COMPLETED"); err != nil {
		t.Fatalf("recomplete child: %v", err)
	}

	after, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (after reopen): %v", err)
	}
	if len(after) != 1 || after[0].ParentTaskID != parentID {
		t.Fatalf("PostgreSQL: reopen+recomplete not recoverable: after = %#v, want exactly [%s]", after, parentID)
	}
}

// TestPostgresListStuckParents is the PostgreSQL twin of the SQLite
// ListStuckParents suite. The query carried several SQLite-only constructs —
// GROUP_CONCAT, json_extract(...), and IS NOT with a column right-hand side
// are the three fixed here; a fourth, wsp.rowid, was already fixed in #3289 —
// each a parse error on Postgres, so ParentWakeReconciler's backing query
// could not run there at all and the reconciler recovered nothing. Postgres
// rejects the whole statement at parse time, so this fails on the first
// construct rather than returning wrong rows.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListStuckParents(t *testing.T) {
	repo, ctx := newPostgresWakeRepo(t)

	wantKey := seedPostgresStuckParent(t, ctx, repo, "pg-parent-2a", "pg-ws-2a")

	rows, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ParentTaskID != "pg-parent-2a" {
		t.Errorf("ParentTaskID = %q, want %q", rows[0].ParentTaskID, "pg-parent-2a")
	}
	if rows[0].AssigneeAgentProfileID != "pg-parent-2a-agent" {
		t.Errorf("AssigneeAgentProfileID = %q, want %q", rows[0].AssigneeAgentProfileID, "pg-parent-2a-agent")
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

// TestPostgresListStuckParentsExcludesCoveredParent drives the three
// predicates the receipt-coverage OR sits inside: the receipt comparison
// (IS DISTINCT FROM), the child-generation comparison (this branch's third
// arm), and the in-flight-run check (JSON extraction). A parent whose
// receipt already matches its child set and generation, and which has a
// queued wake run, must not come back.
func TestPostgresListStuckParentsExcludesCoveredParent(t *testing.T) {
	repo, ctx := newPostgresWakeRepo(t)

	key := seedPostgresStuckParent(t, ctx, repo, "pg-parent-3", "pg-ws-3")

	// Read the candidate's generation before recording the receipt: the
	// generation arm compares against this value, so a receipt recorded
	// without it (e.g. a hand-written INSERT defaulting child_generation to
	// '') would never match and the parent would never be excluded.
	preReceipt, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents (pre-receipt): %v", err)
	}
	if len(preReceipt) != 1 || preReceipt[0].ParentTaskID != "pg-parent-3" {
		t.Fatalf("ListStuckParents (pre-receipt) = %#v, want exactly [pg-parent-3]", preReceipt)
	}
	execPostgres(t, ctx, repo, `
		INSERT INTO parent_child_wake_receipts (parent_task_id, child_set_key, delivery_operation_id, child_generation, delivered_at)
		VALUES (?, ?, 'op-1', ?, ?)
	`, "pg-parent-3", key, preReceipt[0].NewestChildUpdatedAt, time.Now().UTC())

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

// newPostgresWakeRepo opens an isolated Postgres schema and initializes the
// settings, task, and workflow repositories before the office one. These
// repositories provide the schema dependencies that ListStuckParents reads:
// agent_profiles, tasks, runs, and workflow_step_participants.created_at.
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

// TestPostgresListStuckParents_ThirdTierOrdersByCreatedAtNotID is the
// PostgreSQL twin of TestListStuckParents_ThirdTierOrdersByCreatedAtNotID:
// RunnerProjection's third fallback tier — the only line base.go
// behaviourally changed — must pick the same agent on both engines by
// ordering on workflow_step_participants.created_at, not a physical or
// textual row identifier. created_at and id are set in opposite order so
// a pick driven by id (the prior Postgres ordering) returns the wrong
// agent.
func TestPostgresListStuckParents_ThirdTierOrdersByCreatedAtNotID(t *testing.T) {
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
	const parentID = "pg-tier3-parent"
	const recentAgentID, oldAgentID = "pg-tier3-agent-recent", "pg-tier3-agent-old"
	childID := parentID + "-child-0"

	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, project_id, workflow_step_id, created_at, updated_at)
		VALUES (?, 'ws-1', 'Parent', 'office-project', 'step-parent', ?, ?)
	`), parentID, now, now); err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, parent_id, state, created_at, updated_at)
		VALUES (?, 'ws-1', 'Child', ?, 'COMPLETED', ?, ?)
	`), childID, parentID, now, now); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	for _, agentID := range []string{recentAgentID, oldAgentID} {
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`), agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent %s: %v", agentID, err)
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`
			INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'ws-1', '', 'idle', ?, ?)
		`), agentID, agentID, agentID, agentID, now, now); err != nil {
			t.Fatalf("seed agent profile %s: %v", agentID, err)
		}
	}
	// Neither participant is at the parent's current step ('step-parent'),
	// so tier 1 falls through for both; tier 3 must pick by created_at, and
	// id is set in the opposite order to prove it isn't the deciding column.
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO workflow_step_participants (id, step_id, task_id, role, agent_profile_id, created_at)
		VALUES
			('aa-pg-tier3-1', 'other-step-1', ?, 'runner', ?, ?),
			('zz-pg-tier3-2', 'other-step-2', ?, 'runner', ?, ?)
	`), parentID, recentAgentID, now, parentID, oldAgentID, now.Add(-24*time.Hour)); err != nil {
		t.Fatalf("seed tier-3 runners: %v", err)
	}

	candidates, err := repo.ListStuckParents(ctx, "task_children_completed", 5)
	if err != nil {
		t.Fatalf("ListStuckParents: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ParentTaskID != parentID {
		t.Fatalf("candidates = %#v, want exactly [%s]", candidates, parentID)
	}
	if candidates[0].AssigneeAgentProfileID != recentAgentID {
		t.Fatalf("assignee = %q, want %q (latest created_at, not largest id)", candidates[0].AssigneeAgentProfileID, recentAgentID)
	}
}
