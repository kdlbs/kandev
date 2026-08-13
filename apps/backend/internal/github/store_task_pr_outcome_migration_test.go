package github

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
)

// preOutcomeGithubTaskPRsDDL is a snapshot of the github_task_prs DDL as it
// existed before the eight outcome-attribution columns were added. Used to
// simulate a pre-migration database.
const preOutcomeGithubTaskPRsDDL = `
	CREATE TABLE github_task_prs (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		pr_url TEXT NOT NULL,
		pr_title TEXT NOT NULL,
		head_branch TEXT NOT NULL,
		base_branch TEXT NOT NULL,
		author_login TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'open',
		review_state TEXT NOT NULL DEFAULT '',
		checks_state TEXT NOT NULL DEFAULT '',
		mergeable_state TEXT NOT NULL DEFAULT '',
		review_count INTEGER DEFAULT 0,
		pending_review_count INTEGER DEFAULT 0,
		required_reviews INTEGER,
		comment_count INTEGER DEFAULT 0,
		unresolved_review_threads INTEGER DEFAULT 0,
		checks_total INTEGER DEFAULT 0,
		checks_passing INTEGER DEFAULT 0,
		additions INTEGER DEFAULT 0,
		deletions INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		merged_at DATETIME,
		closed_at DATETIME,
		last_synced_at DATETIME,
		detached_at DATETIME,
		updated_at DATETIME NOT NULL,
		UNIQUE(task_id, repository_id, pr_number)
	)`

const outcomeMetaKey = "github_task_pr_outcome_activated_at"

// newPreOutcomeMigrationStore opens a fresh SQLite DB seeded with the
// pre-migration shape of github_task_prs (missing all eight outcome columns)
// plus the tasks/workspaces tables NewStore expects, and returns the raw
// handle for pre-migration seeding before the caller drives schema init via
// NewStore.
func newPreOutcomeMigrationStore(t *testing.T) (*sqlx.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "github-outcome-migration.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, archived_at DATETIME)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := db.Exec(preOutcomeGithubTaskPRsDDL); err != nil {
		t.Fatalf("create pre-migration github_task_prs: %v", err)
	}
	return db, dbPath
}

// TestTaskPROutcomeMigration_AddsColumnsNoBackfillActivatesOnce covers AC-01
// through AC-06: an existing database lacking the eight outcome columns gets
// them via ADD COLUMN with no NOT NULL/DEFAULT, a pre-existing terminal row
// stays NULL in all eight after two startups, the activation instant is
// written exactly once in RFC 3339 form, and a second startup does not
// advance it.
func TestTaskPROutcomeMigration_AddsColumnsNoBackfillActivatesOnce(t *testing.T) {
	db, _ := newPreOutcomeMigrationStore(t)

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id) VALUES (?, ?)
	`), "task-pre-existing", "ws-outcome-migration"); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO github_task_prs (
			id, workspace_id, task_id, owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, state, created_at, closed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "pr-pre-existing", "ws-outcome-migration", "task-pre-existing", "kdlbs", "kandev",
		2476, "https://github.com/kdlbs/kandev/pull/2476", "pre-existing PR",
		"feature/x", "main", "nova28", "closed", now, now, now); err != nil {
		t.Fatalf("seed pre-existing terminal github_task_prs row: %v", err)
	}

	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("first NewStore (migration): %v", err)
	}

	assertOutcomeColumnsNull(t, db, "pr-pre-existing")
	firstActivation := requireOutcomeActivationOnce(t, db)

	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("second NewStore (replay): %v", err)
	}

	assertOutcomeColumnsNull(t, db, "pr-pre-existing")
	secondActivation := requireOutcomeActivationOnce(t, db)
	if secondActivation != firstActivation {
		t.Fatalf("activation instant changed across replay: first=%q second=%q", firstActivation, secondActivation)
	}
}

// TestTaskPROutcomeMigration_FreshInstallReplaysClean covers the ADR-0027
// fresh-plus-replay requirement: a brand-new database (columns already in
// createTablesSQL) initializes once and replays cleanly a second time,
// still stamping the activation instant exactly once.
func TestTaskPROutcomeMigration_FreshInstallReplaysClean(t *testing.T) {
	store := newTestStore(t)

	first := requireOutcomeActivationOnce(t, store.db)

	if _, err := NewStore(store.db, store.db); err != nil {
		t.Fatalf("replay NewStore on fresh install: %v", err)
	}
	second := requireOutcomeActivationOnce(t, store.db)
	if second != first {
		t.Fatalf("activation instant changed across fresh-install replay: first=%q second=%q", first, second)
	}
}

func assertOutcomeColumnsNull(t *testing.T, db *sqlx.DB, id string) {
	t.Helper()
	var (
		isDraft, changedFiles, mergedByLogin, closedByLogin, autoMergeObservedAt any
		disposition, dispositionURL, dispositionRecordedAt                       any
	)
	err := db.QueryRow(db.Rebind(`
		SELECT is_draft, changed_files, merged_by_login, closed_by_login, auto_merge_observed_at,
			disposition, disposition_superseded_by_url, disposition_recorded_at
		FROM github_task_prs WHERE id = ?
	`), id).Scan(
		&isDraft, &changedFiles, &mergedByLogin, &closedByLogin, &autoMergeObservedAt,
		&disposition, &dispositionURL, &dispositionRecordedAt,
	)
	if err != nil {
		t.Fatalf("scan outcome columns for %q: %v", id, err)
	}
	for name, val := range map[string]any{
		"is_draft":                      isDraft,
		"changed_files":                 changedFiles,
		"merged_by_login":               mergedByLogin,
		"closed_by_login":               closedByLogin,
		"auto_merge_observed_at":        autoMergeObservedAt,
		"disposition":                   disposition,
		"disposition_superseded_by_url": dispositionURL,
		"disposition_recorded_at":       dispositionRecordedAt,
	} {
		if val != nil {
			t.Errorf("%s on %q = %v, want NULL", name, id, val)
		}
	}
}

func requireOutcomeActivationOnce(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	val, err := persistence.ReadMetaKey(db, outcomeMetaKey)
	if err != nil {
		t.Fatalf("ReadMetaKey(%s): %v", outcomeMetaKey, err)
	}
	if val == "" {
		t.Fatalf("%s not set after migration", outcomeMetaKey)
	}
	if _, err := time.Parse(time.RFC3339, val); err != nil {
		t.Fatalf("%s = %q is not RFC 3339: %v", outcomeMetaKey, val, err)
	}
	return val
}

// TestAddTaskPROutcomeColumns_NonDuplicateErrorAbortsRatherThanSwallows is
// the regression for AC-03's fail-loud branch, which the migration replay
// tests above never exercise (they only ever see the tolerated
// duplicate-column case). Deliberately omitting github_task_prs entirely
// makes tableColumns' PRAGMA table_info read return zero rows without an
// error (SQLite does not error on a missing table there), so
// addTaskPROutcomeColumns proceeds to `ALTER TABLE github_task_prs ADD
// COLUMN ...`, which fails with "no such table" — a genuine non-duplicate
// error distinct from the one dbutil.IsDuplicateColumnError tolerates.
func TestAddTaskPROutcomeColumns_NonDuplicateErrorAbortsRatherThanSwallows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "no-github-task-prs-table.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	db := sqlx.NewDb(dbConn, "sqlite3")

	store := &Store{db: db, ro: db}
	err = store.addTaskPROutcomeColumns()
	if err == nil {
		t.Fatal("addTaskPROutcomeColumns: want error against a database missing github_task_prs, got nil")
	}
	if dbutil.IsDuplicateColumnError(err) {
		t.Fatalf("addTaskPROutcomeColumns misclassified a missing-table error as duplicate-column: %v", err)
	}
}

// TestActivateTaskPROutcomeTracking_WriteFailureAbortsRatherThanSwallows is
// the companion regression for the activation-instant write (spec: Failure
// modes — "kandev_meta write fails during activation: Startup aborts").
// Pre-creating kandev_meta with a key column that carries no UNIQUE/PRIMARY
// KEY constraint leaves EnsureMetaTable's CREATE TABLE IF NOT EXISTS a
// no-op (the table already exists), so WriteMetaKeyIfAbsent's `INSERT ...
// ON CONFLICT(key) DO NOTHING` fails outright — SQLite requires the
// conflict target to match a real constraint — which is a genuine,
// non-swallowed write failure.
func TestActivateTaskPROutcomeTracking_WriteFailureAbortsRatherThanSwallows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "malformed-kandev-meta.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })
	db := sqlx.NewDb(dbConn, "sqlite3")
	if _, err := db.Exec(`CREATE TABLE kandev_meta (key TEXT)`); err != nil {
		t.Fatalf("seed malformed kandev_meta: %v", err)
	}

	store := &Store{db: db, ro: db}
	if err := store.activateTaskPROutcomeTracking(); err == nil {
		t.Fatal("activateTaskPROutcomeTracking: want error against a malformed kandev_meta table, got nil")
	}
}
