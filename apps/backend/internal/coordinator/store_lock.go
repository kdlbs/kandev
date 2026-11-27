package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// withCoordinatorLock runs fn inside a transaction that holds the
// per-coordinator write lock: SQLite BEGIN IMMEDIATE on a dedicated writer
// connection, or PostgreSQL SELECT ... FOR UPDATE on the coordinator row. It
// returns ErrNotFound without running fn when the coordinator row is absent.
// fn may use only the handle it is given, the store's *Tx methods,
// Service.Record and pure computation; s.db.Rebind is allowed for placeholder
// rendering. Reads through the reader pool happen before the lock is taken.
// An error from fn rolls the transaction back and is returned unwrapped.
func (s *Store) withCoordinatorLock(ctx context.Context, coordinatorID string, fn func(tx coordinatorExec) error) error {
	if dialect.IsPostgres(s.db.DriverName()) {
		return s.withCoordinatorLockPostgres(ctx, coordinatorID, fn)
	}
	return s.withCoordinatorLockSQLite(ctx, coordinatorID, fn)
}

func (s *Store) withCoordinatorLockSQLite(ctx context.Context, coordinatorID string, fn func(tx coordinatorExec) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire writer connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			// The rollback must reach SQLite even when ctx is canceled, or
			// the write lock stays open on the pooled connection.
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()
	if s.afterLock != nil {
		s.afterLock(ctx)
	}
	if err := lockCoordinatorRow(ctx, conn, func(q string) string { return q }, coordinatorID, false); err != nil {
		return err
	}
	if err := fn(conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit coordinator lock: %w", err)
	}
	committed = true
	return nil
}

func (s *Store) withCoordinatorLockPostgres(ctx context.Context, coordinatorID string, fn func(tx coordinatorExec) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin coordinator lock: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCoordinatorRow(ctx, tx, s.db.Rebind, coordinatorID, true); err != nil {
		return err
	}
	if s.afterLock != nil {
		s.afterLock(ctx)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit coordinator lock: %w", err)
	}
	return nil
}

// forUpdateClause is the PostgreSQL row-lock suffix; SQLite takes its write
// lock by transaction instead.
const forUpdateClause = " FOR UPDATE"

// lockCoordinatorRow confirms the coordinator row exists on the locked handle;
// on PostgreSQL the FOR UPDATE read is what takes the row lock.
func lockCoordinatorRow(ctx context.Context, exec coordinatorExec, rebind func(string) string, coordinatorID string, forUpdate bool) error {
	query := `SELECT id FROM coordinators WHERE id = ?`
	if forUpdate {
		query += forUpdateClause
	}
	var id string
	if err := exec.QueryRowContext(ctx, rebind(query), coordinatorID).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock coordinator: %w", err)
	}
	return nil
}
