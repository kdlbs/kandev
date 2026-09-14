package retention

import (
	"context"
	"database/sql/driver"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// advisoryLockKey is retention's own PostgreSQL advisory lock namespace,
// distinct from every other hashtextextended(?, 0) call site in the repo
// (participants.go, secrets/sqlite_store.go, workflow/repository/phase2_sqlite.go)
// so a sweep never contends with participant-seat, secret-transfer, or
// workflow-phase locking.
const advisoryLockKey = "office_run_retention_sweep"

const unlockTimeout = 5 * time.Second

// sweepSession is a PostgreSQL session-scoped, non-blocking exclusivity
// lock for one sweep (AC-OFFICE-RUN-HISTORY-RETENTION-002.12):
//
//   - Single connection budget: every statement the sweep issues — count,
//     delete, census — runs through this one dedicated connection,
//     returned by queryer(), rather than reserving it for the lock alone
//     and running batches on the shared pool. Reserving a second
//     connection is exactly what a maxOpenConns=1 pool (what
//     testutil.OpenIsolatedPostgres, the mandated Postgres-gated test
//     harness, sets) cannot supply; one connection total removes the
//     deadlock.
//   - Exclusivity window: because every sweep statement runs on the lock
//     connection, there is no window where work proceeds on a different
//     connection after the session died — if the session ends, the very
//     next statement on it fails immediately instead of continuing to run
//     against the pool while another backend has already re-acquired the
//     lock. The between-tables alive() check the design specifies is kept
//     anyway, as a cheap early exit before starting a table's work rather
//     than the only guard against loss of exclusivity.
//   - Release safety: release() unlocks and closes on an independent
//     context, not the sweep's (which may already be cancelled), with a
//     bounded timeout, and discards the connection via driver.ErrBadConn
//     whenever the unlock did not provably succeed — so database/sql
//     never pools a session that may still hold the lock, which is what
//     the design's "cannot wedge retention permanently" claim actually
//     requires (internal/db sets no ConnMaxLifetime).
type sweepSession struct {
	conn *sqlx.Conn
}

// acquireSweepSession tries to take the advisory lock on a fresh dedicated
// connection. ok is false when another backend already holds it or the
// connection could not be checked out; the caller records a skip and
// returns rather than retrying (AC-OFFICE-RUN-HISTORY-RETENTION-002.12).
func acquireSweepSession(ctx context.Context, pool *db.Pool) (*sweepSession, bool, error) {
	conn, err := pool.Writer().Connx(ctx)
	if err != nil {
		return nil, false, err
	}

	var acquired bool
	err = conn.GetContext(ctx, &acquired, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, advisoryLockKey)
	if err != nil {
		_ = conn.Close()
		return nil, false, err
	}
	if !acquired {
		_ = conn.Close()
		return nil, false, nil
	}
	return &sweepSession{conn: conn}, true, nil
}

// queryer is the connection every sweep statement must run through for the
// whole sweep's duration (see the single-connection-budget and
// exclusivity-window invariants on sweepSession above).
func (s *sweepSession) queryer() queryer {
	return s.conn
}

// alive reports whether the lock connection is still usable. Checked
// between tables; the sweep stops before the next table when this returns
// false and does not attempt to re-acquire, since a re-acquisition after
// another backend has taken the lock would produce exactly the concurrent
// sweep AC-OFFICE-RUN-HISTORY-RETENTION-002.12 exists to prevent.
func (s *sweepSession) alive(ctx context.Context) bool {
	return s.conn.PingContext(ctx) == nil
}

// release unlocks and closes the session. Always safe to call once; never
// call it twice.
func (s *sweepSession) release() {
	ctx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
	defer cancel()

	var unlocked bool
	err := s.conn.GetContext(ctx, &unlocked, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, advisoryLockKey)
	if err != nil || !unlocked {
		// The unlock did not provably succeed: force database/sql to
		// discard this connection instead of returning it to the pool,
		// so a session that may still hold the lock can never be reused
		// by a later, unrelated caller.
		_ = s.conn.Raw(func(driverConn any) error { return driver.ErrBadConn })
	}
	_ = s.conn.Close()
}
