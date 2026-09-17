package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
)

// DefaultGateFailureThreshold is the consecutive fail-closed evaluation
// count that triggers a durable escalation record
// (AC-OFFICE-BACKPRESSURE-003.5).
const DefaultGateFailureThreshold = 3

// GateFailureEscalationInterval is the minimum time between escalation
// writes for the same (workspace, gate) pair (AC-OFFICE-BACKPRESSURE-003.9).
const GateFailureEscalationInterval = time.Hour

// SetGateFailureThreshold configures the consecutive-failure threshold
// RecordGateOutcome escalates at. A value less than 1 is replaced by
// DefaultGateFailureThreshold, matching the clamp-on-read idiom used by
// ClaimSafetyLimits.
func (r *Repository) SetGateFailureThreshold(threshold int) {
	r.gateFailureThreshold = threshold
}

func (r *Repository) effectiveGateFailureThreshold() int {
	return clampToDefaultInt(r.gateFailureThreshold, DefaultGateFailureThreshold)
}

// RecordGateOutcome is RecordGateOutcomeTx for a standalone caller with no
// transaction already open (e.g. the enqueue-side refusal gates in
// runs/service.causation.go): it begins and commits its own transaction
// around the same read-modify-write.
//
// Callers must never let an error from this method affect an admission
// decision (AC-OFFICE-BACKPRESSURE-003.4): log and continue.
func (r *Repository) RecordGateOutcome(ctx context.Context, workspaceID, gate string, success bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.RecordGateOutcomeTx(ctx, tx, workspaceID, gate, success); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordGateOutcomeTx updates the durable consecutive-failure count for
// one (workspace, gate) pair (AC-OFFICE-BACKPRESSURE-003.8) within a
// transaction the caller owns — required for the claim-side deferral
// gates, which must reuse ClaimNextEligibleRun's already-open transaction
// rather than opening a second one: SQLite's writer is a single
// connection, so a nested RecordGateOutcome call there would deadlock
// against the outer transaction holding that same connection. A
// successful evaluation — readable input, whether it then admitted or
// correctly blocked the candidate (AC-OFFICE-BACKPRESSURE-003.10's
// "successfully evaluated" meaning readable, not permissive) — resets
// the count to 0. A failed evaluation (input could not be read)
// increments it. When the count reaches the effective threshold and at
// least GateFailureEscalationInterval has passed since the last
// escalation for this pair (or none has ever been recorded),
// last_escalation_at is stamped — the durable, operator-visible
// escalation record (AC-OFFICE-BACKPRESSURE-003.5, -003.9).
//
// On Postgres the read takes FOR UPDATE to hold the row against a
// concurrent caller for the duration of the caller's transaction; SQLite
// needs no such lock because its writer connection already serializes
// every transaction. FOR UPDATE locks nothing when the row does not yet
// exist, so the row is created (a no-op via ON CONFLICT DO NOTHING when it
// already exists) before the locking read — otherwise two concurrent
// first-failures for the same pair could both read no row, both compute
// consecutive_failures=1, and the second upsert would silently overwrite
// (lose) the first failure.
func (r *Repository) RecordGateOutcomeTx(ctx context.Context, tx *sqlx.Tx, workspaceID, gate string, success bool) error {
	driver := r.db.DriverName()
	now := time.Now().UTC()

	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO office_gate_failure_state (workspace_id, gate, consecutive_failures, last_escalation_at, updated_at)
		VALUES (?, ?, 0, NULL, ?)
		ON CONFLICT (workspace_id, gate) DO NOTHING
	`), workspaceID, gate, now); err != nil {
		return err
	}

	query := `SELECT consecutive_failures, last_escalation_at FROM office_gate_failure_state WHERE workspace_id = ? AND gate = ?`
	if dialect.IsPostgres(driver) {
		query += " FOR UPDATE"
	}

	var consecutiveFailures int
	var lastEscalationAt sql.NullTime
	if err := tx.QueryRowContext(ctx, tx.Rebind(query), workspaceID, gate).Scan(&consecutiveFailures, &lastEscalationAt); err != nil {
		return err
	}

	if success {
		consecutiveFailures = 0
	} else {
		consecutiveFailures++
		if consecutiveFailures >= r.effectiveGateFailureThreshold() &&
			(!lastEscalationAt.Valid || now.Sub(lastEscalationAt.Time) >= GateFailureEscalationInterval) {
			lastEscalationAt = sql.NullTime{Time: now, Valid: true}
		}
	}

	_, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE office_gate_failure_state
		SET consecutive_failures = ?, last_escalation_at = ?, updated_at = ?
		WHERE workspace_id = ? AND gate = ?
	`), consecutiveFailures, lastEscalationAt, now, workspaceID, gate)
	return err
}

// GetGateFailureState reads the current escalation state for one
// (workspace, gate) pair. Returns sql.ErrNoRows when no evaluation has
// ever been recorded for that pair.
func (r *Repository) GetGateFailureState(ctx context.Context, workspaceID, gate string) (*models.GateFailureState, error) {
	var state models.GateFailureState
	err := r.db.GetContext(ctx, &state, r.db.Rebind(
		`SELECT workspace_id, gate, consecutive_failures, last_escalation_at, updated_at
		 FROM office_gate_failure_state WHERE workspace_id = ? AND gate = ?`,
	), workspaceID, gate)
	if err != nil {
		return nil, err
	}
	return &state, nil
}
