package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
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
			canonical_message_id TEXT NOT NULL DEFAULT '',
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
			message_segment_boundary_sequence BIGINT CHECK (message_segment_boundary_sequence IS NULL OR message_segment_boundary_sequence >= 0),
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
			completed_at TIMESTAMP,
			session_id TEXT NOT NULL DEFAULT '',
			incarnation_id TEXT NOT NULL DEFAULT '',
			harness_generation BIGINT NOT NULL DEFAULT 0,
			submission_id TEXT NOT NULL DEFAULT '',
			turn_id TEXT NOT NULL DEFAULT '',
			outcome TEXT NOT NULL DEFAULT ''
		);
	`, blob, blob))
	if err != nil {
		return err
	}
	return r.initAgentDeliveryRetentionSchema()
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
	if submission.PayloadBytes == 0 {
		submission.PayloadBytes = int64(len(submission.Payload))
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), submission.SessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{submission.SessionID}, nil); err != nil {
		return false, err
	}
	if err := r.ensureAgentDeliverySessionNotRetired(ctx, tx, submission.SessionID, submission.IncarnationID); err != nil {
		return false, err
	}
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
		 dispatch_attempt_id, payload_hash, payload, state, outcome, payload_bytes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		submission.ID, submission.SessionID, submission.IncarnationID,
		submission.HarnessGeneration, submission.OwnerGeneration, submission.DispatchAttemptID,
		submission.PayloadHash, submission.Payload, submission.State, submission.Outcome, submission.PayloadBytes,
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
		       dispatch_attempt_id, payload_hash, payload, state, outcome, payload_pruned_at,
		       payload_bytes, fully_settled_at, terminal_stream_id, terminal_sequence,
		       terminal_turn_id, created_at, updated_at
		FROM agent_delivery_submissions WHERE id = ?`), id)
	var submission models.AgentDeliverySubmission
	var payloadPrunedAt, fullySettledAt sql.NullTime
	if err := row.Scan(
		&submission.ID, &submission.SessionID, &submission.IncarnationID,
		&submission.HarnessGeneration, &submission.OwnerGeneration, &submission.DispatchAttemptID,
		&submission.PayloadHash, &submission.Payload, &submission.State, &submission.Outcome,
		&payloadPrunedAt, &submission.PayloadBytes, &fullySettledAt, &submission.TerminalStreamID,
		&submission.TerminalSequence, &submission.TerminalTurnID,
		&submission.CreatedAt, &submission.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, repoerrors.ErrAgentDeliverySubmissionNotFound
		}
		return nil, err
	}
	if payloadPrunedAt.Valid {
		at := payloadPrunedAt.Time
		submission.PayloadPrunedAt = &at
	}
	if fullySettledAt.Valid {
		at := fullySettledAt.Time
		submission.FullySettledAt = &at
	}
	if submission.PayloadPrunedAt != nil {
		return nil, fmt.Errorf("submission %q: %w", id, repoerrors.ErrAgentDeliveryPayloadPruned)
	}
	return &submission, nil
}

func (r *Repository) TransitionAgentDeliverySubmission(ctx context.Context, id string, from, to models.DeliverySubmissionState, outcome string, updatedAt time.Time) (bool, error) {
	if updatedAt.IsZero() {
		updatedAt = r.nowUTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var sessionID string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id FROM agent_delivery_submissions WHERE id = ?`), id).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return false, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{sessionID}, nil); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_submissions
		SET state = ?, outcome = ?, updated_at = ?
		WHERE id = ? AND state = ?`), to, outcome, updatedAt, id, from)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return count == 1, nil
}

func (r *Repository) ReceiveAgentDeliveryEvent(ctx context.Context, event *models.AgentDeliveryEvent, remoteHighWater int64) (bool, error) {
	if event == nil || event.StreamID == "" || event.Sequence <= 0 || event.EventType == "" ||
		event.SessionID == "" || event.IncarnationID == "" || event.HarnessGeneration <= 0 {
		return false, fmt.Errorf("stream, positive sequence, event type, and owner identity are required")
	}
	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = r.nowUTC()
	}
	contentHash := agentDeliveryEventHash(event)
	event.ContentHash = contentHash
	event.HashVersion = agentDeliveryPayloadHashVersion
	event.PayloadBytes = int64(len(event.Payload))
	if err := r.prepareAgentDeliveryOwnerStatement(ctx, event); err != nil {
		return false, err
	}
	if err := r.prepareAgentDeliveryReceiptStatement(ctx); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockAgentDeliveryEventWritesMode(ctx, tx, r.db.DriverName(), []string{event.SessionID}, []string{event.StreamID}, !event.Terminal); err != nil {
		return false, err
	}
	inserted, err := r.receiveAgentDeliveryEventTx(ctx, tx, event, remoteHighWater, contentHash)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.refreshAgentDeliveryLag(ctx)
	return inserted == 1, nil
}

func (r *Repository) receiveAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, remoteHighWater int64, contentHash string) (int64, error) {
	if err := r.validateAgentDeliveryEventAdmissionTx(ctx, tx, event); err != nil {
		return 0, err
	}
	cursor, cursorExists, err := r.loadAgentDeliveryCursorTx(ctx, tx, event.StreamID)
	if err != nil {
		return 0, err
	}
	if cursorExists {
		if err := validateAgentDeliveryCursorEvent(cursor, event); err != nil {
			return 0, err
		}
	}
	found, err := r.validateAgentDeliveryEventSequenceTx(ctx, tx, event, cursor, cursorExists)
	if err != nil {
		return 0, err
	}
	var inserted int64
	if !found {
		inserted, err = r.insertAgentDeliveryEventTx(ctx, tx, event, contentHash)
		if err != nil {
			return 0, err
		}
	}
	if !cursorExists {
		cursor = models.AgentDeliveryCursor{
			StreamID: event.StreamID, SessionID: event.SessionID,
			IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration,
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO agent_delivery_cursors
			(stream_id, session_id, incarnation_id, harness_generation, message_segment_boundary_sequence, updated_at)
			VALUES (?, ?, ?, ?, 0, ?)`), event.StreamID, event.SessionID, event.IncarnationID,
			event.HarnessGeneration, event.ReceivedAt); err != nil {
			return 0, err
		}
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

func validateAgentDeliveryCursorEvent(cursor models.AgentDeliveryCursor, event *models.AgentDeliveryEvent) error {
	if cursor.SessionID != event.SessionID || cursor.IncarnationID != event.IncarnationID ||
		cursor.HarnessGeneration != event.HarnessGeneration {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	if event.Sequence <= cursor.RetiredSequence {
		return repoerrors.ErrAgentDeliveryReplayExpired
	}
	return nil
}

func (r *Repository) validateAgentDeliveryEventSequenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	event *models.AgentDeliveryEvent,
	cursor models.AgentDeliveryCursor,
	cursorExists bool,
) (bool, error) {
	// A future sequence can be inserted under the stream lock. A conflicting
	// receipt is still independently validated by the insert conflict path.
	if cursorExists && event.Sequence > cursor.ReceivedSequence {
		return false, nil
	}
	stored, _, found, err := r.findAgentDeliveryEventTx(ctx, tx, event.StreamID, event.Sequence)
	if err != nil {
		return false, err
	}
	if found {
		return true, validateStoredAgentDeliveryEvent(stored, event, cursorExists)
	}
	return false, r.validateMissingAgentDeliveryEventSequenceTx(ctx, tx, event, cursor, cursorExists)
}

func validateStoredAgentDeliveryEvent(
	stored models.AgentDeliveryEvent,
	incoming *models.AgentDeliveryEvent,
	cursorExists bool,
) error {
	if !cursorExists {
		return repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	matches, err := sameAgentDeliveryEvent(&stored, incoming)
	if err != nil {
		return err
	}
	if !matches {
		return repoerrors.ErrAgentDeliveryEventConflict
	}
	return nil
}

func (r *Repository) validateMissingAgentDeliveryEventSequenceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	event *models.AgentDeliveryEvent,
	cursor models.AgentDeliveryCursor,
	cursorExists bool,
) error {
	if cursorExists {
		if event.Sequence <= cursor.ReceivedSequence {
			return repoerrors.ErrAgentDeliveryCheckpointMissing
		}
		return nil
	}
	hasRows, err := r.agentDeliveryStreamHasRowsTx(ctx, tx, event.StreamID)
	if err != nil {
		return err
	}
	if hasRows {
		return repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	return nil
}

func (r *Repository) insertAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, contentHash string) (int64, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_delivery_inbox
		(stream_id, sequence, session_id, incarnation_id, harness_generation,
		 submission_id, event_type, payload, terminal, received_at, content_hash, hash_version, payload_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (stream_id, sequence) DO NOTHING`),
		event.StreamID, event.Sequence, event.SessionID, event.IncarnationID,
		event.HarnessGeneration, event.SubmissionID, event.EventType, event.Payload,
		boolToInt(event.Terminal), event.ReceivedAt, contentHash,
		agentDeliveryPayloadHashVersion, len(event.Payload))
	if err != nil {
		return 0, err
	}
	event.ContentHash = contentHash
	event.HashVersion = agentDeliveryPayloadHashVersion
	event.PayloadBytes = int64(len(event.Payload))
	inserted, err := result.RowsAffected()
	if err != nil || inserted != 0 {
		return inserted, err
	}
	stored, _, found, err := r.findAgentDeliveryEventTx(ctx, tx, event.StreamID, event.Sequence)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	matches, err := sameAgentDeliveryEvent(&stored, event)
	if err != nil {
		return 0, err
	}
	if !matches {
		return 0, repoerrors.ErrAgentDeliveryEventConflict
	}
	return 0, nil
}

func (r *Repository) loadAgentDeliveryCursorTx(ctx context.Context, tx *sqlx.Tx, streamID string) (models.AgentDeliveryCursor, bool, error) {
	var cursor models.AgentDeliveryCursor
	var boundary sql.NullInt64
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session_id, incarnation_id, harness_generation, received_sequence,
		       projected_sequence, message_segment_boundary_sequence, remote_high_water, retired_sequence, updated_at
		FROM agent_delivery_cursors WHERE stream_id = ?`), streamID).Scan(
		&cursor.SessionID, &cursor.IncarnationID, &cursor.HarnessGeneration,
		&cursor.ReceivedSequence, &cursor.ProjectedSequence, &boundary, &cursor.RemoteHighWater,
		&cursor.RetiredSequence, &cursor.UpdatedAt)
	if err == nil {
		cursor.StreamID = streamID
		if boundary.Valid {
			cursor.MessageSegmentBoundarySequence = int64Pointer(boundary.Int64)
		}
		return cursor, true, nil
	}
	if err != sql.ErrNoRows {
		return models.AgentDeliveryCursor{}, false, err
	}
	return models.AgentDeliveryCursor{}, false, nil
}

func int64Pointer(value int64) *int64 {
	return &value
}

func (r *Repository) agentDeliveryStreamHasRowsTx(ctx context.Context, tx *sqlx.Tx, streamID string) (bool, error) {
	var exists bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT EXISTS (SELECT 1 FROM agent_delivery_inbox WHERE stream_id = ?)`), streamID).Scan(&exists)
	return exists, err
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
		       received_sequence, projected_sequence, message_segment_boundary_sequence,
		       remote_high_water, retired_sequence, updated_at
		FROM agent_delivery_cursors WHERE stream_id = ?`), streamID)
	var cursor models.AgentDeliveryCursor
	var boundary sql.NullInt64
	if err := row.Scan(&cursor.StreamID, &cursor.SessionID, &cursor.IncarnationID,
		&cursor.HarnessGeneration, &cursor.ReceivedSequence, &cursor.ProjectedSequence,
		&boundary, &cursor.RemoteHighWater, &cursor.RetiredSequence, &cursor.UpdatedAt); err != nil {
		return nil, err
	}
	if boundary.Valid {
		cursor.MessageSegmentBoundarySequence = int64Pointer(boundary.Int64)
	}
	return &cursor, nil
}

func (r *Repository) ListUnprojectedAgentDeliveryEvents(ctx context.Context, streamID string, limit int) ([]*models.AgentDeliveryEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT stream_id, sequence, session_id, incarnation_id, harness_generation,
		       submission_id, event_type, payload, terminal, received_at, projected_at,
		       content_hash, hash_version, payload_pruned_at, payload_bytes, canonical_message_id
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
	if err := r.prepareAgentDeliveryReceiptStatement(ctx); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), event.SessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{event.SessionID}, []string{event.StreamID}); err != nil {
		return false, err
	}
	stored, projectedAt, err := r.loadAgentDeliveryEventTx(ctx, tx, event.StreamID, event.Sequence)
	if err != nil {
		return false, err
	}
	matches, err := sameAgentDeliveryEvent(&stored, event)
	if err != nil {
		return false, err
	}
	if !matches {
		return false, repoerrors.ErrAgentDeliveryEventConflict
	}
	terminalEffect, err := deliveryTerminalSettlementEffect(&stored)
	if err != nil {
		return false, err
	}
	if projectedAt.Valid {
		return false, commitProjectedTerminalEffect(ctx, tx, r.db.Rebind, terminalEffect)
	}
	var projectedSequence int64
	var boundary sql.NullInt64
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT projected_sequence, message_segment_boundary_sequence
		FROM agent_delivery_cursors WHERE stream_id = ?`), event.StreamID).Scan(&projectedSequence, &boundary); err != nil {
		return false, err
	}
	boundarySequence := projectedSequence
	if boundary.Valid {
		boundarySequence = boundary.Int64
	}
	topLevelToolStart := false
	if stored.EventType == streams.EventTypeToolCall {
		var agentEvent streams.AgentEvent
		if err := json.Unmarshal(stored.Payload, &agentEvent); err != nil {
			return false, fmt.Errorf("decode agent tool boundary event: %w", err)
		}
		topLevelToolStart = agentEvent.Type == streams.EventTypeToolCall && agentEvent.ParentToolCallID == ""
	}
	if topLevelToolStart {
		if stored.Sequence != projectedSequence+1 {
			return false, fmt.Errorf("top-level tool boundary sequence gap: expected %d, got %d", projectedSequence+1, stored.Sequence)
		}
		boundarySequence = stored.Sequence
	}
	if err := insertProjectionEffects(ctx, tx, r.db.Rebind, effect, terminalEffect); err != nil {
		return false, err
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
		UPDATE agent_delivery_cursors
		SET message_segment_boundary_sequence = ?, updated_at = ?
		WHERE stream_id = ?`), boundarySequence, now, event.StreamID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.refreshAgentDeliveryLag(ctx)
	return true, nil
}

func commitProjectedTerminalEffect(
	ctx context.Context, tx *sqlx.Tx, rebind func(string) string, terminal *models.AgentDeliveryEffect,
) error {
	if terminal != nil {
		if _, err := insertDeliveryEffectTx(ctx, tx, rebind, terminal); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertProjectionEffects(
	ctx context.Context, tx *sqlx.Tx, rebind func(string) string,
	effect, terminal *models.AgentDeliveryEffect,
) error {
	for _, candidate := range []*models.AgentDeliveryEffect{effect, terminal} {
		if candidate == nil {
			continue
		}
		if _, err := insertDeliveryEffectTx(ctx, tx, rebind, candidate); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) findAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, streamID string, sequence int64) (models.AgentDeliveryEvent, sql.NullTime, bool, error) {
	var stored models.AgentDeliveryEvent
	var terminal int
	var projectedAt, payloadPrunedAt sql.NullTime
	statement, err := r.agentDeliveryReceiptStatement()
	if err != nil {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, false, err
	}
	var row *sqlx.Row
	if statement == nil {
		row = tx.QueryRowxContext(ctx, r.db.Rebind(agentDeliveryReceiptQuery), streamID, sequence)
	} else {
		bound := tx.StmtxContext(ctx, statement)
		defer func() { _ = bound.Close() }()
		row = bound.QueryRowxContext(ctx, streamID, sequence)
	}
	err = row.Scan(
		&stored.SessionID, &stored.IncarnationID, &stored.HarnessGeneration,
		&stored.SubmissionID, &stored.EventType, &stored.Payload, &terminal, &projectedAt,
		&stored.ContentHash, &stored.HashVersion, &payloadPrunedAt, &stored.PayloadBytes,
		&stored.CanonicalMessageID)
	if err == sql.ErrNoRows {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, false, nil
	}
	if err != nil {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, false, err
	}
	stored.StreamID = streamID
	stored.Sequence = sequence
	stored.Terminal = terminal != 0
	if payloadPrunedAt.Valid {
		at := payloadPrunedAt.Time
		stored.PayloadPrunedAt = &at
	}
	return stored, projectedAt, true, nil
}

func (r *Repository) loadAgentDeliveryEventTx(ctx context.Context, tx *sqlx.Tx, streamID string, sequence int64) (models.AgentDeliveryEvent, sql.NullTime, error) {
	stored, projectedAt, found, err := r.findAgentDeliveryEventTx(ctx, tx, streamID, sequence)
	if err != nil {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, err
	}
	if found {
		return stored, projectedAt, nil
	}
	cursor, exists, err := r.loadAgentDeliveryCursorTx(ctx, tx, streamID)
	if err != nil {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, err
	}
	if !exists {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	if sequence <= cursor.RetiredSequence {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, repoerrors.ErrAgentDeliveryReplayExpired
	}
	if sequence <= cursor.ProjectedSequence || sequence <= cursor.ReceivedSequence {
		return models.AgentDeliveryEvent{}, sql.NullTime{}, repoerrors.ErrAgentDeliveryCheckpointMissing
	}
	return models.AgentDeliveryEvent{}, sql.NullTime{}, fmt.Errorf("agent delivery event not found")
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
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), effect.SessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{effect.SessionID}, []string{effect.StreamID}); err != nil {
		return false, err
	}
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
		SELECT effect_key, stream_id, sequence, effect_type, state, created_at, completed_at,
		       session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome
		FROM agent_delivery_effects WHERE effect_key = ?`), effectKey).Scan(
		&effect.EffectKey, &effect.StreamID, &effect.Sequence, &effect.EffectType,
		&effect.State, &effect.CreatedAt, &completedAt, &effect.SessionID, &effect.IncarnationID,
		&effect.HarnessGeneration, &effect.SubmissionID, &effect.TurnID, &effect.Outcome)
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
		(effect_key, stream_id, sequence, effect_type, state, created_at, completed_at,
		 session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (effect_key) DO NOTHING`), effect.EffectKey, effect.StreamID,
		effect.Sequence, effect.EffectType, effect.State, effect.CreatedAt, effect.CompletedAt,
		effect.SessionID, effect.IncarnationID, effect.HarnessGeneration, effect.SubmissionID,
		effect.TurnID, effect.Outcome)
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

func reconcileExistingDeliveryEffect(ctx context.Context, tx deliveryEffectTx, rebind func(string) string, effect *models.AgentDeliveryEffect) (bool, error) {
	var existing models.AgentDeliveryEffect
	if err := tx.QueryRowContext(ctx, rebind(`
		SELECT stream_id, sequence, effect_type, state, session_id, incarnation_id,
		       harness_generation, submission_id, turn_id, outcome
		FROM agent_delivery_effects WHERE effect_key = ?`), effect.EffectKey).Scan(
		&existing.StreamID, &existing.Sequence, &existing.EffectType, &existing.State,
		&existing.SessionID, &existing.IncarnationID, &existing.HarnessGeneration,
		&existing.SubmissionID, &existing.TurnID, &existing.Outcome); err != nil {
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
	if !sameAgentDeliveryEffect(&existing, effect) {
		return false, repoerrors.ErrAgentDeliveryEffectConflict
	}
	return false, nil
}

func sameAgentDeliveryEffect(existing, incoming *models.AgentDeliveryEffect) bool {
	return existing.StreamID == incoming.StreamID && existing.Sequence == incoming.Sequence &&
		existing.EffectType == incoming.EffectType && existing.SessionID == incoming.SessionID &&
		existing.IncarnationID == incoming.IncarnationID &&
		existing.HarnessGeneration == incoming.HarnessGeneration && existing.SubmissionID == incoming.SubmissionID &&
		existing.TurnID == incoming.TurnID && existing.Outcome == incoming.Outcome
}

func sameAgentDeliveryEvent(stored, incoming *models.AgentDeliveryEvent) (bool, error) {
	if !sameAgentDeliveryEventIdentity(stored, incoming) {
		return false, nil
	}
	if stored.HashVersion > agentDeliveryPayloadHashVersion {
		return false, fmt.Errorf("hash version %d: %w", stored.HashVersion, repoerrors.ErrAgentDeliveryHashVersion)
	}
	return sameAgentDeliveryEventPayload(stored, incoming)
}

func sameAgentDeliveryEventIdentity(stored, incoming *models.AgentDeliveryEvent) bool {
	return stored.StreamID == incoming.StreamID && stored.Sequence == incoming.Sequence &&
		stored.SessionID == incoming.SessionID && stored.IncarnationID == incoming.IncarnationID &&
		stored.HarnessGeneration == incoming.HarnessGeneration && stored.SubmissionID == incoming.SubmissionID &&
		stored.EventType == incoming.EventType && stored.Terminal == incoming.Terminal
}

func sameAgentDeliveryEventPayload(stored, incoming *models.AgentDeliveryEvent) (bool, error) {
	if stored.PayloadPrunedAt != nil {
		if stored.HashVersion != agentDeliveryPayloadHashVersion || stored.ContentHash == "" {
			return false, repoerrors.ErrAgentDeliveryReceiptCorrupt
		}
		return stored.ContentHash == agentDeliveryEventHash(incoming), nil
	}
	if stored.HashVersion == agentDeliveryPayloadHashVersion {
		if stored.ContentHash == "" || stored.ContentHash != agentDeliveryEventHash(stored) {
			return false, repoerrors.ErrAgentDeliveryReceiptCorrupt
		}
		if bytes.Equal(stored.Payload, incoming.Payload) {
			return true, nil
		}
		return stored.ContentHash == agentDeliveryEventHash(incoming), nil
	}
	return sameAgentDeliveryPayload(stored.Payload, incoming.Payload), nil
}

// sameAgentDeliveryPayload compares the transport event content while
// tolerating lifecycle fields that are reconstructed by the backend consumer.
// A replayed agentctl event can be delivered to a fresh lifecycle execution;
// its current turn, control-server outcome stamp, and canonical message IDs
// are not part of the retained transport identity and can therefore differ
// from the original projection.
// All adapter-owned content remains strict, so a changed event at the same
// stream sequence is still rejected.
func sameAgentDeliveryPayload(stored, incoming []byte) bool {
	if bytes.Equal(stored, incoming) {
		return true
	}
	storedMode, storedPayload := normalizeAgentDeliveryPayload(stored)
	incomingMode, incomingPayload := normalizeAgentDeliveryPayload(incoming)
	return storedMode == incomingMode && bytes.Equal(storedPayload, incomingPayload)
}

func stripLifecycleDeliveryFields(event *streams.AgentEvent) {
	event.ControlTurnID = 0
	event.TurnID = ""
	event.CanonicalMessageID = ""
	event.CanonicalMessageBaseID = ""
	event.CanonicalProjection = false
	event.CanonicalMessageAppend = false
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
		var payloadPrunedAt sql.NullTime
		if err := rows.Scan(&event.StreamID, &event.Sequence, &event.SessionID,
			&event.IncarnationID, &event.HarnessGeneration, &event.SubmissionID,
			&event.EventType, &event.Payload, &terminal, &event.ReceivedAt, &projected,
			&event.ContentHash, &event.HashVersion, &payloadPrunedAt, &event.PayloadBytes,
			&event.CanonicalMessageID); err != nil {
			return nil, err
		}
		event.Terminal = terminal != 0
		if projected.Valid {
			t := projected.Time
			event.ProjectedAt = &t
		}
		if payloadPrunedAt.Valid {
			t := payloadPrunedAt.Time
			event.PayloadPrunedAt = &t
		}
		events = append(events, &event)
	}
	return events, rows.Err()
}
