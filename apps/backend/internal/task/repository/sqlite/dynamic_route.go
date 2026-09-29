package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/db/dialect"
)

const unclassifiedRouteStateStarting = "starting"

var _ dynamicruntime.Persistence = (*Repository)(nil)
var _ dynamicruntime.ContinuationPersistence = (*Repository)(nil)
var _ dynamicruntime.GenerationStatusClaimer = (*Repository)(nil)
var _ dynamicruntime.RouteStateSnapshotClaimer = (*Repository)(nil)
var _ dynamicruntime.UnclassifiedRouteDecisionRecorder = (*Repository)(nil)
var _ dynamicruntime.UnclassifiedFallbackLaunchClaimer = (*Repository)(nil)

func (r *Repository) SaveRouteState(ctx context.Context, state dynamicruntime.RouteState) error {
	if isTransientRouteSession(state.SessionID) {
		return nil
	}
	updatedAt := state.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO dynamic_route_states (
			session_id, logical_profile_id, execution_profile_id,
			route_generation, profile_version, state, continuation_json, policy_state_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			logical_profile_id = excluded.logical_profile_id,
			execution_profile_id = excluded.execution_profile_id,
			route_generation = excluded.route_generation,
			profile_version = excluded.profile_version,
			state = excluded.state,
			continuation_json = excluded.continuation_json,
			policy_state_json = excluded.policy_state_json,
			updated_at = excluded.updated_at
	`), state.SessionID, state.LogicalProfileID, state.ExecutionProfileID,
		state.Generation, state.ProfileVersion, state.Status, state.ContinuationJSON, state.PolicyStateJSON, updatedAt)
	return err
}

// ClaimRouteState advances a route generation only when the durable row still
// has the caller's expected generation. The insert path is reserved for the
// initial generation, so a restart cannot accidentally reset an existing
// session to generation one.
func (r *Repository) ClaimRouteState(ctx context.Context, expectedGeneration int64, state dynamicruntime.RouteState) (bool, error) {
	if isTransientRouteSession(state.SessionID) {
		return true, nil
	}
	var (
		result sql.Result
		err    error
	)
	if expectedGeneration == 0 {
		result, err = r.db.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO dynamic_route_states (
				session_id, logical_profile_id, execution_profile_id,
				route_generation, profile_version, state, continuation_json, policy_state_json, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(session_id) DO NOTHING
		`), state.SessionID, state.LogicalProfileID, state.ExecutionProfileID,
			state.Generation, state.ProfileVersion, state.Status, state.ContinuationJSON, state.PolicyStateJSON, state.UpdatedAt)
	} else {
		result, err = r.db.ExecContext(ctx, r.db.Rebind(`
			UPDATE dynamic_route_states
			SET logical_profile_id = ?, execution_profile_id = ?,
				route_generation = ?, profile_version = ?, state = ?, continuation_json = ?, policy_state_json = ?, updated_at = ?
			WHERE session_id = ? AND route_generation = ?
		`), state.LogicalProfileID, state.ExecutionProfileID, state.Generation,
			state.ProfileVersion, state.Status, state.ContinuationJSON, state.PolicyStateJSON, state.UpdatedAt, state.SessionID,
			expectedGeneration)
	}
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// ClaimRouteStateFrom updates a route generation only while both its
// generation and status still match the caller's observation. Same-generation
// transitions use this narrower fence so a late recovery callback cannot
// overwrite a route that became active (or vice versa).
func (r *Repository) ClaimRouteStateFrom(
	ctx context.Context,
	expectedGeneration int64,
	expectedStatus string,
	state dynamicruntime.RouteState,
) (bool, error) {
	if isTransientRouteSession(state.SessionID) {
		return true, nil
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE dynamic_route_states
		SET logical_profile_id = ?, execution_profile_id = ?,
			route_generation = ?, profile_version = ?, state = ?, continuation_json = ?, policy_state_json = ?, updated_at = ?
		WHERE session_id = ? AND route_generation = ? AND state = ?
	`), state.LogicalProfileID, state.ExecutionProfileID,
		state.Generation, state.ProfileVersion, state.Status, state.ContinuationJSON, state.PolicyStateJSON, state.UpdatedAt,
		state.SessionID, expectedGeneration, expectedStatus)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// ClaimRouteStateFromSnapshot atomically updates a same-generation route only
// while both status and the observed policy JSON still match. Streak updates
// can remain action_required on both sides, so status alone is not an
// idempotency fence.
func (r *Repository) ClaimRouteStateFromSnapshot(
	ctx context.Context,
	expectedGeneration int64,
	expectedStatus string,
	expectedPolicyState string,
	state dynamicruntime.RouteState,
) (bool, error) {
	if isTransientRouteSession(state.SessionID) {
		return true, nil
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE dynamic_route_states
		SET logical_profile_id = ?, execution_profile_id = ?,
			route_generation = ?, profile_version = ?, state = ?, continuation_json = ?, policy_state_json = ?, updated_at = ?
		WHERE session_id = ? AND route_generation = ? AND state = ? AND policy_state_json = ?
	`), state.LogicalProfileID, state.ExecutionProfileID,
		state.Generation, state.ProfileVersion, state.Status, state.ContinuationJSON, state.PolicyStateJSON, state.UpdatedAt,
		state.SessionID, expectedGeneration, expectedStatus, expectedPolicyState)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// RecordRouteDecision commits the generation claim and immutable attempt row
// together. A stale claim returns dynamicruntime.ErrStaleGeneration.
func (r *Repository) RecordRouteDecision(ctx context.Context, decision dynamicruntime.RouteDecision, state dynamicruntime.RouteState) error {
	if isTransientRouteSession(state.SessionID) {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := recordRouteDecisionTx(ctx, r, tx, decision, state); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordUnclassifiedRouteDecision checks the exact workflow context used to
// admit the failure while holding the same step-then-task locks as workflow
// mutations. The route generation and its immutable attempt are committed in
// that transaction, so a moved or newly vetoed task cannot claim a successor.
func (r *Repository) RecordUnclassifiedRouteDecision(
	ctx context.Context,
	decision dynamicruntime.RouteDecision,
	state dynamicruntime.RouteState,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateUnclassifiedWorkflowContextTx(ctx, tx, evidence); err != nil {
		return err
	}
	if !isTransientRouteSession(state.SessionID) {
		if err := recordRouteDecisionTx(ctx, r, tx, decision, state); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClaimUnclassifiedFallbackLaunch is the final automatic-launch fence. It
// serializes with step edits and task moves, then confirms that this route
// generation still owns the starting claim before the detached worker starts
// the successor.
func (r *Repository) ClaimUnclassifiedFallbackLaunch(
	ctx context.Context,
	decision dynamicruntime.RouteDecision,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateUnclassifiedWorkflowContextTx(ctx, tx, evidence); err != nil {
		return err
	}
	query := `SELECT logical_profile_id, execution_profile_id, route_generation, profile_version, state
		FROM dynamic_route_states WHERE session_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var logicalProfileID, executionProfileID, status string
	var generation, profileVersion int64
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), decision.SessionID).Scan(
		&logicalProfileID, &executionProfileID, &generation, &profileVersion, &status,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dynamicruntime.ErrStaleGeneration
		}
		return err
	}
	if logicalProfileID != decision.LogicalProfileID || executionProfileID != decision.ExecutionProfileID ||
		generation != decision.Generation || profileVersion != decision.ProfileVersion || status != unclassifiedRouteStateStarting {
		return dynamicruntime.ErrStaleGeneration
	}
	return tx.Commit()
}

func (r *Repository) validateUnclassifiedWorkflowContextTx(
	ctx context.Context,
	tx *sqlx.Tx,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	if !evidence.TaskScope || evidence.TaskID == "" || !evidence.StepKnown {
		return dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable
	}
	if err := r.lockUnclassifiedWorkflowStep(ctx, tx, evidence.StepID); err != nil {
		return err
	}
	if err := r.validateUnclassifiedTaskStepTx(ctx, tx, evidence); err != nil {
		return err
	}
	if err := r.validateUnclassifiedSessionTaskTx(ctx, tx, evidence); err != nil {
		return err
	}
	return r.validateUnclassifiedStepRevisionTx(ctx, tx, evidence)
}

func (r *Repository) lockUnclassifiedWorkflowStep(ctx context.Context, tx *sqlx.Tx, stepID string) error {
	if stepID == "" {
		return nil
	}
	if err := lockWorkflowStepForWrite(ctx, tx, r.db.DriverName(), r.db.Rebind, stepID); err != nil {
		return fmt.Errorf("%w: lock workflow step: %v", dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable, err)
	}
	return nil
}

func (r *Repository) validateUnclassifiedTaskStepTx(
	ctx context.Context,
	tx *sqlx.Tx,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	workflowID, stepID, found, err := r.readTaskStepInTx(ctx, tx, evidence.TaskID)
	if err != nil {
		return fmt.Errorf("%w: read task step: %v", dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable, err)
	}
	if !found {
		return dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable
	}
	if workflowID != evidence.WorkflowID || stepID != evidence.StepID {
		return dynamicruntime.ErrUnclassifiedWorkflowContextChanged
	}
	return nil
}

func (r *Repository) validateUnclassifiedSessionTaskTx(
	ctx context.Context,
	tx *sqlx.Tx,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	var sessionTaskID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id FROM task_sessions WHERE id = ?`), evidence.SessionID).Scan(&sessionTaskID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable
		}
		return fmt.Errorf("%w: read task session: %v", dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable, err)
	}
	if sessionTaskID != evidence.TaskID {
		return dynamicruntime.ErrUnclassifiedWorkflowContextChanged
	}
	return nil
}

func (r *Repository) validateUnclassifiedStepRevisionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	evidence dynamicruntime.UnclassifiedFailureEvidence,
) error {
	if evidence.StepID == "" {
		if evidence.StepVeto || !evidence.StepUpdatedAt.IsZero() {
			return dynamicruntime.ErrUnclassifiedWorkflowContextChanged
		}
		return nil
	}

	stepQuery := `SELECT workflow_id, disable_unclassified_fallback, updated_at FROM workflow_steps WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		stepQuery += forUpdateClause
	}
	var currentWorkflowID string
	var veto int
	var updatedAt time.Time
	if err := tx.QueryRowContext(ctx, r.db.Rebind(stepQuery), evidence.StepID).Scan(&currentWorkflowID, &veto, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dynamicruntime.ErrUnclassifiedWorkflowContextChanged
		}
		return fmt.Errorf("%w: read workflow step: %v", dynamicruntime.ErrUnclassifiedWorkflowContextUnavailable, err)
	}
	if currentWorkflowID != evidence.WorkflowID || veto != 0 || evidence.StepVeto ||
		evidence.StepUpdatedAt.IsZero() || !updatedAt.Equal(evidence.StepUpdatedAt) {
		return dynamicruntime.ErrUnclassifiedWorkflowContextChanged
	}
	return nil
}

func recordRouteDecisionTx(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	decision dynamicruntime.RouteDecision,
	state dynamicruntime.RouteState,
) error {
	expectedGeneration := state.Generation - 1
	var result sql.Result
	var err error
	if expectedGeneration == 0 {
		result, err = tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO dynamic_route_states (
				session_id, logical_profile_id, execution_profile_id,
				route_generation, profile_version, state, continuation_json, policy_state_json, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(session_id) DO NOTHING
		`), state.SessionID, state.LogicalProfileID, state.ExecutionProfileID,
			state.Generation, state.ProfileVersion, state.Status, state.ContinuationJSON, state.PolicyStateJSON, state.UpdatedAt)
	} else {
		result, err = tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE dynamic_route_states
			SET logical_profile_id = ?, execution_profile_id = ?,
				route_generation = ?, profile_version = ?, state = ?, continuation_json = ?, policy_state_json = ?, updated_at = ?
			WHERE session_id = ? AND route_generation = ?
		`), state.LogicalProfileID, state.ExecutionProfileID, state.Generation,
			state.ProfileVersion, state.Status, state.ContinuationJSON, state.PolicyStateJSON, state.UpdatedAt, state.SessionID,
			expectedGeneration)
	}
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return dynamicruntime.ErrStaleGeneration
	}
	createdAt := decisionReasonTime(decision, state)
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO dynamic_route_attempts (
			id, session_id, logical_profile_id, execution_profile_id,
			route_generation, profile_version, reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), uuid.New().String(), decision.SessionID, decision.LogicalProfileID,
		decision.ExecutionProfileID, decision.Generation, decision.ProfileVersion,
		decision.Reason, createdAt); err != nil {
		return err
	}
	return nil
}

func decisionReasonTime(_ dynamicruntime.RouteDecision, state dynamicruntime.RouteState) time.Time {
	if !state.UpdatedAt.IsZero() {
		return state.UpdatedAt
	}
	return time.Now().UTC()
}

func (r *Repository) AppendRouteAttempt(ctx context.Context, attempt dynamicruntime.RouteAttempt) error {
	if isTransientRouteSession(attempt.SessionID) {
		return nil
	}
	createdAt := attempt.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO dynamic_route_attempts (
			id, session_id, logical_profile_id, execution_profile_id,
			route_generation, profile_version, reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), uuid.New().String(), attempt.SessionID, attempt.LogicalProfileID,
		attempt.ExecutionProfileID, attempt.Generation, attempt.ProfileVersion,
		attempt.Reason, createdAt)
	return err
}

func (r *Repository) LoadRouteState(ctx context.Context, sessionID string) (*dynamicruntime.RouteState, error) {
	if isTransientRouteSession(sessionID) {
		return nil, nil
	}
	state := &dynamicruntime.RouteState{}
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT session_id, logical_profile_id, execution_profile_id,
		route_generation, profile_version, state, continuation_json, policy_state_json, updated_at
		FROM dynamic_route_states WHERE session_id = ?
	`), sessionID).Scan(
		&state.SessionID, &state.LogicalProfileID, &state.ExecutionProfileID,
		&state.Generation, &state.ProfileVersion, &state.Status, &state.ContinuationJSON, &state.PolicyStateJSON, &state.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return state, nil
}

// ListPendingRouteStates returns only states whose durable policy deadline can
// be reconciled automatically. States marked retrying are intentionally not
// returned after restart because dispatch may already have crossed the
// process boundary and must remain a manual recovery decision.
func (r *Repository) ListPendingRouteStates(ctx context.Context) ([]dynamicruntime.RouteState, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT session_id, logical_profile_id, execution_profile_id,
			route_generation, profile_version, state, continuation_json, policy_state_json, updated_at
		FROM dynamic_route_states
		WHERE state IN (?, ?) ORDER BY updated_at ASC
	`), string("retry_wait"), string("waiting_for_reset"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	states := make([]dynamicruntime.RouteState, 0)
	for rows.Next() {
		var state dynamicruntime.RouteState
		if err := rows.Scan(
			&state.SessionID, &state.LogicalProfileID, &state.ExecutionProfileID,
			&state.Generation, &state.ProfileVersion, &state.Status,
			&state.ContinuationJSON, &state.PolicyStateJSON, &state.UpdatedAt,
		); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return states, nil
}

// ListStartingRouteStates returns every route whose durable status is still
// "starting". Startup reconciliation is the only caller: a healthy route also
// passes through "starting" en route to "active", so listing this status is
// only a safe orphan signal once the caller has confirmed there is no live
// session backing the row (see Service.reconcileOrphanedDynamicStartingRoutes).
func (r *Repository) ListStartingRouteStates(ctx context.Context) ([]dynamicruntime.RouteState, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT session_id, logical_profile_id, execution_profile_id,
			route_generation, profile_version, state, continuation_json, policy_state_json, updated_at
		FROM dynamic_route_states
		WHERE state = ? ORDER BY updated_at ASC
	`), unclassifiedRouteStateStarting)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	states := make([]dynamicruntime.RouteState, 0)
	for rows.Next() {
		var state dynamicruntime.RouteState
		if err := rows.Scan(
			&state.SessionID, &state.LogicalProfileID, &state.ExecutionProfileID,
			&state.Generation, &state.ProfileVersion, &state.Status,
			&state.ContinuationJSON, &state.PolicyStateJSON, &state.UpdatedAt,
		); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return states, nil
}

func (r *Repository) SaveRouteContinuation(ctx context.Context, record dynamicruntime.ContinuationRecord) error {
	if isTransientRouteSession(record.SessionID) {
		return nil
	}
	payload, err := json.Marshal(record.Continuation)
	if err != nil {
		return err
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE dynamic_route_states
		SET continuation_json = ?, updated_at = ?
		WHERE session_id = ? AND route_generation = ?
	`), string(payload), updatedAt, record.SessionID, record.Generation)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return dynamicruntime.ErrStaleGeneration
	}
	return nil
}

func (r *Repository) SaveCircuit(ctx context.Context, snapshot dynamicruntime.CircuitSnapshot) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO dynamic_resource_circuits
			(resource_key, state, until_at, code, probe_until, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(resource_key) DO UPDATE SET
			state = excluded.state,
			until_at = excluded.until_at,
			code = excluded.code,
			probe_until = excluded.probe_until,
			updated_at = excluded.updated_at
	`), snapshot.Key, snapshot.State, nullableTime(snapshot.Until), snapshot.Code,
		nullableTime(snapshot.ProbeUntil), time.Now().UTC())
	return err
}

func (r *Repository) LoadCircuits(ctx context.Context) ([]dynamicruntime.CircuitSnapshot, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT resource_key, state, until_at, code, probe_until
		FROM dynamic_resource_circuits
		WHERE state <> ? ORDER BY resource_key
	`), dynamicruntime.CircuitClosed)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var snapshots []dynamicruntime.CircuitSnapshot
	for rows.Next() {
		var snapshot dynamicruntime.CircuitSnapshot
		var until, probeUntil sql.NullTime
		var state string
		var code string
		if err := rows.Scan(&snapshot.Key, &state, &until, &code, &probeUntil); err != nil {
			return nil, err
		}
		snapshot.State = dynamicruntime.CircuitState(state)
		snapshot.Code = routingerr.Code(code)
		if until.Valid {
			snapshot.Until = until.Time
		}
		if probeUntil.Valid {
			snapshot.ProbeUntil = probeUntil.Time
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func (r *Repository) LoadOrCreate(ctx context.Context) ([]byte, error) {
	var key []byte
	err := r.ro.QueryRowContext(ctx, `SELECT key_bytes FROM dynamic_installation_keys WHERE id = 1`).Scan(&key)
	if err == nil {
		return append([]byte(nil), key...), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO dynamic_installation_keys (id, key_bytes, created_at) VALUES (1, ?, ?)
	`), key, time.Now().UTC())
	if err != nil {
		// Another process may have won the first insert. Read its stable key.
		if readErr := r.ro.QueryRowContext(ctx, `SELECT key_bytes FROM dynamic_installation_keys WHERE id = 1`).Scan(&key); readErr == nil {
			return append([]byte(nil), key...), nil
		}
		return nil, err
	}
	return key, nil
}

func nullableTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value
}

func (r *Repository) ListRouteAttempts(ctx context.Context, sessionID string) ([]dynamicruntime.RouteAttempt, error) {
	if isTransientRouteSession(sessionID) {
		return []dynamicruntime.RouteAttempt{}, nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT session_id, logical_profile_id, execution_profile_id,
			route_generation, profile_version, reason, created_at
		FROM dynamic_route_attempts
		WHERE session_id = ? ORDER BY route_generation ASC
	`), sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	attempts := make([]dynamicruntime.RouteAttempt, 0)
	for rows.Next() {
		var attempt dynamicruntime.RouteAttempt
		if err := rows.Scan(&attempt.SessionID, &attempt.LogicalProfileID,
			&attempt.ExecutionProfileID, &attempt.Generation, &attempt.ProfileVersion,
			&attempt.Reason, &attempt.CreatedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attempts, nil
}

func isTransientRouteSession(sessionID string) bool {
	return strings.HasPrefix(sessionID, "utility:")
}
