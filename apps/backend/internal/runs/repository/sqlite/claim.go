package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// claimCandidateBatchSize bounds how many queued rows one
// ClaimNextEligibleRun call inspects before giving up. The claim
// statement can no longer select its answer in one predicate once the
// ceilings and budgets are per-agent/per-workspace/per-routine instead
// of a single "count = 0" lock, so candidates are read in claim order
// and evaluated one at a time until one clears every gate.
const claimCandidateBatchSize = 20

// Deferral gate names, in the precedence
// docs/specs/office/system-design/unattended-launch-safety-02.md
// "Attributing a deferral" declares (most specific first).
const (
	gateAgentCeiling     = "agent_ceiling"
	gateWorkspaceCeiling = "workspace_ceiling"
	gateInstanceCeiling  = "instance_ceiling"
	gateRoutineBudget    = "routine_budget"
	gateWorkspaceBudget  = "workspace_budget"
)

// launchClaimLockKey is the fixed, instance-wide Postgres advisory-lock
// key ClaimNextEligibleRun holds for the duration of its transaction.
// It is a constant, not derived from a workspace or agent id, because
// the broadest ceiling (instance-wide) is what the lock must serialize;
// a per-workspace key would leave the instance ceiling racing between
// workspaces. See internal/secrets.WorkspaceLockKey for the precedent
// this follows for a *scoped* lock; this one is deliberately unscoped.
const launchClaimLockKey int64 = 0x4c41554e434c4331

// ClaimNextEligibleRun atomically claims the next eligible queued run.
// "Eligible" now means more than FIFO-per-agent: the row must also
// clear the per-agent, per-workspace, and instance-wide concurrency
// ceilings and (unless human_rooted) the workspace and routine launch
// budgets, per REQ-OFFICE-LAUNCH-SAFETY-001/005. Claim order follows
// priority_class with age-based promotion, per REQ-OFFICE-BACKPRESSURE-001/002.
// A successful claim appends one office_launch_ledger row in the same
// transaction, per REQ-OFFICE-LAUNCH-SAFETY-002.
//
// Every ceiling/budget check runs inside one transaction — a Postgres
// advisory lock (SQLite's single writer already serializes) — so two
// concurrent callers can never both observe the same free slot. A run
// that clears no gate is left untouched in status='queued'; the caller
// sees sql.ErrNoRows exactly as before this rewrite.
func (r *Repository) ClaimNextEligibleRun(ctx context.Context) (*models.Run, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	driver := r.db.DriverName()
	if dialect.IsPostgres(driver) {
		if _, err := tx.ExecContext(ctx, r.db.Rebind("SELECT pg_advisory_xact_lock(?)"), launchClaimLockKey); err != nil {
			return nil, fmt.Errorf("acquire launch claim lock: %w", err)
		}
	}

	now := time.Now().UTC()
	limits := r.effectiveClaimLimits()
	promotionCutoff := now.Add(-limits.PromotionAge)
	budgetWindowStart := now.Add(-BudgetWindow)

	// AC-OFFICE-BACKPRESSURE-002.1/002.2: a run queued strictly longer
	// than PromotionAge claims as one class better, clamped at
	// `recovery` (1) so promotion can never reach `human` (0). Reused
	// from dialect.GreatestTimestamp, which is a generic two-argument
	// max/GREATEST despite the timestamp-flavoured name.
	promotedClass := fmt.Sprintf(
		"CASE WHEN w.requested_at < ? THEN %s ELSE w.priority_class END",
		dialect.GreatestTimestamp(driver, "w.priority_class - 1", "1"),
	)
	query := fmt.Sprintf(`
		SELECT w.* FROM runs w
		WHERE w.status = 'queued'
		  AND (w.scheduled_retry_at IS NULL OR w.scheduled_retry_at <= ?)
		  AND w.routing_blocked_status IS NULL
		ORDER BY %s ASC, w.requested_at ASC, w.id ASC
		LIMIT %d
	`, promotedClass, claimCandidateBatchSize)

	// Positional placeholder order follows the assembled query text, not
	// the order these values are computed above: the WHERE clause's
	// `scheduled_retry_at <= ?` appears before the ORDER BY CASE's
	// `requested_at < ?`, so `now` binds first and `promotionCutoff` second.
	var candidates []models.Run
	if err := tx.SelectContext(ctx, &candidates, tx.Rebind(query), now, promotionCutoff); err != nil {
		return nil, err
	}

	// attributedGate is set from the first (highest-priority under the
	// effective claim order) candidate's blocking gate, and reported at
	// most once for the whole attempt: AC-OFFICE-BACKPRESSURE-003.6/.7
	// attribute a no-row claim attempt to the gate blocking the
	// highest-priority eligible run, counted per attempt rather than
	// per blocked candidate. candidates is already in that exact order
	// (the SELECT above), so candidates[0]'s gate is the one to report;
	// later candidates are still evaluated (to find one that clears
	// every gate) but their blocks are not separately counted.
	var attributedGate string
	var attributedRun *models.Run

	for i := range candidates {
		candidate := &candidates[i]
		if blocked, gate := r.claimGateBlocks(ctx, tx, candidate, limits, budgetWindowStart); blocked {
			if attributedGate == "" {
				attributedGate = gate
				attributedRun = candidate
			}
			continue
		}
		claimed, err := r.commitClaim(ctx, tx, candidate)
		if err != nil {
			return nil, err
		}
		return claimed, nil
	}
	if attributedGate != "" {
		shared.LaunchDeferredTotal.Add(shared.LaunchSafetyLabel("gate", attributedGate), 1)
		r.logDeferral(attributedGate, attributedRun)
	}
	// No candidate cleared every gate: no run row changes, but every
	// gate evaluated above wrote its outcome to office_gate_failure_state
	// in this same transaction (RecordGateOutcomeTx, called from
	// claimGateBlocks's per-gate evaluators). That durable record must
	// still land even though nothing was claimed — REQ-OFFICE-BACKPRESSURE-003.8
	// tracks deferrals, so committing only on a successful claim would
	// silently discard the escalation data for exactly the attempts that
	// matter most.
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return nil, sql.ErrNoRows
}

// logDeferral emits AC-OFFICE-BACKPRESSURE-003.7's structured log entry
// for a claim attempt that returned no row while eligible queued runs
// existed: the gate attributed by AC-OFFICE-BACKPRESSURE-003.6, the
// blocked run's workspace and agent profile, and its wake reason. A
// nil/never-set logger (most tests) simply skips the entry.
func (r *Repository) logDeferral(gate string, run *models.Run) {
	if r.log == nil {
		return
	}
	r.log.Info("run claim deferred",
		zap.String("gate", gate),
		zap.String("run_id", run.ID),
		zap.String("workspace_id", run.WorkspaceID),
		zap.String("agent_profile", run.AgentProfileID),
		zap.String("reason", run.Reason),
	)
}

// commitClaim marks candidate claimed and appends its launch-ledger
// row, then commits the transaction the caller began.
func (r *Repository) commitClaim(ctx context.Context, tx *sqlx.Tx, candidate *models.Run) (*models.Run, error) {
	claimedAt := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`,
	), claimedAt, candidate.ID); err != nil {
		return nil, err
	}
	ledgerID := uuid.New().String()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO office_launch_ledger (
			id, run_id, workspace_id, causation_id, routine_id, human_rooted, claimed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`), ledgerID, candidate.ID, candidate.WorkspaceID, candidate.CausationID,
		candidate.RoutineID, dialect.BoolToInt(candidate.HumanRooted), claimedAt); err != nil {
		return nil, fmt.Errorf("append launch ledger: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	candidate.Status = "claimed"
	candidate.ClaimedAt = &claimedAt
	return candidate, nil
}

// claimGateBlocks evaluates every deferral gate for candidate, in the
// precedence docs/specs/office/system-design/unattended-launch-safety-02.md
// "Attributing a deferral" declares (most specific first): agent_ceiling,
// workspace_ceiling, instance_ceiling, routine_budget, workspace_budget.
// The first gate that blocks is reported; a gate whose input cannot be
// read fails closed rather than admitting the candidate
// (AC-OFFICE-LAUNCH-SAFETY, "Failure and recovery"). Each gate records its
// own outcome (readable-and-blocked, readable-and-passed, or unreadable)
// against the durable escalation state, per REQ-OFFICE-BACKPRESSURE-003.8.
func (r *Repository) claimGateBlocks(
	ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits, budgetWindowStart time.Time,
) (bool, string) {
	if r.evalAgentCeilingGate(ctx, tx, candidate, limits) {
		return true, gateAgentCeiling
	}
	if r.evalWorkspaceCeilingGate(ctx, tx, candidate, limits) {
		return true, gateWorkspaceCeiling
	}
	if r.evalInstanceCeilingGate(ctx, tx, candidate, limits) {
		return true, gateInstanceCeiling
	}

	if candidate.HumanRooted {
		// AC-OFFICE-LAUNCH-SAFETY-005.6: the budget exemption tests the
		// root of the chain, not this run's own (recomputed) priority
		// class, so it is checked from the persisted human_rooted flag.
		return false, ""
	}

	if candidate.RoutineID != "" && r.evalRoutineBudgetGate(ctx, tx, candidate, limits, budgetWindowStart) {
		return true, gateRoutineBudget
	}
	if r.evalWorkspaceBudgetGate(ctx, tx, candidate, limits, budgetWindowStart) {
		return true, gateWorkspaceBudget
	}
	return false, ""
}

// recordGateOutcome persists a gate's evaluation outcome for workspaceID
// (RecordGateOutcomeTx, reusing the caller's open transaction), counting
// rather than propagating any failure to persist it: per
// AC-OFFICE-BACKPRESSURE-003.4, failure-tracking itself must never affect
// the admission decision.
func (r *Repository) recordGateOutcome(ctx context.Context, tx *sqlx.Tx, workspaceID, gate string, success bool) {
	if err := r.RecordGateOutcomeTx(ctx, tx, workspaceID, gate, success); err != nil {
		shared.GateOutcomeRecordFailedTotal.Add(shared.LaunchSafetyLabel("gate", gate), 1)
	}
}

// evalAgentCeilingGate reports whether candidate's agent is at or over its
// effective per-agent ceiling. Returns true (blocked) both when the
// ceiling is reached and when a required input could not be read
// (fail closed).
func (r *Repository) evalAgentCeilingGate(
	ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits,
) bool {
	agentCap, err := r.agentCeiling(ctx, tx, candidate.AgentProfileID, limits)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateAgentCeiling), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateAgentCeiling, false)
		return true
	}
	if agentCap <= 0 {
		// No resolvable agent_profiles row (or an invalid non-positive
		// value): a missing ceiling input defers rather than admitting
		// an unbounded agent, per AC-OFFICE-LAUNCH-SAFETY-001.8. The read
		// itself succeeded (an authoritative "no profile"), so this is a
		// successful evaluation for AC-OFFICE-BACKPRESSURE-003.10.
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateAgentCeiling, true)
		return true
	}
	agentClaimed, err := r.countClaimed(ctx, tx, "agent_profile_id = ?", candidate.AgentProfileID)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateAgentCeiling), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateAgentCeiling, false)
		return true
	}
	r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateAgentCeiling, true)
	return agentClaimed >= agentCap
}

// evalWorkspaceCeilingGate reports whether candidate's workspace is at or
// over the instance-wide per-workspace ceiling.
func (r *Repository) evalWorkspaceCeilingGate(
	ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits,
) bool {
	workspaceClaimed, err := r.countClaimed(ctx, tx, "workspace_id = ?", candidate.WorkspaceID)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateWorkspaceCeiling), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateWorkspaceCeiling, false)
		return true
	}
	r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateWorkspaceCeiling, true)
	return workspaceClaimed >= limits.MaxConcurrentWorkspace
}

// evalInstanceCeilingGate reports whether the instance-wide concurrent
// claim ceiling has been reached.
func (r *Repository) evalInstanceCeilingGate(
	ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits,
) bool {
	instanceClaimed, err := r.countClaimed(ctx, tx, "1 = 1")
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateInstanceCeiling), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateInstanceCeiling, false)
		return true
	}
	r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateInstanceCeiling, true)
	return instanceClaimed >= limits.MaxConcurrentInstance
}

// evalRoutineBudgetGate reports whether candidate's routine is at or over
// its rolling-hour launch budget. Only called for a non-human-rooted
// candidate with a routine id.
func (r *Repository) evalRoutineBudgetGate(
	ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits, budgetWindowStart time.Time,
) bool {
	routineClaims, err := r.countLedger(ctx, tx, "routine_id = ? AND claimed_at > ?", candidate.RoutineID, budgetWindowStart)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateRoutineBudget), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateRoutineBudget, false)
		return true
	}
	r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateRoutineBudget, true)
	return routineClaims >= limits.RoutineBudgetPerHour
}

// evalWorkspaceBudgetGate reports whether candidate's workspace is at or
// over its rolling-hour launch budget. Only called for a non-human-rooted
// candidate.
func (r *Repository) evalWorkspaceBudgetGate(
	ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits, budgetWindowStart time.Time,
) bool {
	workspaceClaims, err := r.countLedger(ctx, tx, "workspace_id = ? AND claimed_at > ?", candidate.WorkspaceID, budgetWindowStart)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateWorkspaceBudget), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateWorkspaceBudget, false)
		return true
	}
	r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateWorkspaceBudget, true)
	return workspaceClaims >= limits.WorkspaceBudgetPerHour
}

// agentCeiling resolves the effective per-agent claim ceiling: the
// agent's own configured max_concurrent_sessions, clamped to the
// workspace and instance ceilings so raising it on one agent cannot
// raise the real bound (AC-OFFICE-LAUNCH-SAFETY-001.9). A missing
// agent_profiles row returns (0, nil) — not an error — so the caller
// treats it as a deferral rather than a failed read.
func (r *Repository) agentCeiling(
	ctx context.Context, tx *sqlx.Tx, agentProfileID string, limits ClaimSafetyLimits,
) (int, error) {
	var maxSessions int
	err := tx.QueryRowContext(ctx, tx.Rebind(
		`SELECT max_concurrent_sessions FROM agent_profiles WHERE id = ?`,
	), agentProfileID).Scan(&maxSessions)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return minInt(maxSessions, minInt(limits.MaxConcurrentWorkspace, limits.MaxConcurrentInstance)), nil
}

// countClaimed counts claimed runs matching whereClause, e.g. "1 = 1"
// for the instance-wide ceiling.
func (r *Repository) countClaimed(ctx context.Context, tx *sqlx.Tx, whereClause string, args ...interface{}) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM runs WHERE status = 'claimed' AND " + whereClause
	err := tx.QueryRowContext(ctx, tx.Rebind(query), args...).Scan(&count)
	return count, err
}

// countLedger counts office_launch_ledger rows matching whereClause,
// the durable record the launch budgets count against instead of
// runs.claimed_at (see office_launch_ledger's doc comment for why).
func (r *Repository) countLedger(ctx context.Context, tx *sqlx.Tx, whereClause string, args ...interface{}) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM office_launch_ledger WHERE " + whereClause
	err := tx.QueryRowContext(ctx, tx.Rebind(query), args...).Scan(&count)
	return count, err
}
