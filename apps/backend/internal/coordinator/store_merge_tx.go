package coordinator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// withMergeTx runs fn in one write transaction. On PostgreSQL it takes the
// advisory lock of the change key, then the wake lock of each coordinator in
// ascending id order (the same lock RecordWake takes), so concurrent
// transitions of one change and wake recordings cannot interleave or
// deadlock. On SQLite the single writer connection serialises them.
func (s *Store) withMergeTx(ctx context.Context, changeKey string, coordinatorIDs []string, fn func(tx coordinatorExec) error) error {
	if !dialect.IsPostgres(s.db.DriverName()) {
		return s.withSQLiteWriteTx(ctx, fn)
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin merge tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`SELECT pg_advisory_xact_lock(hashtextextended('coordinator_change:' || ?, 0))`), changeKey); err != nil {
		return fmt.Errorf("take change lock: %w", err)
	}
	for _, id := range coordinatorIDs {
		if err := s.takeWakeLock(ctx, tx, id); err != nil {
			return err
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit merge tx: %w", err)
	}
	return nil
}

func (s *Store) withSQLiteWriteTx(ctx context.Context, fn func(tx coordinatorExec) error) error {
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
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()
	if err := fn(conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit merge tx: %w", err)
	}
	committed = true
	return nil
}
