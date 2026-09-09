package retention

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// newPostgresTestSweeper is sweep_test.go's newTestSweeper, built against a
// real, isolated-schema PostgreSQL connection instead of in-memory SQLite.
// tasks is created first, mirroring production boot order (see
// child_summaries_postgres_test.go).
func newPostgresTestSweeper(t *testing.T, dsn string) (*Sweeper, *sqlx.DB) {
	t.Helper()
	conn := testutil.OpenIsolatedPostgres(t, dsn)
	if _, err := taskrepo.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, err := officesqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("init office schema: %v", err)
	}
	pool := db.NewPool(conn, conn)
	settingsRaw, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("init settings schema: %v", err)
	}
	store := NewStore(pool)
	settingsStore := NewSettingsStore(settingsRaw)
	previewMarker := NewPreviewMarkerStore(settingsRaw)
	return NewSweeper(pool, store, settingsStore, previewMarker), conn
}

// TestRunSweep_TwoBackendsOnePostgres_LoserSkipsAcrossBothTables is
// AC-OFFICE-RUN-HISTORY-RETENTION-002.12's mandated two-backend test: the
// seed gives the winner two tables' worth of work, and the loser's attempt
// happens in the pause between them (via testBetweenTablesSweep). A
// transaction-scoped lock would have released between the winner's two
// per-table statements and let the loser in; this must not happen with the
// session-scoped lock.
func TestRunSweep_TwoBackendsOnePostgres_LoserSkipsAcrossBothTables(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()

	winner, conn := newPostgresTestSweeper(t, dsn)
	saveZeroFloorSettings(t, winner)
	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	seedRoutineRun(t, conn, newID(), "r-1", "done", &old, old)
	seedRun(t, conn, newID(), "agent-1", "finished", &old, old)
	winner.RunSweep(ctx) // preview pass for both tables; no lock contention to test yet

	loser, _ := newPostgresTestSweeper(t, dsn)

	testBetweenTablesSweep = func(queryer) {
		loser.RunSweep(ctx)
	}
	t.Cleanup(func() { testBetweenTablesSweep = nil })

	winner.RunSweep(ctx) // deleting pass: office_routine_runs, pause, then runs

	winnerLast, ok := winner.LastSweepSnapshot()
	if !ok {
		t.Fatal("winner LastSweepSnapshot: ok = false, want true")
	}
	if winnerLast.OfficeRoutineRuns.Deleted != 1 {
		t.Fatalf("winner office_routine_runs.Deleted = %d, want 1", winnerLast.OfficeRoutineRuns.Deleted)
	}
	if winnerLast.Runs.Deleted != 1 {
		t.Fatalf("winner runs.Deleted = %d, want 1 (winner must complete both tables)", winnerLast.Runs.Deleted)
	}

	if _, ok := loser.LastSweepSnapshot(); ok {
		t.Fatal("loser LastSweepSnapshot: ok = true, want false (loser must not have run)")
	}
	loserSkips, _ := loser.SkipSnapshot()
	if loserSkips != 1 {
		t.Fatalf("loser skip count = %d, want 1", loserSkips)
	}
}

// TestRunSweep_LockLostMidSweep_StopsBeforeNextTableAndDoesNotReacquire is
// the companion test the design's Testing section requires: dropping the
// winner's lock connection mid-sweep must stop it before the next table
// (F25) and record the whole attempt as a skip rather than a partial
// result (F28), never attempting to re-acquire.
func TestRunSweep_LockLostMidSweep_StopsBeforeNextTableAndDoesNotReacquire(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	ctx := context.Background()

	victim, conn := newPostgresTestSweeper(t, dsn)
	saveZeroFloorSettings(t, victim)

	// The pool's one connection (SetMaxOpenConns(1)) is what pg_terminate_backend
	// kills below; database/sql transparently opens a replacement on the
	// next query, which starts on the default search_path rather than
	// this test's isolated schema. Capture the schema now to restore it
	// before any post-mortem query on conn.
	var schema string
	if err := conn.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatalf("select current_schema: %v", err)
	}

	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	seedRoutineRun(t, conn, newID(), "r-1", "done", &old, old)
	seedRun(t, conn, newID(), "agent-1", "finished", &old, old)
	victim.RunSweep(ctx) // preview pass for both tables

	admin := testutil.OpenIsolatedPostgres(t, dsn)
	adminPool := db.NewPool(admin, admin)

	testBetweenTablesSweep = func(q queryer) {
		var pid int
		if err := q.GetContext(ctx, &pid, `SELECT pg_backend_pid()`); err != nil {
			t.Fatalf("select pg_backend_pid: %v", err)
		}
		if _, err := adminPool.Writer().ExecContext(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
			t.Fatalf("terminate lock connection: %v", err)
		}
		// pg_terminate_backend signals the backend asynchronously; wait
		// for it to actually leave pg_stat_activity before returning, so
		// the alive() check right after this hook is not racing the
		// signal's delivery.
		deadline := time.Now().Add(5 * time.Second)
		for {
			var stillThere bool
			if err := adminPool.Writer().GetContext(ctx, &stillThere,
				`SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid = $1)`, pid,
			); err != nil {
				t.Fatalf("poll pg_stat_activity: %v", err)
			}
			if !stillThere {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("backend %d still present in pg_stat_activity after 5s", pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.Cleanup(func() { testBetweenTablesSweep = nil })

	beforeAttempt, ok := victim.LastSweepSnapshot()
	if !ok {
		t.Fatal("LastSweepSnapshot after the preview pass: ok = false, want true")
	}

	victim.RunSweep(ctx) // deleting pass: office_routine_runs succeeds, then the lock connection dies

	// The preview pass already set LastSweep; a lock lost mid-attempt
	// must leave it exactly as-is rather than publishing a partial
	// result (F28) — not become unset, which would also be true after a
	// genuinely successful sweep with nothing yet recorded.
	afterAttempt, ok := victim.LastSweepSnapshot()
	if !ok {
		t.Fatal("LastSweepSnapshot after the lost-lock attempt: ok = false, want true (still the preview pass's result)")
	}
	if afterAttempt != beforeAttempt {
		t.Fatalf("LastSweep changed after a lost-lock attempt: before=%+v after=%+v", beforeAttempt, afterAttempt)
	}
	skips, _ := victim.SkipSnapshot()
	if skips != 1 {
		t.Fatalf("skip count = %d, want 1", skips)
	}

	// conn's one physical connection was the one just terminated;
	// database/sql opened a replacement on the default search_path, so
	// restore the isolated schema before verifying table state.
	if _, err := conn.Exec("SET search_path TO " + schema); err != nil {
		t.Fatalf("restore search_path: %v", err)
	}

	// office_routine_runs' delete committed before the session died
	// (AC-002.5: batches already committed stay committed).
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs`); n != 0 {
		t.Fatalf("office_routine_runs rows = %d, want 0 (the first table's committed delete survives)", n)
	}
	// runs was never reached.
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs`); n != 1 {
		t.Fatalf("runs rows = %d, want 1 (the second table must not have been touched)", n)
	}

	// A fresh session can now acquire the lock: it was not re-acquired by
	// the victim, and PostgreSQL released it when the session ended.
	replacement, ok, err := acquireSweepSession(ctx, adminPool)
	if err != nil {
		t.Fatalf("acquireSweepSession: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true (the terminated session must have released the lock)")
	}
	replacement.release()
}
