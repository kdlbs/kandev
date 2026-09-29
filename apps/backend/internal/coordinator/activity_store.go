package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ActivityCursor is the keyset position after which the next page starts.
type ActivityCursor struct {
	CreatedAt time.Time
	ID        string
}

// ActivityCount is one grouped summary bucket.
type ActivityCount struct {
	Class   Action          `db:"action_class"`
	Outcome ActivityOutcome `db:"outcome"`
	Edited  bool            `db:"edited"`
	Rows    int64           `db:"rows"`
	Refusal int64           `db:"refusals"`
}

// ListActivityRows returns up to limit rows of one coordinator, newest first,
// strictly after before when set, restricted to class when non-empty.
func (s *Store) ListActivityRows(ctx context.Context, coordinatorID string, class Action, before *ActivityCursor, limit int) ([]ActivityRow, error) {
	query := `SELECT ` + activityColumns + ` FROM coordinator_activity WHERE coordinator_id = ?`
	args := []any{coordinatorID}
	if class != "" {
		query += ` AND action_class = ?`
		args = append(args, string(class))
	}
	if before != nil {
		at := before.CreatedAt.UTC()
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, at, at, before.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	var rows []ActivityRow
	if err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list coordinator activity: %w", err)
	}
	return rows, nil
}

// GetActivityRow reads one row of the coordinator through exec; ErrNotFound
// when the row is absent or belongs to another coordinator.
func (s *Store) GetActivityRow(ctx context.Context, exec coordinatorExec, coordinatorID, id string) (*ActivityRow, error) {
	var row ActivityRow
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT `+activityColumns+` FROM coordinator_activity WHERE id = ? AND coordinator_id = ?`), id, coordinatorID).
		Scan(&row.ID, &row.CoordinatorID, &row.WorkspaceID, &row.ActionClass, &row.Outcome, &row.Authorization,
			&row.TargetTaskID, &row.ProposalID, &row.ActorUserID, &row.ReasonCode, &row.Detail, &row.Edited,
			&row.RefusalCount, &row.UndoneAt, &row.UndoneBy, &row.UndoOfID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get coordinator activity: %w", err)
	}
	return &row, nil
}

// ActivityCounts groups the coordinator's rows created at or after since.
func (s *Store) ActivityCounts(ctx context.Context, coordinatorID string, since time.Time) ([]ActivityCount, error) {
	var out []ActivityCount
	err := s.ro.SelectContext(ctx, &out, s.ro.Rebind(`SELECT action_class, outcome, edited, COUNT(*) AS rows, COALESCE(SUM(refusal_count), 0) AS refusals
		FROM coordinator_activity WHERE coordinator_id = ? AND created_at >= ?
		GROUP BY action_class, outcome, edited`), coordinatorID, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("count coordinator activity: %w", err)
	}
	return out, nil
}

// EarliestActivityAt is the oldest row time of the coordinator, nil with none.
func (s *Store) EarliestActivityAt(ctx context.Context, coordinatorID string) (*time.Time, error) {
	var at time.Time
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT created_at FROM coordinator_activity WHERE coordinator_id = ? ORDER BY created_at, id LIMIT 1`), coordinatorID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("earliest coordinator activity: %w", err)
	}
	at = at.UTC()
	return &at, nil
}
