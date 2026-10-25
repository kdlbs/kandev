package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) initExecutionObservationSchema() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS agent_execution_observer (
		id INTEGER PRIMARY KEY CHECK (id=1), owner_token TEXT NOT NULL, generation BIGINT NOT NULL CHECK(generation>0));
	CREATE TABLE IF NOT EXISTS agent_execution_observations (
		attempt_id TEXT PRIMARY KEY REFERENCES agent_execution_snapshots(attempt_id) ON DELETE CASCADE,
		payload TEXT NOT NULL, revision BIGINT NOT NULL CHECK(revision>=0),
		status TEXT NOT NULL CHECK(status IN ('pending','configured','starting','active','failed','unknown','stopped')));
	CREATE INDEX IF NOT EXISTS idx_execution_observations_status ON agent_execution_observations(status,attempt_id)`)
	return err
}

// ClaimExecutionObserver fences observations from earlier backend processes.
func (r *Repository) ClaimExecutionObserver(ctx context.Context, token string) (uint64, error) {
	if token == "" || len(token) > 128 {
		return 0, errExecutionSnapshotConflict
	}
	var generation uint64
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`INSERT INTO agent_execution_observer (id,owner_token,generation) VALUES (1,?,1)
		ON CONFLICT (id) DO UPDATE SET generation=CASE WHEN agent_execution_observer.owner_token=excluded.owner_token THEN agent_execution_observer.generation ELSE agent_execution_observer.generation+1 END,
		owner_token=excluded.owner_token RETURNING generation`), token).Scan(&generation)
	return generation, err
}

func (r *Repository) RecordExecutionObservation(ctx context.Context, token string, snapshot *models.ExecutionSnapshot, observation contract.Observation) (*models.ExecutionObservation, error) {
	if !validObservationInput(snapshot, observation) {
		return nil, errExecutionSnapshotConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	generation, err := r.lockExecutionObserver(ctx, tx, token)
	if err != nil {
		return nil, err
	}
	if err = r.lockObservedSnapshot(ctx, tx, snapshot); err != nil {
		return nil, err
	}
	previous, err := readStoredObservation(ctx, tx, snapshot.AttemptID)
	if err != nil {
		return nil, err
	}
	if !observationCanAdvance(previous, observation) {
		return nil, errExecutionSnapshotConflict
	}
	value := &models.ExecutionObservation{SchemaVersion: 1, ObserverGeneration: generation, Revision: 1, SessionID: snapshot.SessionID, TaskID: snapshot.TaskID, SnapshotSequence: snapshot.Sequence, Observation: observation}
	if previous != nil {
		value.Revision = previous.Revision + 1
		if value.EvidenceAt == nil {
			value.EvidenceAt = previous.EvidenceAt
			value.Mechanism = previous.Mechanism
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_execution_observations (attempt_id,payload,revision,status) VALUES (?,?,?,?)
		ON CONFLICT(attempt_id) DO UPDATE SET payload=excluded.payload,revision=excluded.revision,status=excluded.status`), snapshot.AttemptID, string(raw), value.Revision, value.Status)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	var stored models.ExecutionObservation
	err = json.Unmarshal(raw, &stored)
	return &stored, err
}

func (r *Repository) lockExecutionObserver(ctx context.Context, tx *sqlx.Tx, token string) (uint64, error) {
	locked, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE agent_execution_observer SET owner_token=owner_token WHERE id=1 AND owner_token=?`), token)
	if err != nil {
		return 0, err
	}
	count, err := locked.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, errExecutionSnapshotConflict
	}
	var generation uint64
	err = tx.QueryRowContext(ctx, `SELECT generation FROM agent_execution_observer WHERE id=1`).Scan(&generation)
	return generation, err
}

func (r *Repository) lockObservedSnapshot(ctx context.Context, tx *sqlx.Tx, expected *models.ExecutionSnapshot) error {
	if err := r.managedSessionDeletionBarrierTx(ctx, tx, expected.SessionID); err != nil {
		return err
	}
	if err := r.lockTaskRowInTx(ctx, tx, expected.TaskID); err != nil {
		return err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), expected.SessionID); err != nil {
		return err
	}
	var raw string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT payload FROM agent_execution_snapshots WHERE session_id=? ORDER BY sequence DESC LIMIT 1`), expected.SessionID).Scan(&raw)
	if err != nil {
		return err
	}
	var actual models.ExecutionSnapshot
	if json.Unmarshal([]byte(raw), &actual) != nil || !reflect.DeepEqual(&actual, expected) {
		return errExecutionSnapshotConflict
	}
	return nil
}

func readStoredObservation(ctx context.Context, tx *sqlx.Tx, attemptID string) (*models.ExecutionObservation, error) {
	var raw string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT payload FROM agent_execution_observations WHERE attempt_id=?`), attemptID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if raw == jsonNull {
		return nil, nil
	}
	var result models.ExecutionObservation
	if json.Unmarshal([]byte(raw), &result) != nil || !result.ValidFields() || result.AttemptID != attemptID {
		return nil, errExecutionSnapshotConflict
	}
	return &result, nil
}

func observationCanAdvance(previous *models.ExecutionObservation, next contract.Observation) bool {
	if previous == nil {
		return true
	}
	if next.AttemptVersion < previous.AttemptVersion || next.CheckedAt.Before(previous.CheckedAt) {
		return false
	}
	if previous.Status == contract.StatusStopped || previous.Status == contract.StatusFailed {
		return next.Status == previous.Status && next.AttemptVersion == previous.AttemptVersion
	}
	return true
}

func decodeExecutionObservation(payload string, snapshot *models.ExecutionSnapshot, generation uint64, now time.Time) *models.ExecutionObservation {
	var value models.ExecutionObservation
	if snapshot == nil || snapshot.Sandbox == nil || generation == 0 || json.Unmarshal([]byte(payload), &value) != nil || !value.ValidFields() || value.ObserverGeneration > generation {
		return nil
	}
	if !observationMatchesSnapshot(&value, snapshot) {
		return nil
	}
	if value.ObserverGeneration != generation {
		value.ProviderHealth = contract.HealthUnknown
		if value.Status != contract.StatusStopped && value.Status != contract.StatusFailed {
			value.Status, value.Reason = contract.StatusUnknown, "ownership_unavailable"
			value.ValidUntil = nil
		}
	}
	if !value.Fresh(now) {
		value.Status, value.Reason = contract.StatusUnknown, "evidence_unavailable"
		value.ValidUntil = nil
	}
	value.ObserverGeneration = generation
	return &value
}

func validObservationInput(snapshot *models.ExecutionSnapshot, value contract.Observation) bool {
	return snapshot != nil && snapshot.Sandbox != nil && value.AttemptID == snapshot.AttemptID &&
		value.Generation == snapshot.Sandbox.Generation && value.ValidFields() && value.Fresh(time.Now())
}

func observationMatchesSnapshot(value *models.ExecutionObservation, snapshot *models.ExecutionSnapshot) bool {
	return value.SessionID == snapshot.SessionID && value.TaskID == snapshot.TaskID && value.AttemptID == snapshot.AttemptID &&
		value.SnapshotSequence == snapshot.Sequence && value.Generation == snapshot.Sandbox.Generation
}
