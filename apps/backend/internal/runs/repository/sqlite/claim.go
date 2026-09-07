package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

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

	for i := range candidates {
		candidate := &candidates[i]
		if blocked, gate := r.claimGateBlocks(ctx, tx, candidate, limits, budgetWindowStart); blocked {
			shared.LaunchDeferredTotal.Add(shared.LaunchSafetyLabel("gate", gate), 1)
			continue
		}
		claimed, err := r.commitClaim(ctx, tx, candidate)
		if err != nil {
			return nil, err
		}
		return claimed, nil
	}
	return nil, sql.ErrNoRows
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
		candidate.RoutineID, candidate.HumanRooted, claimedAt); err != nil {
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
// (AC-OFFICE-LAUNCH-SAFETY, "Failure and recovery").
func (r *Repository) claimGateBlocks(
	ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits, budgetWindowStart time.Time,
) (bool, string) {
	agentCap, err := r.agentCeiling(ctx, tx, candidate.AgentProfileID, limits)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateAgentCeiling), 1)
		return true, gateAgentCeiling
	}
	if agentCap <= 0 {
		// No resolvable agent_profiles row (or an invalid non-positive
		// value): a missing ceiling input defers rather than admitting
		// an unbounded agent, per AC-OFFICE-LAUNCH-SAFETY-001.8.
		return true, gateAgentCeiling
	}
	agentClaimed, err := r.countClaimed(ctx, tx, "agent_profile_id = ?", candidate.AgentProfileID)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateAgentCeiling), 1)
		return true, gateAgentCeiling
	}
	if agentClaimed >= agentCap {
		return true, gateAgentCeiling
	}

	workspaceClaimed, err := r.countClaimed(ctx, tx, "workspace_id = ?", candidate.WorkspaceID)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateWorkspaceCeiling), 1)
		return true, gateWorkspaceCeiling
	}
	if workspaceClaimed >= limits.MaxConcurrentWorkspace {
		return true, gateWorkspaceCeiling
	}

	instanceClaimed, err := r.countClaimed(ctx, tx, "1 = 1")
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateInstanceCeiling), 1)
		return true, gateInstanceCeiling
	}
	if instanceClaimed >= limits.MaxConcurrentInstance {
		return true, gateInstanceCeiling
	}

	if candidate.HumanRooted {
		// AC-OFFICE-LAUNCH-SAFETY-005.6: the budget exemption tests the
		// root of the chain, not this run's own (recomputed) priority
		// class, so it is checked from the persisted human_rooted flag.
		return false, ""
	}

	if candidate.RoutineID != "" {
		routineClaims, err := r.countLedger(ctx, tx, "routine_id = ? AND claimed_at > ?", candidate.RoutineID, budgetWindowStart)
		if err != nil {
			shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateRoutineBudget), 1)
			return true, gateRoutineBudget
		}
		if routineClaims >= limits.RoutineBudgetPerHour {
			return true, gateRoutineBudget
		}
	}

	workspaceClaims, err := r.countLedger(ctx, tx, "workspace_id = ? AND claimed_at > ?", candidate.WorkspaceID, budgetWindowStart)
	if err != nil {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateWorkspaceBudget), 1)
		return true, gateWorkspaceBudget
	}
	if workspaceClaims >= limits.WorkspaceBudgetPerHour {
		return true, gateWorkspaceBudget
	}

	return false, ""
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
