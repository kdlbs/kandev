package retention

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

// TestSweepSession_SecondBackendSkipsRatherThanBlocks proves
// AC-OFFICE-RUN-HISTORY-RETENTION-002.12: a second backend racing for the
// same advisory lock gets ok=false immediately rather than waiting.
func TestSweepSession_SecondBackendSkipsRatherThanBlocks(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()

	first := testutil.OpenIsolatedPostgres(t, dsn)
	firstPool := db.NewPool(first, first)
	winner, ok, err := acquireSweepSession(ctx, firstPool)
	if err != nil {
		t.Fatalf("acquireSweepSession (winner): %v", err)
	}
	if !ok {
		t.Fatal("winner: ok = false, want true")
	}
	defer winner.release()

	second := testutil.OpenIsolatedPostgres(t, dsn)
	secondPool := db.NewPool(second, second)
	loser, ok, err := acquireSweepSession(ctx, secondPool)
	if err != nil {
		t.Fatalf("acquireSweepSession (loser): %v", err)
	}
	if ok {
		loser.release()
		t.Fatal("loser: ok = true, want false")
	}
}

// TestSweepSession_RunsOnMaxOpenConnsOnePool is F27's regression test: the
// mandated Postgres test harness (testutil.OpenIsolatedPostgres) opens with
// SetMaxOpenConns(1). Reserving the lock connection AND running batches on
// the shared pool would deadlock there, since no second connection is ever
// available. Routing every statement through the lock connection's own
// queryer() must not deadlock and must give a winner more than one table's
// worth of work to do, so a transaction-scoped lock would have incorrectly
// looked sufficient here.
func TestSweepSession_RunsOnMaxOpenConnsOnePool(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()

	conn := testutil.OpenIsolatedPostgres(t, dsn)
	pool := db.NewPool(conn, conn)

	session, ok, err := acquireSweepSession(ctx, pool)
	if err != nil {
		t.Fatalf("acquireSweepSession: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	defer session.release()

	q := session.queryer()
	for i := 0; i < 3; i++ {
		var one int
		if err := q.GetContext(ctx, &one, `SELECT 1`); err != nil {
			t.Fatalf("query %d on lock connection: %v", i, err)
		}
		if one != 1 {
			t.Fatalf("query %d = %d, want 1", i, one)
		}
	}
}

// TestSweepSession_ReleaseAllowsReacquisition proves release() actually
// drops the lock rather than merely closing a connection database/sql
// might still consider live for pooling purposes.
func TestSweepSession_ReleaseAllowsReacquisition(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()
	conn := testutil.OpenIsolatedPostgres(t, dsn)
	pool := db.NewPool(conn, conn)

	first, ok, err := acquireSweepSession(ctx, pool)
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	first.release()

	second, ok, err := acquireSweepSession(ctx, pool)
	if err != nil {
		t.Fatalf("second acquireSweepSession: %v", err)
	}
	if !ok {
		t.Fatal("second acquire: ok = false, want true after release")
	}
	second.release()
}

// TestSweepSession_AliveFalseAfterSessionTerminated proves the
// between-tables liveness check (F25's early exit) actually detects a
// dropped session, and that PostgreSQL's own advisory-lock self-healing
// (F26's justification for a session lock over a lease row) lets a new
// session acquire afterward.
func TestSweepSession_AliveFalseAfterSessionTerminated(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()

	admin := testutil.OpenIsolatedPostgres(t, dsn)
	adminPool := db.NewPool(admin, admin)

	victimConn := testutil.OpenIsolatedPostgres(t, dsn)
	victimPool := db.NewPool(victimConn, victimConn)
	victim, ok, err := acquireSweepSession(ctx, victimPool)
	if err != nil || !ok {
		t.Fatalf("victim acquire: ok=%v err=%v", ok, err)
	}

	var pid int
	if err := victim.conn.GetContext(ctx, &pid, `SELECT pg_backend_pid()`); err != nil {
		t.Fatalf("select pg_backend_pid: %v", err)
	}
	if _, err := adminPool.Writer().ExecContext(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatalf("terminate victim backend: %v", err)
	}

	if victim.alive(ctx) {
		t.Fatal("alive() = true after backend termination, want false")
	}

	replacement, ok, err := acquireSweepSession(ctx, adminPool)
	if err != nil {
		t.Fatalf("replacement acquireSweepSession: %v", err)
	}
	if !ok {
		t.Fatal("replacement: ok = false, want true (terminated session must release the lock)")
	}
	replacement.release()
}
