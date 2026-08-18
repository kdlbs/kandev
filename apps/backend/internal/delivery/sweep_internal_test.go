package delivery

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// newInternalTestRepo is a package-internal twin of the external test
// suite's newTestRepo (store_test.go, package delivery_test), needed here
// solely because evaluatePair is unexported and the soft-delete race this
// file covers must land its DB write between the due-selection read and
// evaluatePair's own RepositoryInfo re-read — a sequencing no black-box
// (RunPass-only) test can express.
func newInternalTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "delivery-ledger.db")
	rawConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(rawConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := taskrepo.Provide(db, db, nil); err != nil {
		t.Fatalf("init task schema: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init delivery schema: %v", err)
	}
	return repo, db
}

func seedInternalWorkspace(t *testing.T, db *sqlx.DB, id string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), id, id, now, now); err != nil {
		t.Fatalf("seed workspace %s: %v", id, err)
	}
}

func seedInternalRepository(t *testing.T, db *sqlx.DB, id, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO repositories (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), id, workspaceID, id, now, now); err != nil {
		t.Fatalf("seed repository %s: %v", id, err)
	}
}

func seedInternalTask(t *testing.T, db *sqlx.DB, id, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), id, workspaceID, id, now, now); err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

func countInternalLedgerRows(t *testing.T, db *sqlx.DB, taskID, repositoryID string) int {
	t.Helper()
	var n int
	if err := db.QueryRowxContext(context.Background(), db.Rebind(`
		SELECT COUNT(*) FROM task_delivery_ledger WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID).Scan(&n); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return n
}

// TestEvaluatePair_RepositorySoftDeletedAfterDueSelectionIsNotWritten covers
// the freeze guarantee under spec "Sweep selection predicate": "A pair
// whose repository has a non-NULL repositories.deleted_at is never due,
// whichever condition below would otherwise select it." SelectDuePairs
// enforces that at due-selection time, but evaluatePair re-reads
// RepositoryInfo independently and, before this fix, never rechecked
// DeletedAt — so a repository soft-deleted in the window between
// due-selection and evaluation (a real race within a single sweep pass,
// since the two reads are not in the same transaction) would still be
// classified and written, silently voiding the freeze (Review round 1,
// finding #5). This test simulates that exact window: the pair is
// selected as due first, the repository is soft-deleted second, and only
// then is the pair evaluated.
func TestEvaluatePair_RepositorySoftDeletedAfterDueSelectionIsNotWritten(t *testing.T) {
	repo, db := newInternalTestRepo(t)
	seedInternalWorkspace(t, db, "ws-1")
	seedInternalRepository(t, db, "repo-1", "ws-1")
	seedInternalTask(t, db, "task-1", "ws-1")

	pair := CandidatePair{TaskID: "task-1", RepositoryID: "repo-1"}

	// Due-selection happens while the repository is still live.
	due, _, _ := repo.SelectDuePairs(context.Background(), []CandidatePair{pair}, time.Now().UTC())
	if len(due) != 1 {
		t.Fatalf("due = %+v, want the one live candidate", due)
	}

	// The repository is soft-deleted in the window between due-selection
	// and evaluation.
	if _, err := db.Exec(db.Rebind(`UPDATE repositories SET deleted_at = ? WHERE id = ?`),
		time.Now().UTC(), "repo-1"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	sweep := NewSweep(repo, nil, nil)
	if err := sweep.evaluatePair(context.Background(), due[0]); err != nil {
		t.Fatalf("evaluatePair: %v", err)
	}

	if n := countInternalLedgerRows(t, db, "task-1", "repo-1"); n != 0 {
		t.Fatalf("ledger rows = %d, want 0 (a repository soft-deleted after due-selection must never be written)", n)
	}
}

// TestIsDue_PerPairTaskInfoQueryErrorFailsOpen covers the inconsistency
// flagged in Review round 1, finding #3: SelectDuePairs's own bulk-ledger-
// read failure fails OPEN (every non-frozen candidate is treated as due,
// "When the ledger itself cannot be read"), but isDue's per-pair TaskInfo /
// mostRecentInputObservation queries — reading exactly the same input
// tables (task_session_git_snapshots, task_sessions, task_repositories,
// the provider tables) that spec "Failure modes" names as "an input query
// fails" — used to fail CLOSED (silently not due, no counter, no log) on a
// query error, silently skipping a pair already carrying a ledger row.
// isDue now fails open on those errors too, so the pair is selected as due
// and the same underlying failure is discovered and counted by
// evaluatePair's own input-query handling (delivery_ledger_evaluation_errors_total),
// rather than being silently absorbed one layer up with no observability at
// all.
func TestIsDue_PerPairTaskInfoQueryErrorFailsOpen(t *testing.T) {
	repo, db := newInternalTestRepo(t)
	seedInternalWorkspace(t, db, "ws-1")
	seedInternalRepository(t, db, "repo-1", "ws-1")
	seedInternalTask(t, db, "task-1", "ws-1")

	// Seed an existing ledger row so isDue reaches its TaskInfo call
	// (the "no ledger row" branch returns true before ever touching
	// TaskInfo, and would not exercise this fix).
	if _, err := repo.Upsert(context.Background(), UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: Classification{Outcome: OutcomeUnknown, Basis: BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed ledger row: %v", err)
	}

	// Force a genuine query error in isDue's TaskInfo read without
	// dropping the tasks table outright: task_delivery_ledger.task_id is
	// ON DELETE CASCADE to tasks(id), and SQLite's DROP TABLE cascades
	// that FK, which would delete the ledger row just seeded above and
	// make the pair due via the "no ledger row" branch instead of
	// exercising this fix. Renaming a column away breaks the query
	// (TaskInfo selects workspace_id by name) while leaving the table and
	// row, and the ledger row, intact.
	if _, err := db.Exec(`ALTER TABLE tasks RENAME COLUMN workspace_id TO workspace_id_renamed`); err != nil {
		t.Fatalf("rename tasks.workspace_id: %v", err)
	}

	due, fallback, _ := repo.SelectDuePairs(context.Background(),
		[]CandidatePair{{TaskID: "task-1", RepositoryID: "repo-1"}}, time.Now().UTC())
	if fallback {
		t.Fatal("fallback must be false: the bulk ledger read itself succeeded")
	}
	if len(due) != 1 {
		t.Fatalf("due = %+v, want the pair selected as due (fail open on a per-pair query error)", due)
	}
}

// TestPublishWriterHealth_LogsStalledWarnWhenThresholdExceeded is the
// firing-state half of Review round 1, finding #4: every
// TestComputeStallSignal_* case in the external suite keeps StallSeconds
// at 0, and none of them drives Sweep.publishWriterHealth's own threshold
// comparison (sweep.go), which is where the "delivery_ledger.stalled" warn
// actually lives (ComputeStallSignal only computes the number). Calling
// publishWriterHealth directly — rather than through RunPass — sidesteps
// an unrelated interaction: any session on a resolvable (task, repository)
// pair becomes a Candidate, and a pair with no ledger row is
// unconditionally due, so RunPass would immediately re-evaluate and
// overwrite the very last_evaluated_at gap this test needs to keep
// intact. Testing this unexported method from the same package is exactly
// what evaluatePair's own soft-delete race test above does for the same
// reason.
func TestPublishWriterHealth_LogsStalledWarnWhenThresholdExceeded(t *testing.T) {
	repo, db := newInternalTestRepo(t)
	seedInternalWorkspace(t, db, "ws-1")
	seedInternalRepository(t, db, "repo-1", "ws-1")
	seedInternalTask(t, db, "task-1", "ws-1")
	if _, err := repo.Upsert(context.Background(), UpsertInput{
		TaskID: "task-1", RepositoryID: "repo-1", WorkspaceID: "ws-1",
		Classification: Classification{Outcome: OutcomeUnknown, Basis: BasisNoObservations, Rank: 2},
		EvaluatedAt:    time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed ledger row: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, repository_id, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "sess-live", "task-1", "repo-1", now, now); err != nil {
		t.Fatalf("seed task_session: %v", err)
	}

	core, observed := observer.New(zapcore.WarnLevel)
	testLogger, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("build logger: %v", err)
	}

	sweep := NewSweep(repo, nil, testLogger)
	sweep.publishWriterHealth(context.Background())

	entries := observed.FilterMessage("delivery_ledger.stalled").All()
	if len(entries) != 1 {
		t.Fatalf("got %d %q warnings, want 1 (all warnings: %+v)", len(entries), "delivery_ledger.stalled", observed.All())
	}
	fields := entries[0].ContextMap()
	stallSeconds, ok := fields["stall_seconds"].(int64)
	if !ok || stallSeconds <= int64(StallThreshold.Seconds()) {
		t.Fatalf("stall_seconds field = %v, want an int64 > %d", fields["stall_seconds"], int64(StallThreshold.Seconds()))
	}
	if _, ok := fields["last_evaluated_unix"].(int64); !ok {
		t.Fatalf("last_evaluated_unix field = %v, want an int64", fields["last_evaluated_unix"])
	}
}
