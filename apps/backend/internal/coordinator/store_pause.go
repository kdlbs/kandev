package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SetPaused sets or clears the coordinator's paused state with one conditional
// statement under the wake lock, so it serialises with turn reservation and
// with every other pause or resume. changed is false when the state already
// was as requested. ErrNotFound when the coordinator row is absent.
func (s *Store) SetPaused(ctx context.Context, coordinatorID string, paused bool, by string) (changed bool, err error) {
	err = s.WithWakeLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		var res sql.Result
		var execErr error
		if paused {
			res, execErr = tx.ExecContext(ctx, s.db.Rebind(`
				UPDATE coordinators SET paused_at = ?, paused_by = ? WHERE id = ? AND paused_at IS NULL`),
				s.now().UTC(), by, coordinatorID)
		} else {
			res, execErr = tx.ExecContext(ctx, s.db.Rebind(`
				UPDATE coordinators SET paused_at = NULL, paused_by = NULL WHERE id = ? AND paused_at IS NOT NULL`),
				coordinatorID)
		}
		changed, execErr = rowChanged(res, execErr, "set coordinator paused")
		return execErr
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

// IsPaused reads the stored state; a coordinator with no row reads as not paused.
func (s *Store) IsPaused(ctx context.Context, coordinatorID string) (bool, error) {
	var at sql.NullTime
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT paused_at FROM coordinators WHERE id = ?`), coordinatorID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read paused state: %w", err)
	}
	return at.Valid, nil
}

// PausedCoordinatorIDs lists the paused coordinators ordered by id.
func (s *Store) PausedCoordinatorIDs(ctx context.Context) ([]string, error) {
	return s.queryIDs(ctx, "paused coordinators", `SELECT id FROM coordinators WHERE paused_at IS NOT NULL ORDER BY id`)
}
