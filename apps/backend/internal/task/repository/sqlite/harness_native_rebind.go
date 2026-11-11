package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// RebindHarnessGenerationNativeSession moves the current generation and the
// execution's resume token to a native conversation created by an in-place
// context reset. Both writes and the history row commit together, or nothing
// changes. The token may already hold the new ID when the fresh session event
// won the race; a generation already bound to the new ID is accepted unchanged.
func (r *Repository) RebindHarnessGenerationNativeSession(ctx context.Context, req models.HarnessNativeRebind) (bool, error) {
	if !completeNativeRebind(req) {
		return false, fmt.Errorf("complete native rebind identity is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockNativeRebindTx(ctx, tx, req.SessionID); err != nil {
		return false, err
	}
	native, eligible, err := r.nativeRebindTargetTx(ctx, tx, req)
	if err != nil || !eligible {
		return false, err
	}
	if moved, err := r.nativeRebindMovesTokenTx(ctx, tx, req); err != nil || !moved {
		return false, err
	}
	if native == req.PreviousNativeSessionID {
		if moved, err := r.rebindGenerationNativeTx(ctx, tx, req); err != nil || !moved {
			return false, err
		}
	}
	return true, tx.Commit()
}

func completeNativeRebind(req models.HarnessNativeRebind) bool {
	return req.SessionID != "" && req.IncarnationID != "" && req.Generation > 0 && req.TaskID != "" && req.ExecutionID != "" &&
		req.PreviousNativeSessionID != "" && req.NativeSessionID != "" && req.PreviousNativeSessionID != req.NativeSessionID
}

func (r *Repository) lockNativeRebindTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if err := r.ensureExecutorRunningAvailableTx(ctx, tx, sessionID); err != nil {
		return err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{sessionID}, nil); err != nil {
		return err
	}
	if _, err := lockTaskSessionRow(ctx, tx, sessionID); err != nil {
		return err
	}
	return r.retainQuickChatHistoryTx(ctx, tx, sessionID)
}

// nativeRebindTargetTx returns the current generation's native ID when the
// requested generation is current, no restore attempt is incomplete, and the
// generation is bound to the previous or the new native conversation.
func (r *Repository) nativeRebindTargetTx(ctx context.Context, tx *sqlx.Tx, req models.HarnessNativeRebind) (string, bool, error) {
	var generation int64
	var native string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT generation, native_session_id FROM harness_session_generations
		WHERE session_id = ? AND incarnation_id = ?
		ORDER BY generation DESC LIMIT 1`), req.SessionID, req.IncarnationID).Scan(&generation, &native)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if generation != req.Generation || (native != req.PreviousNativeSessionID && native != req.NativeSessionID) {
		return "", false, nil
	}
	var restoring bool
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT EXISTS (SELECT 1 FROM session_restore_attempts WHERE session_id = ? AND completed_at IS NULL)`),
		req.SessionID).Scan(&restoring); err != nil {
		return "", false, err
	}
	return native, !restoring, nil
}

// nativeRebindMovesTokenTx stores the new token on the owning execution's row
// when that row still names the previous or the new native conversation.
func (r *Repository) nativeRebindMovesTokenTx(ctx context.Context, tx *sqlx.Tx, req models.HarnessNativeRebind) (bool, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE executors_running
		   SET resume_token = ?, last_message_uuid = '', updated_at = ?
		 WHERE session_id = ? AND task_id = ? AND agent_execution_id = ? AND resume_token IN (?, ?)`),
		req.NativeSessionID, time.Now().UTC(), req.SessionID, req.TaskID, req.ExecutionID,
		req.PreviousNativeSessionID, req.NativeSessionID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return err == nil && rows == 1, err
}

func (r *Repository) rebindGenerationNativeTx(ctx context.Context, tx *sqlx.Tx, req models.HarnessNativeRebind) (bool, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE harness_session_generations SET native_session_id = ?
		 WHERE session_id = ? AND incarnation_id = ? AND generation = ? AND native_session_id = ?`),
		req.NativeSessionID, req.SessionID, req.IncarnationID, req.Generation, req.PreviousNativeSessionID)
	if err != nil {
		return false, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return false, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO harness_native_session_rebinds
		(id, session_id, incarnation_id, generation, previous_native_session_id, native_session_id, execution_id, rebound_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		uuid.NewString(), req.SessionID, req.IncarnationID, req.Generation,
		req.PreviousNativeSessionID, req.NativeSessionID, req.ExecutionID, time.Now().UTC())
	return err == nil, err
}
