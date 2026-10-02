package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// upsertStallSQL is valid on both SQLite and PostgreSQL (docs/specs/
// coordinator/system-design/needs-you.md#stall-records). The WHERE clause on
// the DO UPDATE fences a strictly newer last_event_at: a redelivered or
// out-of-order event (equal or earlier last_event_at) is skipped, and the
// skip itself is not reported as an affected row by either dialect's driver.
const upsertStallSQL = `
	INSERT INTO coordinator_stalls (task_id, workspace_id, stalled_for_ms, last_event_at, detected_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT (task_id) DO UPDATE SET
		stalled_for_ms = excluded.stalled_for_ms,
		last_event_at = excluded.last_event_at,
		detected_at = excluded.detected_at,
		workspace_id = excluded.workspace_id
	WHERE excluded.last_event_at > coordinator_stalls.last_event_at`

// UpsertStall inserts or replaces a coordinator_stalls row, per Build
// decision 12. Returns whether the row was inserted or updated; a
// redelivery or out-of-order event (last_event_at not strictly newer than
// the stored value) returns false without error.
func (s *Store) UpsertStall(ctx context.Context, stall *Stall) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(upsertStallSQL),
		stall.TaskID, stall.WorkspaceID, stall.StalledForMs, stall.LastEventAt, stall.DetectedAt)
	if err != nil {
		return false, fmt.Errorf("upsert stall: %w", err)
	}
	return matchedRow(res)
}

// DeleteStall deletes the stall row for taskID. ErrNotFound if no row
// matched.
func (s *Store) DeleteStall(ctx context.Context, taskID string) error {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM coordinator_stalls WHERE task_id = ?`), taskID)
	if err != nil {
		return fmt.Errorf("delete stall: %w", err)
	}
	matched, err := matchedRow(res)
	if err != nil {
		return err
	}
	if !matched {
		return ErrNotFound
	}
	return nil
}

const stallColumns = `task_id, workspace_id, stalled_for_ms, last_event_at, detected_at`

// stallRow is the DB scan target for coordinator_stalls.
type stallRow struct {
	TaskID       string    `db:"task_id"`
	WorkspaceID  string    `db:"workspace_id"`
	StalledForMs int64     `db:"stalled_for_ms"`
	LastEventAt  time.Time `db:"last_event_at"`
	DetectedAt   time.Time `db:"detected_at"`
}

func (r *stallRow) toStall() *Stall {
	return &Stall{
		TaskID:       r.TaskID,
		WorkspaceID:  r.WorkspaceID,
		StalledForMs: r.StalledForMs,
		LastEventAt:  r.LastEventAt,
		DetectedAt:   r.DetectedAt,
	}
}

// GetStall returns the coordinator_stalls row for taskID scoped to
// workspaceID (docs/specs/coordinator/system-design/copilot-tools.md#item-read).
// ErrNotFound if no row matches: the task never stalled, or its row was
// cleared by a later non-stalled sweep pass.
func (s *Store) GetStall(ctx context.Context, workspaceID, taskID string) (*Stall, error) {
	var row stallRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT `+stallColumns+` FROM coordinator_stalls WHERE task_id = ? AND workspace_id = ?`),
		taskID, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get stall: %w", err)
	}
	return row.toStall(), nil
}

// ListStalls returns every stall row of a workspace ordered by task_id, per
// Build decision 12. Never nil.
func (s *Store) ListStalls(ctx context.Context, workspaceID string) ([]*Stall, error) {
	var rows []stallRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT `+stallColumns+` FROM coordinator_stalls WHERE workspace_id = ? ORDER BY task_id`),
		workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list stalls: %w", err)
	}
	result := make([]*Stall, len(rows))
	for i := range rows {
		result[i] = rows[i].toStall()
	}
	return result, nil
}
