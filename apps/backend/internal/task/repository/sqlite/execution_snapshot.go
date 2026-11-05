package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

var errExecutionSnapshotConflict = errors.New("execution snapshot identity changed")

func (r *Repository) initExecutionSnapshotSchema() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS agent_execution_snapshots (
		attempt_id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES task_sessions(id) ON DELETE CASCADE,
		execution_id TEXT NOT NULL,
		sequence BIGINT NOT NULL CHECK (sequence > 0),
		generation BIGINT NOT NULL CHECK (generation > 0),
		payload TEXT NOT NULL,
		UNIQUE (session_id, sequence),
		UNIQUE (session_id, execution_id, generation)
	)`)
	if err != nil {
		return err
	}
	return r.initExecutionObservationSchema()
}

// CaptureExecutionSnapshot freezes public launch metadata under the same locks
// that protect replacement of the session's current execution.
func (r *Repository) CaptureExecutionSnapshot(ctx context.Context, input models.ExecutionSnapshot) (*models.ExecutionSnapshot, error) {
	if !input.ValidPublicFields() || input.Generation != 0 || input.Sequence != 0 || !input.CapturedAt.IsZero() {
		return nil, errExecutionSnapshotConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = r.lockExecutionSnapshot(ctx, tx, &input); err != nil {
		return nil, err
	}
	var payload string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT payload FROM agent_execution_snapshots WHERE attempt_id = ?`), input.AttemptID).Scan(&payload)
	if err == nil {
		return replayExecutionSnapshot(payload, input)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(MAX(sequence), 0) + 1,
		COALESCE(MAX(CASE WHEN execution_id = ? THEN generation ELSE 0 END), 0) + 1
		FROM agent_execution_snapshots WHERE session_id = ?`), input.ExecutionID, input.SessionID).Scan(&input.Sequence, &input.Generation); err != nil {
		return nil, err
	}
	input.CapturedAt = time.Now().UTC()
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO agent_execution_snapshots
		(attempt_id, session_id, execution_id, sequence, generation, payload) VALUES (?, ?, ?, ?, ?, ?)`),
		input.AttemptID, input.SessionID, input.ExecutionID, input.Sequence, input.Generation, string(raw)); err != nil {
		return nil, err
	}
	if input.Sandbox != nil {
		if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_execution_observations (attempt_id,payload,revision,status) VALUES (?,'null',0,'pending')`), input.AttemptID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	var result models.ExecutionSnapshot
	err = json.Unmarshal(raw, &result)
	return &result, err
}

func (r *Repository) lockExecutionSnapshot(ctx context.Context, tx *sqlx.Tx, input *models.ExecutionSnapshot) error {
	if err := r.managedSessionDeletionBarrierTx(ctx, tx, input.SessionID); err != nil {
		return err
	}
	if err := r.managedDeletionBarrierTx(ctx, tx, input.TaskID); err != nil {
		return err
	}
	if err := r.lockTaskRowInTx(ctx, tx, input.TaskID); err != nil {
		return err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), input.SessionID); err != nil {
		return err
	}
	var executorID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT er.executor_id FROM executors_running er
		JOIN task_sessions s ON s.id = er.session_id AND s.task_id = er.task_id
		WHERE er.session_id = ? AND er.task_id = ? AND er.agent_execution_id = ?`),
		input.SessionID, input.TaskID, input.ExecutionID).Scan(&executorID)
	if errors.Is(err, sql.ErrNoRows) {
		return errExecutionSnapshotConflict
	}
	if err != nil {
		return err
	}
	input.ExecutorID = executorID
	return nil
}

func replayExecutionSnapshot(payload string, input models.ExecutionSnapshot) (*models.ExecutionSnapshot, error) {
	var stored models.ExecutionSnapshot
	if err := json.Unmarshal([]byte(payload), &stored); err != nil {
		return nil, err
	}
	input.Generation, input.Sequence, input.CapturedAt = stored.Generation, stored.Sequence, stored.CapturedAt
	if !reflect.DeepEqual(input, stored) {
		return nil, errExecutionSnapshotConflict
	}
	return &stored, nil
}

func decodeExecutionSnapshot(payload string, session *models.TaskSession) *models.ExecutionSnapshot {
	var snapshot models.ExecutionSnapshot
	if json.Unmarshal([]byte(payload), &snapshot) != nil || !snapshot.ValidPublicFields() ||
		snapshot.SessionID != session.ID || snapshot.TaskID != session.TaskID ||
		snapshot.Sequence == 0 || snapshot.Generation == 0 || snapshot.CapturedAt.IsZero() {
		return nil
	}
	return &snapshot
}
