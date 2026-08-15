package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestCostEventTxAtomicity_RealReposShareTransaction is the PR #2606 review
// round 1 follow-up (F2): the only prior coverage of
// recordCostEventAndRollup's shared-transaction path
// (event_subscribers.go) used a test double that discarded its *sqlx.Tx
// parameter, so neither session.go's `tx != nil` branch nor
// CreateCostEventTx executing against a real transaction was ever exercised.
//
// This builds the real office and task repositories over one *sqlx.DB (the
// NewWithDB(db, db, nil) pattern already used by
// cost_event_contract_migration_test.go and workflow_test.go — safe because
// office.Repository.BeginTx and task.Repository.IncrementTaskSessionUsageTx
// are constructed from the same underlying writer connection in production,
// see BeginTx's doc comment), then drives BeginTx -> CreateCostEventTx ->
// IncrementTaskSessionUsageTx -> Rollback and asserts both office_cost_events
// and task_sessions are unchanged, followed by a committed retry that
// updates both.
func TestCostEventTxAtomicity_RealReposShareTransaction(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	taskRepo, err := taskrepo.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	officeRepo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-atomic', 'ws', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES ('task-atomic', 'ws-atomic', 't', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, started_at, updated_at) VALUES ('session-atomic', 'task-atomic', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	countCostEvents := func() int {
		t.Helper()
		var n int
		if err := db.GetContext(ctx, &n, `SELECT COUNT(*) FROM office_cost_events`); err != nil {
			t.Fatalf("count cost events: %v", err)
		}
		return n
	}
	rollupTotals := func() (tokensIn, tokensCachedIn, tokensOut, costSubcents int64) {
		t.Helper()
		row := db.QueryRowContext(ctx, `
			SELECT tokens_in, tokens_cached_in, tokens_out, cost_subcents
			  FROM task_sessions WHERE id = 'session-atomic'
		`)
		if err := row.Scan(&tokensIn, &tokensCachedIn, &tokensOut, &costSubcents); err != nil {
			t.Fatalf("read rollup totals: %v", err)
		}
		return tokensIn, tokensCachedIn, tokensOut, costSubcents
	}
	buildEvent := func(usageEventID string) *models.CostEvent {
		return &models.CostEvent{
			SessionID:    "session-atomic",
			TaskID:       "task-atomic",
			CostSubcents: 500,
			UsageEventID: &usageEventID,
			OccurredAt:   now,
		}
	}

	// Rollback: both writes land in the same transaction; rolling it back
	// must undo both, not just the one whose caller happened to fail.
	tx, err := officeRepo.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := officeRepo.CreateCostEventTx(ctx, tx, buildEvent("usage-evt-rollback")); err != nil {
		t.Fatalf("create cost event in tx: %v", err)
	}
	if err := taskRepo.IncrementTaskSessionUsageTx(ctx, tx, "session-atomic", 100, 40, 50, 500); err != nil {
		t.Fatalf("increment rollup in tx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if n := countCostEvents(); n != 0 {
		t.Fatalf("cost events after rollback = %d, want 0", n)
	}
	if tokensIn, tokensCachedIn, tokensOut, costSubcents := rollupTotals(); tokensIn != 0 || tokensCachedIn != 0 || tokensOut != 0 || costSubcents != 0 {
		t.Fatalf("rollup totals after rollback = (in=%d cached=%d out=%d cost=%d), want all zero",
			tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}

	// Commit: a fresh transaction with the same shape commits both writes
	// together, proving the office-begun transaction can actually reach
	// task_sessions, a table the task package owns.
	tx2, err := officeRepo.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin tx (commit path): %v", err)
	}
	if err := officeRepo.CreateCostEventTx(ctx, tx2, buildEvent("usage-evt-commit")); err != nil {
		t.Fatalf("create cost event in tx (commit path): %v", err)
	}
	if err := taskRepo.IncrementTaskSessionUsageTx(ctx, tx2, "session-atomic", 100, 40, 50, 500); err != nil {
		t.Fatalf("increment rollup in tx (commit path): %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if n := countCostEvents(); n != 1 {
		t.Fatalf("cost events after commit = %d, want 1", n)
	}
	if tokensIn, tokensCachedIn, tokensOut, costSubcents := rollupTotals(); tokensIn != 100 || tokensCachedIn != 40 || tokensOut != 50 || costSubcents != 500 {
		t.Fatalf("rollup totals after commit = (in=%d cached=%d out=%d cost=%d), want (100, 40, 50, 500)",
			tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}
