package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// turnReadRow is one coordinator_unattended_turns row as the read routes
// show it.
type turnReadRow struct {
	ID              string
	CoordinatorID   string
	ConvTaskID      string
	SessionID       string
	StartedAt       time.Time
	FinishedAt      *time.Time
	Outcome         *string
	WakeCount       int
	DeniedPerms     int
	CostSubcents    *int64
	StopRequestedAt *time.Time
}

// wakeReadRow is one wake row attached to a run.
type wakeReadRow struct {
	ID     string
	Kind   string
	TaskID string
}

const turnReadColumns = `id, coordinator_id, conversation_task_id, session_id, started_at, finished_at, outcome,
	wake_count, denied_permissions, cost_subcents, stop_requested_at`

func scanTurnReadRow(sc interface{ Scan(...any) error }) (*turnReadRow, error) {
	var r turnReadRow
	var fin, stop sql.NullTime
	var outcome sql.NullString
	var cost sql.NullInt64
	if err := sc.Scan(&r.ID, &r.CoordinatorID, &r.ConvTaskID, &r.SessionID, &r.StartedAt, &fin, &outcome,
		&r.WakeCount, &r.DeniedPerms, &cost, &stop); err != nil {
		return nil, err
	}
	r.StartedAt = r.StartedAt.UTC()
	if fin.Valid {
		v := fin.Time.UTC()
		r.FinishedAt = &v
	}
	if stop.Valid {
		v := stop.Time.UTC()
		r.StopRequestedAt = &v
	}
	if outcome.Valid {
		r.Outcome = &outcome.String
	}
	if cost.Valid {
		r.CostSubcents = &cost.Int64
	}
	return &r, nil
}

// newestTurnRead is the coordinator's newest turn row by started_at then id,
// nil when it has none.
func (s *Store) newestTurnRead(ctx context.Context, coordinatorID string) (*turnReadRow, error) {
	r, err := scanTurnReadRow(s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT `+turnReadColumns+`
		FROM coordinator_unattended_turns WHERE coordinator_id = ?
		ORDER BY started_at DESC, id DESC LIMIT 1`), coordinatorID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read newest unattended turn: %w", err)
	}
	return r, nil
}

// turnReadByID is one of the coordinator's turn rows, nil when the id names no
// row of that coordinator.
func (s *Store) turnReadByID(ctx context.Context, coordinatorID, id string) (*turnReadRow, error) {
	r, err := scanTurnReadRow(s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT `+turnReadColumns+`
		FROM coordinator_unattended_turns WHERE id = ? AND coordinator_id = ?`), id, coordinatorID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read unattended turn: %w", err)
	}
	return r, nil
}

// lastWokeAt is the started_at of the newest turn row whose message was
// accepted (session_turn_id set), nil when none.
func (s *Store) lastWokeAt(ctx context.Context, coordinatorID string) (*time.Time, error) {
	var at time.Time
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT started_at FROM coordinator_unattended_turns
		WHERE coordinator_id = ? AND session_turn_id IS NOT NULL
		ORDER BY started_at DESC, id ASC LIMIT 1`), coordinatorID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read last woke: %w", err)
	}
	at = at.UTC()
	return &at, nil
}

// pendingWakeSummary is the count and oldest created_at of the coordinator's
// pending wakes from one query, so the two always agree.
func (s *Store) pendingWakeSummary(ctx context.Context, coordinatorID string) (int, *time.Time, error) {
	var oldest time.Time
	var n int
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT created_at, COUNT(*) OVER ()
		FROM coordinator_wakes WHERE coordinator_id = ? AND status = ?
		ORDER BY created_at ASC, id ASC LIMIT 1`), coordinatorID, wakeStatusPending).Scan(&oldest, &n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("read pending wakes: %w", err)
	}
	oldest = oldest.UTC()
	return n, &oldest, nil
}

// wakesOfTurn lists the wakes attached to a turn in message order.
func (s *Store) wakesOfTurn(ctx context.Context, turnID string) ([]wakeReadRow, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`SELECT id, kind, task_id FROM coordinator_wakes
		WHERE turn_id = ? ORDER BY created_at ASC, id ASC`), turnID)
	if err != nil {
		return nil, fmt.Errorf("list run wakes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []wakeReadRow{}
	for rows.Next() {
		var w wakeReadRow
		if err := rows.Scan(&w.ID, &w.Kind, &w.TaskID); err != nil {
			return nil, fmt.Errorf("scan run wake: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list run wakes: %w", err)
	}
	return out, nil
}
