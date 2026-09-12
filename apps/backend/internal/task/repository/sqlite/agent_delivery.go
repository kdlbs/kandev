package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const deliveryEffectPending = models.DeliveryEffectPending

func (r *Repository) initAgentDeliverySchema() error {
	blob := dialect.BlobType(r.db.DriverName())
	_, err := r.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS agent_delivery_submissions (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			harness_generation BIGINT NOT NULL,
			owner_generation BIGINT NOT NULL,
			dispatch_attempt_id TEXT NOT NULL DEFAULT '',
			payload_hash TEXT NOT NULL,
			payload %s NOT NULL,
			state TEXT NOT NULL,
			outcome TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_agent_delivery_submissions_session
			ON agent_delivery_submissions(session_id, created_at DESC);

		CREATE TABLE IF NOT EXISTS agent_delivery_inbox (
			stream_id TEXT NOT NULL,
			sequence BIGINT NOT NULL,
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			harness_generation BIGINT NOT NULL,
			submission_id TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL,
			payload %s NOT NULL,
			terminal INTEGER NOT NULL DEFAULT 0,
			received_at TIMESTAMP NOT NULL,
			projected_at TIMESTAMP,
			PRIMARY KEY (stream_id, sequence)
		);
		CREATE INDEX IF NOT EXISTS idx_agent_delivery_inbox_unprojected
			ON agent_delivery_inbox(stream_id, projected_at, sequence);

		CREATE TABLE IF NOT EXISTS agent_delivery_cursors (
			stream_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			harness_generation BIGINT NOT NULL,
			received_sequence BIGINT NOT NULL DEFAULT 0,
			projected_sequence BIGINT NOT NULL DEFAULT 0,
			remote_high_water BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMP NOT NULL
		);

		CREATE TABLE IF NOT EXISTS agent_delivery_effects (
			effect_key TEXT PRIMARY KEY,
			stream_id TEXT NOT NULL,
			sequence BIGINT NOT NULL,
			effect_type TEXT NOT NULL,
			state TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP
		);
	`, blob, blob))
	return err
}

func (r *Repository) PrepareAgentDeliverySubmission(ctx context.Context, submission *models.AgentDeliverySubmission) (bool, error) {
	if submission == nil || submission.ID == "" || submission.PayloadHash == "" {
		return false, fmt.Errorf("submission id and payload hash are required")
	}
	if submission.State == "" {
		submission.State = models.DeliverySubmissionPrepared
	}
	if submission.CreatedAt.IsZero() {
		submission.CreatedAt = r.nowUTC()
	}
	if submission.UpdatedAt.IsZero() {
		submission.UpdatedAt = submission.CreatedAt
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var existingHash string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT payload_hash FROM agent_delivery_submissions WHERE id = ?`), submission.ID).Scan(&existingHash)
	switch err {
	case nil:
		if existingHash != submission.PayloadHash {
			return false, repoerrors.ErrAgentDeliverySubmissionConflict
		}
		return false, tx.Commit()
	case sql.ErrNoRows:
	default:
		return false, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_delivery_submissions
		(id, session_id, incarnation_id, harness_generation, owner_generation,
		 dispatch_attempt_id, payload_hash, payload, state, outcome, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		submission.ID, submission.SessionID, submission.IncarnationID,
		submission.HarnessGeneration, submission.OwnerGeneration, submission.DispatchAttemptID,
		submission.PayloadHash, submission.Payload, submission.State, submission.Outcome,
		submission.CreatedAt, submission.UpdatedAt)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) GetAgentDeliverySubmission(ctx context.Context, id string) (*models.AgentDeliverySubmission, error) {
	row := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT id, session_id, incarnation_id, harness_generation, owner_generation,
		       dispatch_attempt_id, payload_hash, payload, state, outcome, created_at, updated_at
		FROM agent_delivery_submissions WHERE id = ?`), id)
	var submission models.AgentDeliverySubmission
	if err := row.Scan(
		&submission.ID, &submission.SessionID, &submission.IncarnationID,
		&submission.HarnessGeneration, &submission.OwnerGeneration, &submission.DispatchAttemptID,
		&submission.PayloadHash, &submission.Payload, &submission.State, &submission.Outcome,
		&submission.CreatedAt, &submission.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, repoerrors.ErrAgentDeliverySubmissionNotFound
		}
		return nil, err
	}
	return &submission, nil
}

func (r *Repository) TransitionAgentDeliverySubmission(ctx context.Context, id string, from, to models.DeliverySubmissionState, outcome string, updatedAt time.Time) (bool, error) {
	if updatedAt.IsZero() {
		updatedAt = r.nowUTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_submissions
		SET state = ?, outcome = ?, updated_at = ?
		WHERE id = ? AND state = ?`), to, outcome, updatedAt, id, from)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r *Repository) ReceiveAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, remoteHighWater int64) (bool, error) {
	if event == nil || event.StreamID == "" || event.Sequence <= 0 || event.EventType == "" {
		return false, fmt.Errorf("stream, positive sequence, and event type are required")
	}
	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = r.nowUTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	inserted, err := r.receiveAgentDeliveryEventTx(ctx, tx, event, remoteHighWater)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.refreshAgentDeliveryLag(ctx)
	return inserted == 1, nil
}

func (r *Repository) receiveAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, remoteHighWater int64) (int64, error) {
	inserted, err := r.insertAgentDeliveryEventTx(ctx, tx, event)
	if err != nil {
		return 0, err
	}
	cursor, err := r.loadOrCreateAgentDeliveryCursorTx(ctx, tx, event)
	if err != nil {
		return 0, err
	}
	if remoteHighWater > cursor.RemoteHighWater {
		cursor.RemoteHighWater = remoteHighWater
	}
	if err := advanceReceivedCursorTx(ctx, tx, r.db.Rebind, event.StreamID, &cursor); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_cursors
		SET received_sequence = ?, remote_high_water = ?, updated_at = ?
		WHERE stream_id = ?`), cursor.ReceivedSequence, cursor.RemoteHighWater, event.ReceivedAt, event.StreamID); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (r *Repository) insertAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) (int64, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_delivery_inbox
		(stream_id, sequence, session_id, incarnation_id, harness_generation,
		 submission_id, event_type, payload, terminal, received_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (stream_id, sequence) DO NOTHING`),
		event.StreamID, event.Sequence, event.SessionID, event.IncarnationID,
		event.HarnessGeneration, event.SubmissionID, event.EventType, event.Payload,
		boolToInt(event.Terminal), event.ReceivedAt)
	if err != nil {
		return 0, err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted != 0 {
		return inserted, err
	}
	var stored models.AgentDeliveryEvent
	var terminal int
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id, incarnation_id, harness_generation, submission_id,
		       event_type, payload, terminal
		FROM agent_delivery_inbox
		WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence).Scan(
		&stored.SessionID, &stored.IncarnationID, &stored.HarnessGeneration,
		&stored.SubmissionID, &stored.EventType, &stored.Payload, &terminal); err != nil {
		return 0, err
	}
	stored.StreamID = event.StreamID
	stored.Sequence = event.Sequence
	stored.Terminal = terminal != 0
	if !sameAgentDeliveryEvent(&stored, event) {
		return 0, repoerrors.ErrAgentDeliveryEventConflict
	}
	return 0, nil
}

func (r *Repository) loadOrCreateAgentDeliveryCursorTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) (models.AgentDeliveryCursor, error) {
	var cursor models.AgentDeliveryCursor
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id, incarnation_id, harness_generation, received_sequence,
		       projected_sequence, remote_high_water, updated_at
		FROM agent_delivery_cursors WHERE stream_id = ?`), event.StreamID).Scan(
		&cursor.SessionID, &cursor.IncarnationID, &cursor.HarnessGeneration,
		&cursor.ReceivedSequence, &cursor.ProjectedSequence, &cursor.RemoteHighWater,
		&cursor.UpdatedAt)
	if err == nil {
		if cursor.SessionID != event.SessionID || cursor.IncarnationID != event.IncarnationID || cursor.HarnessGeneration != event.HarnessGeneration {
			return models.AgentDeliveryCursor{}, fmt.Errorf("agent delivery stream owner mismatch")
		}
		cursor.StreamID = event.StreamID
		return cursor, nil
	}
	if err != sql.ErrNoRows {
		return models.AgentDeliveryCursor{}, err
	}
	cursor = models.AgentDeliveryCursor{
		StreamID: event.StreamID, SessionID: event.SessionID, IncarnationID: event.IncarnationID,
		HarnessGeneration: event.HarnessGeneration,
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_delivery_cursors
		(stream_id, session_id, incarnation_id, harness_generation, updated_at)
		VALUES (?, ?, ?, ?, ?)`), event.StreamID, event.SessionID, event.IncarnationID,
		event.HarnessGeneration, event.ReceivedAt)
	return cursor, err
}

func advanceReceivedCursorTx(ctx context.Context, tx *sqlx.Tx, rebind func(string) string, streamID string, cursor *models.AgentDeliveryCursor) error {
	for {
		var present int
		err := tx.QueryRowxContext(ctx, rebind("SELECT 1 FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?"), streamID, cursor.ReceivedSequence+1).Scan(&present)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		cursor.ReceivedSequence++
	}
}

func (r *Repository) GetAgentDeliveryCursor(ctx context.Context, streamID string) (*models.AgentDeliveryCursor, error) {
	row := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT stream_id, session_id, incarnation_id, harness_generation,
		       received_sequence, projected_sequence, remote_high_water, updated_at
		FROM agent_delivery_cursors WHERE stream_id = ?`), streamID)
	var cursor models.AgentDeliveryCursor
	if err := row.Scan(&cursor.StreamID, &cursor.SessionID, &cursor.IncarnationID,
		&cursor.HarnessGeneration, &cursor.ReceivedSequence, &cursor.ProjectedSequence,
		&cursor.RemoteHighWater, &cursor.UpdatedAt); err != nil {
		return nil, err
	}
	return &cursor, nil
}

func (r *Repository) ListUnprojectedAgentDeliveryEvents(ctx context.Context, streamID string, limit int) ([]*models.AgentDeliveryEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT stream_id, sequence, session_id, incarnation_id, harness_generation,
		       submission_id, event_type, payload, terminal, received_at, projected_at
		FROM agent_delivery_inbox
		WHERE stream_id = ? AND projected_at IS NULL
		ORDER BY sequence LIMIT ?`), streamID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanAgentDeliveryEvents(rows)
}

func (r *Repository) ProjectAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, effect *models.AgentDeliveryEffect) (bool, error) {
	if event == nil {
		return false, fmt.Errorf("event is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	stored, projectedAt, err := r.loadAgentDeliveryEventTx(ctx, tx, event.StreamID, event.Sequence)
	if err != nil {
		return false, err
	}
	if !sameAgentDeliveryEvent(&stored, event) {
		return false, repoerrors.ErrAgentDeliveryEventConflict
	}
	if projectedAt.Valid {
		return false, tx.Commit()
	}
	if effect != nil {
		if _, err := insertDeliveryEffectTx(ctx, tx, r.db.Rebind, effect); err != nil {
			return false, err
		}
	}
	now := r.nowUTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ? AND sequence = ?`),
		now, event.StreamID, event.Sequence); err != nil {
		return false, err
	}
	if err := advanceProjectedCursorTx(ctx, tx, r.db.Rebind, event.StreamID, now); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_cursors SET updated_at = ? WHERE stream_id = ?`),
		now, event.StreamID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.refreshAgentDeliveryLag(ctx)
	return true, nil
}

func (r *Repository) loadAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, streamID string, sequence int64) (models.AgentDeliveryEvent, sql.NullTime, error) {
	var stored models.AgentDeliveryEvent
	var terminal int
	var projectedAt sql.NullTime
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id, incarnation_id, harness_generation, submission_id,
		       event_type, payload, terminal, projected_at
		FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`), streamID, sequence).Scan(
		&stored.SessionID, &stored.IncarnationID, &stored.HarnessGeneration,
		&stored.SubmissionID, &stored.EventType, &stored.Payload, &terminal, &projectedAt)
	if err == sql.ErrNoRows {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, fmt.Errorf("agent delivery event not found")
	}
	if err != nil {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, err
	}
	stored.StreamID = streamID
	stored.Sequence = sequence
	stored.Terminal = terminal != 0
	return stored, projectedAt, nil
}

func advanceProjectedCursorTx(ctx context.Context, tx *sqlx.Tx, rebind func(string) string, streamID string, now time.Time) error {
	var projected int64
	if err := tx.QueryRowxContext(ctx, rebind(`
		SELECT projected_sequence FROM agent_delivery_cursors WHERE stream_id = ?`), streamID).Scan(&projected); err != nil {
		return err
	}
	for {
		var present int
		err := tx.QueryRowxContext(ctx, rebind(`
			SELECT 1 FROM agent_delivery_inbox
			WHERE stream_id = ? AND sequence = ? AND projected_at IS NOT NULL`),
			streamID, projected+1).Scan(&present)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return err
		}
		projected++
	}
	_, err := tx.ExecContext(ctx, rebind(`
		UPDATE agent_delivery_cursors SET projected_sequence = ?, updated_at = ? WHERE stream_id = ?`),
		projected, now, streamID)
	return err
}

func (r *Repository) PutAgentDeliveryEffect(ctx context.Context, effect *models.AgentDeliveryEffect) (bool, error) {
	if effect == nil || effect.EffectKey == "" {
		return false, fmt.Errorf("effect key is required")
	}
	if effect.CreatedAt.IsZero() {
		effect.CreatedAt = r.nowUTC()
	}
	if effect.State == "" {
		effect.State = deliveryEffectPending
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	inserted, err := insertDeliveryEffectTx(ctx, tx, r.db.Rebind, effect)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return inserted, nil
}

func (r *Repository) GetAgentDeliveryEffect(ctx context.Context, effectKey string) (*models.AgentDeliveryEffect, error) {
	if effectKey == "" {
		return nil, fmt.Errorf("effect key is required")
	}
	var effect models.AgentDeliveryEffect
	var completedAt sql.NullTime
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT effect_key, stream_id, sequence, effect_type, state, created_at, completed_at
		FROM agent_delivery_effects WHERE effect_key = ?`), effectKey).Scan(
		&effect.EffectKey, &effect.StreamID, &effect.Sequence, &effect.EffectType,
		&effect.State, &effect.CreatedAt, &completedAt)
	if err == sql.ErrNoRows {
		return nil, repoerrors.ErrAgentDeliveryEffectNotFound
	}
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		t := completedAt.Time
		effect.CompletedAt = &t
	}
	return &effect, nil
}

type deliveryEffectTx interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func insertDeliveryEffectTx(ctx context.Context, tx deliveryEffectTx, rebind func(string) string, effect *models.AgentDeliveryEffect) (bool, error) {
	if effect.CreatedAt.IsZero() {
		effect.CreatedAt = time.Now().UTC()
	}
	if effect.State == "" {
		effect.State = models.DeliveryEffectPending
	}
	result, err := tx.ExecContext(ctx, rebind(`
		INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (effect_key) DO NOTHING`), effect.EffectKey, effect.StreamID,
		effect.Sequence, effect.EffectType, effect.State, effect.CreatedAt, effect.CompletedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 1 {
		return true, nil
	}
	return reconcileExistingDeliveryEffect(ctx, tx, rebind, effect)
}

// completeDeliveryEffectTx claims a pending intent at the same transaction as
// its authoritative consumer transition. A completed effect suppresses a
// replay; a pending effect remains eligible for exactly one consumer to claim.
func completeDeliveryEffectTx(ctx context.Context, tx deliveryEffectTx, rebind func(string) string, effect *models.AgentDeliveryEffect) (bool, error) {
	if effect == nil || effect.EffectKey == "" {
		return false, fmt.Errorf("effect key is required")
	}
	now := time.Now().UTC()
	if effect.CreatedAt.IsZero() {
		effect.CreatedAt = now
	}
	result, err := tx.ExecContext(ctx, rebind(`
		UPDATE agent_delivery_effects
		SET state = ?, completed_at = ?
		WHERE effect_key = ? AND effect_type = ? AND state = ?`),
		models.DeliveryEffectCompleted, now, effect.EffectKey, effect.EffectType,
		models.DeliveryEffectPending)
	if err != nil {
		return false, err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
		return false, rowsErr
	} else if affected == 1 {
		return true, nil
	}
	completed := *effect
	completed.State = models.DeliveryEffectCompleted
	completed.CompletedAt = &now
	inserted, err := insertDeliveryEffectTx(ctx, tx, rebind, &completed)
	if err != nil {
		return false, err
	}
	if inserted {
		return true, nil
	}
	var effectType string
	if err := tx.QueryRowContext(ctx, rebind(`
		SELECT effect_type FROM agent_delivery_effects WHERE effect_key = ?`),
		effect.EffectKey).Scan(&effectType); err != nil {
		return false, err
	}
	if effectType != effect.EffectType {
		return false, repoerrors.ErrAgentDeliveryEffectConflict
	}
	return false, nil
}

func reconcileExistingDeliveryEffect(ctx context.Context, tx deliveryEffectTx, rebind func(string) string, effect *models.AgentDeliveryEffect) (bool, error) {
	var existing struct {
		StreamID   string
		Sequence   int64
		EffectType string
		State      string
	}
	if err := tx.QueryRowContext(ctx, rebind(`
		SELECT stream_id, sequence, effect_type, state
		FROM agent_delivery_effects WHERE effect_key = ?`), effect.EffectKey).Scan(
		&existing.StreamID, &existing.Sequence, &existing.EffectType, &existing.State); err != nil {
		return false, err
	}
	if existing.StreamID == "" && existing.Sequence == 0 && effect.StreamID != "" && effect.Sequence > 0 &&
		existing.EffectType == effect.EffectType && existing.State == models.DeliveryEffectCompleted {
		if _, err := tx.ExecContext(ctx, rebind(`
			UPDATE agent_delivery_effects
			SET stream_id = ?, sequence = ?
			WHERE effect_key = ? AND stream_id = '' AND sequence = 0`),
			effect.StreamID, effect.Sequence, effect.EffectKey); err != nil {
			return false, err
		}
		return false, nil
	}
	if existing.StreamID != effect.StreamID || existing.Sequence != effect.Sequence || existing.EffectType != effect.EffectType {
		return false, repoerrors.ErrAgentDeliveryEffectConflict
	}
	return false, nil
}

func sameAgentDeliveryEvent(stored, incoming *models.AgentDeliveryEvent) bool {
	return stored.StreamID == incoming.StreamID &&
		stored.Sequence == incoming.Sequence &&
		stored.SessionID == incoming.SessionID &&
		stored.IncarnationID == incoming.IncarnationID &&
		stored.HarnessGeneration == incoming.HarnessGeneration &&
		stored.SubmissionID == incoming.SubmissionID &&
		stored.EventType == incoming.EventType &&
		stored.Terminal == incoming.Terminal &&
		bytes.Equal(stored.Payload, incoming.Payload)
}

type deliveryEventRows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}

func scanAgentDeliveryEvents(rows deliveryEventRows) ([]*models.AgentDeliveryEvent, error) {
	var events []*models.AgentDeliveryEvent
	for rows.Next() {
		var event models.AgentDeliveryEvent
		var terminal int
		var projected sql.NullTime
		if err := rows.Scan(&event.StreamID, &event.Sequence, &event.SessionID,
			&event.IncarnationID, &event.HarnessGeneration, &event.SubmissionID,
			&event.EventType, &event.Payload, &terminal, &event.ReceivedAt, &projected); err != nil {
			return nil, err
		}
		event.Terminal = terminal != 0
		if projected.Valid {
			t := projected.Time
			event.ProjectedAt = &t
		}
		events = append(events, &event)
	}
	return events, rows.Err()
}
