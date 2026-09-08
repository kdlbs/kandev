package sqlite

// TestUsageEventInsertAndRollupShareOneTransaction is the repointed
// replacement (docs/specs/task-cost-ledger/spec.md AC-21, AC-36) for the
// deleted office/repository/sqlite test
// TestCostEventTxAtomicity_RealReposShareTransaction. That test drove
// office.Repository.BeginTx -> office.Repository.CreateCostEventTx ->
// task.Repository.IncrementTaskSessionUsageTx to prove the transaction
// office's recordCostEventAndRollup began could really reach task_sessions,
// a table the task package owns - not just a test double that discarded its
// *sqlx.Tx parameter (PR #2606 review round 1, F2).
//
// AC-21 removes that caller: recordCostEventAndRollup no longer begins a
// shared transaction at all. IncrementTaskSessionUsageTx is retained because
// the ledger writer's own insertUsageEventAndRollup (usage_events.go) is now
// the real caller of this exact capability, so this test repoints the same
// rollback-then-commit shape at the ledger writer's own two writes -
// insertUsageEventRowTx and IncrementTaskSessionUsageTx - sharing one
// *sqlx.Tx, driven manually rather than through CreateTaskUsageEvent so the
// tx != nil branch is exercised explicitly, the same guarantee the original
// test existed to pin.

import (
	"context"
	"testing"
)

func TestUsageEventInsertAndRollupShareOneTransaction(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ctx := context.Background()
	createUsageEventsTestTask(t, repo, "task-shared-tx")
	createUsageEventsTestSession(t, repo, "session-shared-tx", "task-shared-tx")

	// Rollback: both writes land in the same transaction; rolling it back
	// must undo both, not just the one whose caller happened to fail.
	rollbackEvent := newTestUsageEvent("evt-shared-tx-rollback", "task-shared-tx", "session-shared-tx")
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := repo.insertUsageEventRowTx(ctx, tx, rollbackEvent); err != nil {
		t.Fatalf("insert usage event row in tx: %v", err)
	}
	if err := repo.IncrementTaskSessionUsageTx(ctx, tx, "session-shared-tx", 100, 25, 30, 42); err != nil {
		t.Fatalf("increment rollup in tx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != 0 {
		t.Fatalf("row count after rollback = %d, want 0", got)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-shared-tx")
	if tokensIn != 0 || tokensCachedIn != 0 || tokensOut != 0 || costSubcents != 0 {
		t.Fatalf("rollup after rollback = (%d,%d,%d,%d), want all zero", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}

	// Commit: a fresh transaction with the same shape commits both writes
	// together.
	commitEvent := newTestUsageEvent("evt-shared-tx-commit", "task-shared-tx", "session-shared-tx")
	tx2, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx (commit path): %v", err)
	}
	if err := repo.insertUsageEventRowTx(ctx, tx2, commitEvent); err != nil {
		t.Fatalf("insert usage event row in tx (commit path): %v", err)
	}
	if err := repo.IncrementTaskSessionUsageTx(ctx, tx2, "session-shared-tx", 100, 25, 30, 42); err != nil {
		t.Fatalf("increment rollup in tx (commit path): %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != 1 {
		t.Fatalf("row count after commit = %d, want 1", got)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents = readTaskSessionRollup(t, repo, "session-shared-tx")
	if tokensIn != 100 || tokensCachedIn != 25 || tokensOut != 30 || costSubcents != 42 {
		t.Fatalf("rollup after commit = (%d,%d,%d,%d), want (100,25,30,42)", tokensIn, tokensCachedIn, tokensOut, costSubcents)
	}
}
