package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ceilingTurn is the part of a coordinator_unattended_turns row the ceiling
// stop reads.
type ceilingTurn struct {
	ID                   string     `db:"id"`
	CoordinatorID        string     `db:"coordinator_id"`
	SessionID            string     `db:"session_id"`
	SessionTurnID        *string    `db:"session_turn_id"`
	StartCeilingSubcents int64      `db:"start_ceiling_subcents"`
	StopRequestedAt      *time.Time `db:"stop_requested_at"`
	Outcome              *string    `db:"outcome"`
}

const ceilingTurnColumns = `id, coordinator_id, session_id, session_turn_id, start_ceiling_subcents, stop_requested_at, outcome`

// openCeilingTurn returns the coordinator's open unattended turn, or nil when
// there is none (the partial unique index allows at most one).
func (s *Store) openCeilingTurn(ctx context.Context, coordinatorID string) (*ceilingTurn, error) {
	var row ceilingTurn
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT `+ceilingTurnColumns+` FROM coordinator_unattended_turns
		WHERE coordinator_id = ? AND outcome IS NULL`), coordinatorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read open unattended turn: %w", err)
	}
	return &row, nil
}

// ceilingTurnByID re-reads one turn row, nil when it is gone.
func (s *Store) ceilingTurnByID(ctx context.Context, id string) (*ceilingTurn, error) {
	var row ceilingTurn
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`
		SELECT `+ceilingTurnColumns+` FROM coordinator_unattended_turns WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read unattended turn: %w", err)
	}
	return &row, nil
}

// markTurnStopRequested sets the retry marker on an open, unmarked turn and
// reports whether this call set it.
func (s *Store) markTurnStopRequested(ctx context.Context, id string, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_unattended_turns SET stop_requested_at = ?
		WHERE id = ? AND outcome IS NULL AND stop_requested_at IS NULL`), at.UTC(), id)
	return rowChanged(res, err, "mark unattended turn stop")
}

// settleTurnStoppedAtCeiling settles an open turn as stopped_at_ceiling and
// reports whether this call changed the row. cost nil stores NULL.
func (s *Store) settleTurnStoppedAtCeiling(ctx context.Context, id string, at time.Time, cost *int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_unattended_turns
		SET outcome = 'stopped_at_ceiling', finished_at = ?, cost_subcents = ?
		WHERE id = ? AND outcome IS NULL`), at.UTC(), cost, id)
	return rowChanged(res, err, "settle unattended turn at ceiling")
}

// raiseTurnCost fills a settled turn's NULL cost or raises a lower one. It
// never lowers a stored value, so a stale concurrent recompute cannot undo a
// newer one and repeating it is idempotent.
func (s *Store) raiseTurnCost(ctx context.Context, id string, cost int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_unattended_turns SET cost_subcents = ?
		WHERE id = ? AND outcome IS NOT NULL AND (cost_subcents IS NULL OR cost_subcents < ?)`), cost, id, cost)
	return rowChanged(res, err, "raise unattended turn cost")
}

func rowChanged(res sql.Result, err error, what string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("%s rows affected: %w", what, err)
	}
	return n > 0, nil
}
