package retention

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

// openSharedSchemaPostgresConn opens a second, independent PostgreSQL
// connection pointed at the same isolated test schema as an existing
// connection. A real second physical connection is required to hold a row
// lock that a concurrent statement on the first connection genuinely blocks
// on — a sequential in-process test hook cannot reproduce that.
func openSharedSchemaPostgresConn(t *testing.T, dsn, schema string) *sqlx.DB {
	t.Helper()
	raw, err := db.OpenPostgres(dsn, 1, 1)
	if err != nil {
		t.Fatalf("open second postgres connection: %v", err)
	}
	conn := sqlx.NewDb(raw, "pgx")
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("set search_path on second connection: %v", err)
	}
	return conn
}

// TestDeleteRunBatch_Postgres_ConcurrentResurrectionDuringDeleteExcludesRow
// is the regression test for deleteRunsByIDs' missing direct status/age
// predicate (AC-OFFICE-RUN-HISTORY-RETENTION-002.4): a run resurrected by a
// concurrent transaction that has not yet committed when DeleteRunBatch
// selects it, but commits while the DELETE statement is blocked acquiring
// the row's lock — driving PostgreSQL's real EvalPlanQual recheck path. The
// package's existing resurrection tests
// (TestDeleteRunBatch_MidTransactionResurrectionRetriesThenSurvives) use a
// sequential in-process hook that resurrects strictly before the DELETE
// statement starts; they cannot reach this mid-statement window, which
// needs a second, genuinely concurrent connection.
func TestDeleteRunBatch_Postgres_ConcurrentResurrectionDuringDeleteExcludesRow(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()

	sweeper, conn := newPostgresTestSweeper(t, dsn)
	store := sweeper.store

	var schema string
	if err := conn.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatalf("select current_schema: %v", err)
	}

	runID := newID()
	finished := daysAgo(60)
	seedRun(t, conn, runID, "agent-1", "finished", &finished, finished)
	seedRunEvent(t, conn, runID, 0)

	holder := openSharedSchemaPostgresConn(t, dsn, schema)
	holderTx, err := holder.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder tx: %v", err)
	}
	// Resurrecting the row inside an uncommitted transaction takes its row
	// lock immediately, but the new values are not visible to conn's own
	// snapshot until Commit below: selectEligibleRunIDs' plain read is
	// never blocked by an uncommitted writer, so it still sees the row as
	// terminal and eligible, exactly the window this test targets.
	if _, err := holderTx.ExecContext(ctx, `UPDATE runs SET status = 'queued', finished_at = NULL WHERE id = $1`, runID); err != nil {
		t.Fatalf("resurrect run inside holder tx: %v", err)
	}

	admin := testutil.OpenIsolatedPostgres(t, dsn) // separate schema; pg_stat_activity is instance-wide, not schema-scoped
	deleteDone := make(chan RunBatchResult, 1)
	deleteErr := make(chan error, 1)
	go func() {
		result, err := store.DeleteRunBatch(ctx, conn, daysAgo(30), 0, 100)
		if err != nil {
			deleteErr <- err
			return
		}
		deleteDone <- result
	}()

	// Wait for DeleteRunBatch's DELETE statement to actually be blocked on
	// the holder's row lock before committing, so the recheck this test
	// targets is guaranteed to happen rather than racing ahead of it.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		if err := admin.GetContext(ctx, &waiting, `
			SELECT EXISTS(
				SELECT 1 FROM pg_stat_activity
				WHERE wait_event_type = 'Lock' AND query ILIKE '%DELETE FROM runs%'
			)`); err != nil {
			t.Fatalf("poll pg_stat_activity: %v", err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("DeleteRunBatch's DELETE never showed up waiting on the holder's row lock")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := holderTx.Commit(); err != nil {
		t.Fatalf("commit holder tx: %v", err)
	}

	var result RunBatchResult
	select {
	case result = <-deleteDone:
	case err := <-deleteErr:
		t.Fatalf("DeleteRunBatch: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("DeleteRunBatch did not complete after the holder committed")
	}

	if result.RunsDeleted != 0 || result.Abandoned {
		t.Fatalf("result = %+v, want a clean no-op: the row was resurrected before the delete's row lock was granted, so PostgreSQL's own recheck of the row must see it as no longer eligible", result)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE id = ?`, runID); n != 1 {
		t.Fatalf("resurrected run was deleted despite the concurrent recheck")
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM run_events WHERE run_id = ?`, runID); n != 1 {
		t.Fatalf("resurrected run's event was deleted despite the concurrent recheck")
	}
}

// TestDeleteRoutineRunsBatch_Postgres_OldestFirstWithBacklogMatchesSQLite is
// AC-OFFICE-RUN-HISTORY-RETENTION-005.1's mandated backlog-path parity test:
// batch selection order is fixed by named columns
// (completion_time ASC, id ASC) rather than left to the engine, so this
// must select and delete the exact same rows on PostgreSQL as
// TestDeleteRoutineRunsBatch_OldestFirstAndOrderedByNamedColumns proves on
// SQLite for the identical settings and starting rows.
func TestDeleteRoutineRunsBatch_Postgres_OldestFirstWithBacklogMatchesSQLite(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()

	sweeper, conn := newPostgresTestSweeper(t, dsn)
	store := sweeper.store

	routineID := newID()
	seedRoutine(t, conn, routineID)

	cutoff := daysAgo(30)
	oldest, middle, newest := newID(), newID(), newID()
	oldC, midC, newC := daysAgo(90), daysAgo(60), daysAgo(45)
	seedRoutineRun(t, conn, oldest, routineID, "done", &oldC, oldC)
	seedRoutineRun(t, conn, middle, routineID, "done", &midC, midC)
	seedRoutineRun(t, conn, newest, routineID, "done", &newC, newC)

	// floor 0 so all three are eligible; batch limit 2 -> the two oldest
	// go, the newest survives as backlog — same as the SQLite test.
	deleted, err := store.DeleteRoutineRunsBatch(ctx, conn, cutoff, 0, 2)
	if err != nil {
		t.Fatalf("DeleteRoutineRunsBatch: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE id = ?`, newest); n != 1 {
		t.Fatal("newest row was deleted on PostgreSQL; oldest-first ordering violated")
	}
	for _, id := range []string{oldest, middle} {
		if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE id = ?`, id); n != 0 {
			t.Fatalf("row %s (older) still present on PostgreSQL after batch limit 2", id)
		}
	}

	eligible, err := store.CountEligibleRoutineRuns(ctx, conn, cutoff, 0)
	if err != nil {
		t.Fatalf("CountEligibleRoutineRuns: %v", err)
	}
	if eligible != 1 {
		t.Fatalf("remaining eligible = %d, want 1 (backlog: the newest row is still eligible, just not yet batched)", eligible)
	}
}
