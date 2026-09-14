package sqlite

// Internal (white-box) test file: commitClaim is unexported, so exercising
// its own compare-and-swap guard directly needs package-level access, unlike
// every other test in this directory (package sqlite_test).

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/models"
)

// commitClaimTestSchema declares only the two tables commitClaim
// touches (runs, office_launch_ledger). This package's own NewWithDB
// deliberately does not create them (schema ownership belongs to the
// office repository's initSchema), and this file cannot import that
// package: package sqlite's non-test code is already imported by it, so
// an internal (white-box) test file importing it back is a cycle Go
// test builds reject, unlike an external sqlite_test file. Kept in sync
// with internal/office/repository/sqlite/base.go's createRunTables and
// base_migrations.go's office_launch_ledger DDL by
// TestCommitClaim_RowChangedAwayFromQueuedIsNotSilentlyClaimed exercising
// every column commitClaim's INSERT writes.
const commitClaimTestSchema = `
CREATE TABLE runs (
	id TEXT PRIMARY KEY,
	agent_profile_id TEXT NOT NULL,
	reason TEXT NOT NULL,
	payload TEXT DEFAULT '{}',
	status TEXT NOT NULL DEFAULT 'queued',
	coalesced_count INTEGER DEFAULT 1,
	idempotency_key TEXT,
	context_snapshot TEXT DEFAULT '{}',
	capabilities TEXT NOT NULL DEFAULT '{}',
	input_snapshot TEXT NOT NULL DEFAULT '{}',
	output_summary TEXT NOT NULL DEFAULT '',
	failure_reason TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	retry_count INTEGER DEFAULT 0,
	scheduled_retry_at TIMESTAMP,
	error_message TEXT NOT NULL DEFAULT '',
	cancel_reason TEXT,
	outcome TEXT,
	logical_provider_order TEXT,
	requested_tier TEXT,
	resolved_execution_profile_id TEXT,
	resolved_provider_id TEXT,
	resolved_model TEXT,
	current_route_attempt_seq INTEGER NOT NULL DEFAULT 0,
	routing_blocked_status TEXT,
	earliest_retry_at TIMESTAMP,
	route_cycle_baseline_seq INTEGER NOT NULL DEFAULT 0,
	result_json TEXT NOT NULL DEFAULT '{}',
	assembled_prompt TEXT NOT NULL DEFAULT '',
	summary_injected TEXT NOT NULL DEFAULT '',
	continuation_scope TEXT NOT NULL DEFAULT '',
	requested_at TIMESTAMP NOT NULL,
	claimed_at TIMESTAMP,
	finished_at TIMESTAMP,
	causation_id TEXT NOT NULL DEFAULT '',
	parent_run_id TEXT NOT NULL DEFAULT '',
	causation_depth INTEGER NOT NULL DEFAULT 0,
	priority_class INTEGER NOT NULL DEFAULT 2,
	human_rooted INTEGER NOT NULL DEFAULT 0,
	routine_id TEXT NOT NULL DEFAULT '',
	actor_kind TEXT NOT NULL DEFAULT 'system',
	actor_id TEXT NOT NULL DEFAULT '',
	workspace_id TEXT NOT NULL DEFAULT ''
);
CREATE TABLE office_launch_ledger (
	id           TEXT      PRIMARY KEY,
	run_id       TEXT      NOT NULL,
	workspace_id TEXT      NOT NULL,
	causation_id TEXT      NOT NULL DEFAULT '',
	routine_id   TEXT      NOT NULL DEFAULT '',
	human_rooted INTEGER   NOT NULL DEFAULT 0,
	claimed_at   TIMESTAMP NOT NULL
);
`

// newCommitClaimTestRepo builds a runs repository directly over
// commitClaimTestSchema (see its doc comment for why this can't reuse
// the office repository's initSchema here).
func newCommitClaimTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "commit-claim.db")
	writerRaw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	writer := sqlx.NewDb(writerRaw, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Exec(commitClaimTestSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return NewWithDB(writer, writer)
}

// TestCommitClaim_RowChangedAwayFromQueuedIsNotSilentlyClaimed pins the
// claim-vs-cancel race: on Postgres's default READ COMMITTED isolation, a
// concurrent cancel can commit between the candidate scan's SELECT and
// this transaction's claiming UPDATE, so the in-memory candidate can be
// stale by the time commitClaim runs. This test reproduces that end
// condition deterministically (the row's real status no longer matches
// what the in-memory candidate says) without needing real concurrent
// connections: commitClaim's UPDATE must be a compare-and-swap on
// status='queued', not a blind write, or it silently resurrects a
// cancelled run as claimed.
func TestCommitClaim_RowChangedAwayFromQueuedIsNotSilentlyClaimed(t *testing.T) {
	repo := newCommitClaimTestRepo(t)
	ctx := context.Background()

	run := &models.Run{
		ID:             "run-race-1",
		AgentProfileID: "agent-race-1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	// Simulate the concurrent cancel that committed after the candidate
	// scan read status='queued' but before this claim's UPDATE runs.
	if _, err := repo.db.Exec(`UPDATE runs SET status = 'cancelled' WHERE id = ?`, run.ID); err != nil {
		t.Fatalf("simulate concurrent cancel: %v", err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	candidate := *run
	candidate.Status = "queued" // the stale in-memory read scanCandidatePages produced

	claimed, claimErr := repo.commitClaim(ctx, tx, &candidate)
	// commitClaim commits on success; on failure the caller (ClaimNextEligibleRun)
	// would roll back, so this test does the same before reading the row back
	// on the same single-connection writer handle — otherwise the still-open
	// tx holds the only connection and the reads below deadlock.
	_ = tx.Rollback()
	if claimErr == nil && claimed != nil {
		t.Fatalf("commitClaim silently claimed a run concurrently changed away from queued: %+v", claimed)
	}

	var status string
	if err := repo.db.Get(&status, `SELECT status FROM runs WHERE id = ?`, run.ID); err != nil {
		t.Fatalf("read back status: %v", err)
	}
	if status != "cancelled" {
		t.Errorf("run status = %q, want cancelled (commitClaim must not overwrite a concurrent status change)", status)
	}

	var ledgerCount int
	if err := repo.db.Get(&ledgerCount, `SELECT COUNT(*) FROM office_launch_ledger WHERE run_id = ?`, run.ID); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerCount != 0 {
		t.Errorf("launch ledger rows = %d, want 0 (no claim actually happened)", ledgerCount)
	}
}
