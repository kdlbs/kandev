package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
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
	query := `SELECT ` + activityReadColumns + ` FROM coordinator_activity WHERE coordinator_id = ?`
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
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT `+activityReadColumns+` FROM coordinator_activity WHERE id = ? AND coordinator_id = ?`), id, coordinatorID).
		Scan(&row.ID, &row.CoordinatorID, &row.WorkspaceID, &row.ActionClass, &row.Outcome, &row.Authorization,
			&row.TargetTaskID, &row.ProposalID, &row.ActorUserID, &row.ReasonCode, &row.Detail, &row.Edited,
			&row.RefusalCount, &row.UndoneAt, &row.UndoneBy, &row.UndoOfID, &row.CreatedAt, &row.UpdatedAt, &row.UnattendedTurnID)
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

// ActivityCountsIn groups the coordinator's rows created in [since, until)
// through exec, so a caller holding the coordinator lock reads on its own
// handle.
func (s *Store) ActivityCountsIn(ctx context.Context, exec coordinatorExec, coordinatorID string, since, until time.Time) ([]ActivityCount, error) {
	rows, err := exec.QueryContext(ctx, s.db.Rebind(`SELECT action_class, outcome, edited, COUNT(*) AS rows, COALESCE(SUM(refusal_count), 0) AS refusals
		FROM coordinator_activity WHERE coordinator_id = ? AND created_at >= ? AND created_at < ?
		GROUP BY action_class, outcome, edited`), coordinatorID, since.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("count coordinator activity: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ActivityCount
	for rows.Next() {
		var c ActivityCount
		if err := rows.Scan(&c.Class, &c.Outcome, &c.Edited, &c.Rows, &c.Refusal); err != nil {
			return nil, fmt.Errorf("scan coordinator activity count: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
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

// MoveOutcomes reads the outcome_json of the coordinator's proposals by id,
// one query for a page. A proposal with no outcome maps to nil; an absent
// proposal is absent from the map.
func (s *Store) MoveOutcomes(ctx context.Context, coordinatorID string, ids []string) (map[string]*string, error) {
	out := map[string]*string{}
	if len(ids) == 0 {
		return out, nil
	}
	query, args, err := sqlx.In(`SELECT id, outcome_json FROM coordinator_proposals WHERE coordinator_id = ? AND id IN (?)`, coordinatorID, ids)
	if err != nil {
		return nil, fmt.Errorf("read proposal outcomes: %w", err)
	}
	var rows []struct {
		ID      string         `db:"id"`
		Outcome sql.NullString `db:"outcome_json"`
	}
	if err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("read proposal outcomes: %w", err)
	}
	for _, r := range rows {
		if r.Outcome.Valid {
			v := r.Outcome.String
			out[r.ID] = &v
		} else {
			out[r.ID] = nil
		}
	}
	return out, nil
}
