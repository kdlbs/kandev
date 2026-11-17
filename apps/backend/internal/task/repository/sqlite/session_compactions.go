package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// copilotNativeCompactionProvider is the canonical registry identity persisted
// with Copilot receipts. Projection validates it rather than trusting a
// provider label supplied by the wire.
const copilotNativeCompactionProvider = "copilot-acp"

// projectContextCompactionGroupTx applies one or more native compaction
// observations inside the canonical delivery transaction. The group is always
// single-type and non-content, so each event owns its own result slot.
func (r *Repository) projectContextCompactionGroupTx(
	ctx context.Context,
	tx *sqlx.Tx,
	group []canonicalDeliveryBatchEntry,
	results []models.CanonicalAgentDeliveryProjectionResult,
) error {
	for _, item := range group {
		count, reporting, err := r.projectContextCompactionObservationTx(
			ctx, tx, item.stored, item.agentEvent.ContextCompaction,
		)
		if err != nil {
			return err
		}
		results[item.index].ContextCompactionCount = count
		results[item.index].ContextCompactionReporting = reporting
	}
	return nil
}

// projectContextCompactionObservationTx commits the receipt, lifetime count,
// and reporting metadata together under the canonical transaction. It returns
// the committed count and reporting so post-commit publication can describe
// exactly what is stored.
func (r *Repository) projectContextCompactionObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	stored models.AgentDeliveryEvent,
	observation *streams.ContextCompactionObservation,
) (*int64, *streams.ContextCompactionReporting, error) {
	if observation == nil {
		return nil, nil, nil
	}
	if observation.Provider != copilotNativeCompactionProvider {
		return nil, nil, nil
	}
	if stored.SessionID == "" || stored.HarnessGeneration <= 0 {
		return nil, nil, nil
	}
	if err := r.validateCompactionNativeIdentityTx(
		ctx, tx, stored.SessionID, stored.IncarnationID, stored.HarnessGeneration, observation.NativeSessionID,
	); err != nil {
		if errors.Is(err, errCompactionNativeIdentity) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if err := lockSessionCompactionWrites(ctx, tx, r.db.DriverName(), stored.SessionID); err != nil {
		return nil, nil, err
	}
	if _, err := lockTaskSessionRow(ctx, tx, stored.SessionID); err != nil {
		return nil, nil, err
	}
	count, reporting, err := r.readSessionCompactionMetadataTx(ctx, tx, stored.SessionID)
	if err != nil {
		return nil, nil, err
	}
	currentGeneration, err := r.isCurrentCompactionGenerationTx(ctx, tx, stored, observation.NativeSessionID)
	if err != nil {
		return nil, nil, err
	}

	updatedCount, updatedReporting, changed, err := r.applyCompactionObservationTx(
		ctx, tx, stored, observation, count, reporting, currentGeneration,
	)
	if err != nil || !changed {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	if err := r.writeSessionCompactionMetadataTx(ctx, tx, stored.SessionID, updatedCount, updatedReporting); err != nil {
		return nil, nil, err
	}
	return &updatedCount, updatedReporting, nil
}

func (r *Repository) applyCompactionObservationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	stored models.AgentDeliveryEvent,
	observation *streams.ContextCompactionObservation,
	count int64,
	reporting *streams.ContextCompactionReporting,
	currentGeneration bool,
) (int64, *streams.ContextCompactionReporting, bool, error) {
	switch observation.Kind {
	case streams.ContextCompactionObservationKindCompletion:
		if !observation.ValidCompletion() {
			return count, reporting, false, nil
		}
		inserted, err := r.insertSessionCompactionReceiptTx(ctx, tx, stored, observation)
		if err != nil || !inserted {
			return count, reporting, false, err
		}
		return count + 1, nextCopilotCompactionReporting(reporting, observation, currentGeneration), true, nil
	case streams.ContextCompactionObservationKindReporting:
		if !observation.ValidReporting() {
			return count, reporting, false, nil
		}
		return count, nextCopilotCompactionReporting(reporting, observation, currentGeneration), true, nil
	default:
		return count, reporting, false, nil
	}
}

var errCompactionNativeIdentity = errors.New("compaction observation does not match the retained native conversation")

// isCurrentCompactionGenerationTx reports whether the durable event belongs to
// the latest authenticated generation and the native conversation currently
// bound to it. Historical replay may add its retained increment, but it cannot
// change current availability. An in-place A -> B rebind keeps the generation
// number, so generation alone would let A's evidence mark B available.
func (r *Repository) isCurrentCompactionGenerationTx(
	ctx context.Context,
	tx *sqlx.Tx,
	stored models.AgentDeliveryEvent,
	nativeSessionID string,
) (bool, error) {
	var latest int64
	var bound string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT generation, native_session_id FROM harness_session_generations
		WHERE session_id = ? AND incarnation_id = ?
		ORDER BY generation DESC LIMIT 1`), stored.SessionID, stored.IncarnationID).Scan(&latest, &bound)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return latest == stored.HarnessGeneration && bound == nativeSessionID, nil
}

// lockSessionCompactionWrites acquires the repository's transaction-scoped
// session advisory lock before reading or rewriting compaction metadata on
// PostgreSQL. SQLite's single writer already serializes the same section.
func lockSessionCompactionWrites(
	ctx context.Context,
	tx *sqlx.Tx,
	driverName, sessionID string,
) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	return lockAgentDeliveryKey(ctx, tx, agentDeliverySessionLockNamespace+sessionID)
}

func (r *Repository) validateCompactionNativeIdentityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID, incarnationID string,
	generation int64,
	nativeSessionID string,
) error {
	if incarnationID == "" || nativeSessionID == "" {
		return errCompactionNativeIdentity
	}
	var bound, agentID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT hg.native_session_id,
			COALESCE((SELECT a.name FROM agent_profiles ap
				JOIN agents a ON a.id = ap.agent_id
				WHERE ap.id = ts.agent_profile_id LIMIT 1), '')
		FROM harness_session_generations hg
		JOIN task_sessions ts ON ts.id = hg.session_id
		WHERE hg.session_id = ? AND hg.incarnation_id = ? AND hg.generation = ?`),
		sessionID, incarnationID, generation).Scan(&bound, &agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return errCompactionNativeIdentity
	}
	if err != nil {
		return err
	}
	if agentID != copilotNativeCompactionProvider {
		return errCompactionNativeIdentity
	}
	if bound == nativeSessionID {
		return nil
	}
	// A retained predecessor remains valid evidence for this exact generation:
	// a completion journaled before an in-place A -> B reset is bound to A in
	// previous_native_session_id and must still project once. New stale raw
	// frames are rejected earlier, at the adapter binding boundary.
	var rebound bool
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM harness_native_session_rebinds
			WHERE session_id = ? AND incarnation_id = ? AND generation = ?
			  AND (native_session_id = ? OR previous_native_session_id = ?)
		)`), sessionID, incarnationID, generation, nativeSessionID, nativeSessionID).Scan(&rebound)
	if err != nil {
		return err
	}
	if !rebound {
		return errCompactionNativeIdentity
	}
	return nil
}

func (r *Repository) insertSessionCompactionReceiptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	stored models.AgentDeliveryEvent,
	observation *streams.ContextCompactionObservation,
) (bool, error) {
	observedAt := stored.ReceivedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_compactions
			(session_id, provider, native_session_id, compaction_attempt_id,
			 incarnation_id, harness_generation, delivery_stream_id, delivery_sequence,
			 observed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`),
		stored.SessionID, observation.Provider, observation.NativeSessionID, observation.AttemptID,
		stored.IncarnationID, stored.HarnessGeneration, stored.StreamID, stored.Sequence,
		observedAt, r.nowUTC())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// readSessionCompactionMetadataTx reads the current metadata while the session
// row is locked on PostgreSQL (SQLite serializes writers). Callers merge only
// their own keys back through writeSessionCompactionMetadataTx, so concurrent
// gauge updates are not clobbered.
func (r *Repository) readSessionCompactionMetadataTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) (int64, *streams.ContextCompactionReporting, error) {
	var raw sql.NullString
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT metadata FROM task_sessions WHERE id = ?`), sessionID).Scan(&raw); err != nil {
		return 0, nil, err
	}
	metadata := map[string]interface{}{}
	if raw.Valid && raw.String != "" && raw.String != "null" {
		_ = json.Unmarshal([]byte(raw.String), &metadata)
	}
	count := metadataInt64(metadata[models.SessionMetaKeyContextCompactionCount])
	var reporting *streams.ContextCompactionReporting
	if value, ok := metadata[models.SessionMetaKeyContextCompactionReporting]; ok {
		if encoded, err := json.Marshal(value); err == nil {
			var parsed streams.ContextCompactionReporting
			if json.Unmarshal(encoded, &parsed) == nil {
				reporting = streams.NormalizeReporting(&parsed)
			}
		}
	}
	return count, reporting, nil
}

func metadataInt64(value interface{}) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return 0
	}
}

func nextCopilotCompactionReporting(
	previous *streams.ContextCompactionReporting,
	observation *streams.ContextCompactionObservation,
	currentGeneration bool,
) *streams.ContextCompactionReporting {
	reporting := &streams.ContextCompactionReporting{
		Source:       streams.ContextCompactionSourceCopilotNative,
		Availability: streams.ContextCompactionAvailabilityUnavailable,
		Coverage:     streams.ContextCompactionCoverageObserved,
		Revision:     1,
	}
	if previous != nil {
		copied := *previous
		reporting = &copied
		reporting.Revision++
	}
	if currentGeneration {
		// Only the current authenticated runtime can prove collection. A valid
		// completion from this generation sets availability; historical replay
		// below leaves the previous availability untouched.
		if observation.Kind == streams.ContextCompactionObservationKindCompletion {
			reporting.Availability = streams.ContextCompactionAvailabilityAvailable
		}
		if observation.Availability != "" {
			reporting.Availability = observation.Availability
		}
	}
	if observation.Coverage == streams.ContextCompactionCoveragePartial {
		reporting.Coverage = streams.ContextCompactionCoveragePartial
	}
	return streams.NormalizeReporting(reporting)
}

//nolint:dupword // Nested jsonb_set/json_set identifiers are intentional SQL nesting.
func (r *Repository) writeSessionCompactionMetadataTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
	count int64,
	reporting *streams.ContextCompactionReporting,
) error {
	reportingJSON := "null"
	if reporting != nil {
		encoded, err := json.Marshal(reporting)
		if err != nil {
			return err
		}
		reportingJSON = string(encoded)
	}
	now := r.nowUTC()
	var query string
	if dialect.IsPostgres(r.db.DriverName()) {
		base := "CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END"
		query = `
			UPDATE task_sessions
			SET metadata = jsonb_set(
				jsonb_set(` + base + `, '{context_compaction_count}', to_jsonb(?::bigint), true),
				'{context_compaction_reporting}', ?::jsonb, true
			)::text, updated_at = ?
			WHERE id = ?`
	} else {
		base := "CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END"
		query = `
			UPDATE task_sessions
			SET metadata = json_set(
				json_set(` + base + `, '$.context_compaction_count', ?),
				'$.context_compaction_reporting', json(?)
			), updated_at = ?
			WHERE id = ?`
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(query), count, reportingJSON, now, sessionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("agent session not found: %s", sessionID)
	}
	return nil
}

// SetSessionCompactionAvailability updates current-runtime collection
// availability without touching the lifetime count. It is used when a gauge
// proves the runtime is collecting, or when the backend determines the
// retained peer cannot collect natively. Available -> unavailable is allowed;
// the coverage limitation remains sticky.
func (r *Repository) SetSessionCompactionAvailability(
	ctx context.Context,
	sessionID string,
	availability string,
	coverage string,
) (*streams.ContextCompactionReporting, error) {
	if err := validateCompactionAvailability(availability, coverage); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionCompactionWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return nil, err
	}
	if _, err := lockTaskSessionRow(ctx, tx, sessionID); err != nil {
		return nil, err
	}
	count, previous, err := r.readSessionCompactionMetadataTx(ctx, tx, sessionID)
	if err != nil {
		return nil, err
	}
	normalized, changed := nextAvailabilityReporting(previous, availability, coverage)
	if !changed {
		return previous, nil
	}
	if err := r.writeSessionCompactionMetadataTx(ctx, tx, sessionID, count, normalized); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateCompactionAvailability(availability, coverage string) error {
	if availability != streams.ContextCompactionAvailabilityAvailable &&
		availability != streams.ContextCompactionAvailabilityUnavailable {
		return fmt.Errorf("unknown compaction availability %q", availability)
	}
	if coverage != streams.ContextCompactionCoverageObserved &&
		coverage != streams.ContextCompactionCoveragePartial {
		return fmt.Errorf("unknown compaction coverage %q", coverage)
	}
	return nil
}

func nextAvailabilityReporting(
	previous *streams.ContextCompactionReporting,
	availability string,
	coverage string,
) (*streams.ContextCompactionReporting, bool) {
	if previous != nil && previous.Availability == availability &&
		(coverage != streams.ContextCompactionCoveragePartial ||
			previous.Coverage == streams.ContextCompactionCoveragePartial) {
		return previous, false
	}
	reporting := &streams.ContextCompactionReporting{
		Source:       streams.ContextCompactionSourceCopilotNative,
		Availability: availability,
		Coverage:     coverage,
		Revision:     1,
	}
	if previous != nil {
		copied := *previous
		reporting = &copied
		reporting.Revision++
		reporting.Availability = availability
		if coverage == streams.ContextCompactionCoveragePartial ||
			reporting.Coverage == streams.ContextCompactionCoveragePartial {
			reporting.Coverage = streams.ContextCompactionCoveragePartial
		} else {
			reporting.Coverage = coverage
		}
	}
	normalized := streams.NormalizeReporting(reporting)
	return normalized, normalized != nil
}
