package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const turnSelectColumns = `id, task_session_id, task_id, execution_profile_id, route_generation,
	started_at, completed_at, metadata, created_at, updated_at`

// stampChildStallStartInTx records the parent, workspace, and workflow entry
// a child task's turn starts in, inside the turn-insert transaction. A read
// failure leaves the turn unstamped (and therefore ineligible) rather than
// obstructing the turn.
func (r *Repository) stampChildStallStartInTx(ctx context.Context, tx stepTransitionTx, turn *models.Turn) {
	var parentID, workspaceID sql.NullString
	err := tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT parent_id, workspace_id FROM tasks WHERE id = ?`,
	), turn.TaskID).Scan(&parentID, &workspaceID)
	if err != nil || parentID.String == "" {
		return
	}
	start := models.ChildStallStart{ParentTaskID: parentID.String, WorkspaceID: workspaceID.String}
	var transitionID int64
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id FROM task_step_transitions WHERE task_id = ? ORDER BY id DESC LIMIT 1
	`), turn.TaskID).Scan(&transitionID)
	if err == nil {
		start.TransitionID = transitionID
	}
	if turn.Metadata == nil {
		turn.Metadata = map[string]interface{}{}
	}
	turn.Metadata[models.TurnMetaKeyChildStallStart] = start.ToMap()
}

// AbandonTurn marks a turn as completed with completed_at = started_at, giving it
// zero duration. Used when a turn was orphaned by an interruption (backend
// restart, agent crash) and the previous "running" window was not real work —
// recording `now` would inflate analytics and the UI's last-turn duration with
// hours of dead time. A child task's turn records the abandoned settlement in
// the same transaction so it can never be mistaken for a live settlement.
func (r *Repository) abandonTurnWithChildStallSettlement(ctx context.Context, id string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin abandon turn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var sessionID string
	err = tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT task_session_id FROM task_session_turns WHERE id = ?`,
	), id).Scan(&sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read turn session before abandon: %w", err)
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return err
	}
	var metadataJSON string
	err = tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT metadata FROM task_session_turns WHERE id = ? AND completed_at IS NULL`,
	), id).Scan(&metadataJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read turn before abandon: %w", err)
	}
	serialized, err := abandonedTurnMetadata(metadataJSON)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_turns
		SET completed_at = started_at, metadata = ?, updated_at = ?
		WHERE id = ? AND completed_at IS NULL
	`), serialized, time.Now().UTC(), id); err != nil {
		return err
	}
	return tx.Commit()
}

func abandonedTurnMetadata(metadataJSON string) (string, error) {
	metadata, serialized, err := applyTurnMetadataPatch(metadataJSON, nil, nil)
	if err != nil {
		return "", err
	}
	if _, isChild := metadata[models.TurnMetaKeyChildStallStart]; !isChild {
		return serialized, nil
	}
	_, serialized, err = applyTurnMetadataPatch(metadataJSON, map[string]interface{}{
		models.TurnMetaKeyChildStallSettlement: models.ChildStallSettlementAbandoned,
	}, nil)
	return serialized, err
}

// ListUnresolvedChildStallTurns returns completed child-task turns at or after
// since that the child-turn stalled producer has not resolved, oldest first.
func (r *Repository) ListUnresolvedChildStallTurns(ctx context.Context, since time.Time, limit int) ([]*models.Turn, error) {
	if limit <= 0 {
		limit = 200
	}
	predicate, _ := currentTurnAuthority(r.ro.DriverName(), "turn_row")
	query := `SELECT ` + turnSelectColumns + `
		FROM task_session_turns turn_row
		WHERE completed_at IS NOT NULL AND completed_at >= ? AND ` + predicate + `
		  AND metadata LIKE ? AND metadata NOT LIKE ?
		ORDER BY completed_at ASC, id ASC
		LIMIT ?`
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), since.UTC(),
		`%"`+models.TurnMetaKeyChildStallStart+`"%`,
		`%"`+models.TurnMetaKeyChildStallResolved+`":true%`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []*models.Turn
	for rows.Next() {
		turn, err := scanTurn(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, turn)
	}
	return result, rows.Err()
}

// GetLatestCompletedTurnBySessionID returns the most recently started
// completed turn of a session, or nil when the session has none.
func (r *Repository) GetLatestCompletedTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error) {
	predicate, _ := currentTurnAuthority(r.ro.DriverName(), "turn_row")
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT `+turnSelectColumns+`
		FROM task_session_turns turn_row
		WHERE task_session_id = ? AND completed_at IS NOT NULL AND `+predicate+`
		ORDER BY started_at DESC, created_at DESC, id DESC
		LIMIT 1`), sessionID)
	turn, err := scanTurnRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return turn, err
}
