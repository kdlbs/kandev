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
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/models"
)

// claimCandidatePageSize is how many queued rows one page of the
// candidate scan fetches at a time. The claim statement can no longer
// select its answer in one predicate once the ceilings and budgets are
// per-agent/per-workspace/per-routine instead of a single "count = 0"
// lock, so candidates are read in claim order, a page at a time, and
// evaluated one at a time until one clears every gate.
//
// A single fixed-size page is fetched once, not looped: an earlier
// version stopped after the first claimCandidateBatchSize=20 rows and
// returned sql.ErrNoRows if none of those cleared every gate, even when
// row 21 was fully claimable. That let one saturated agent or workspace
// with >=20 queued rows ranked ahead of everything else starve every
// other workspace's claim attempts indefinitely, once its own queued
// rows permanently occupied the whole candidate window — the opposite of
// what REQ-OFFICE-BACKPRESSURE-002's age promotion exists to prevent.
// ClaimNextEligibleRun now pages through claimCandidateScanCap rows
// total (in claimCandidatePageSize chunks, within the same transaction)
// before giving up, so a lower-priority-but-claimable row is only missed
// once the scan's generous worst-case bound is exhausted, not after 20
// rows.
const claimCandidatePageSize = 20

// claimCandidateScanCap bounds the total number of queued rows one
// ClaimNextEligibleRun call will inspect across every page before
// deferring. This is a circuit breaker against a pathological backlog
// (hundreds of thousands of queued rows) making a single claim attempt
// scan unboundedly, not a correctness boundary: hitting it is recorded
// via shared.LaunchClaimScanCapHitTotal so it is visible to operators,
// and the attempt defers (returns sql.ErrNoRows) exactly like "no
// candidate cleared every gate" — it never admits a row it didn't
// actually evaluate.
const claimCandidateScanCap = 2000

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
	// than PromotionAge claims as one class better. Only classes above
	// `recovery` (1) are eligible: `human` (0) and `recovery` (1) are
	// left untouched so a human run's promotion is always a no-op.
	const promotedClass = "CASE WHEN w.requested_at < ? AND w.priority_class > 1 THEN w.priority_class - 1 ELSE w.priority_class END"
	query := fmt.Sprintf(`
		SELECT w.* FROM runs w
		WHERE w.status = 'queued'
		  AND (w.scheduled_retry_at IS NULL OR w.scheduled_retry_at <= ?)
		  AND w.routing_blocked_status IS NULL
		ORDER BY %s ASC, w.requested_at ASC, w.id ASC
		LIMIT ? OFFSET ?
	`, promotedClass)

	claimed, scan, err := r.scanCandidatePages(ctx, tx, query, now, promotionCutoff, limits, budgetWindowStart)
	if err != nil {
		return nil, err
	}
	if claimed != nil {
		return claimed, nil
	}
	if scan.capHit {
		shared.LaunchClaimScanCapHitTotal.Add(1)
		if r.log != nil {
			r.log.Warn("claim candidate scan cap reached without exhausting the queued set",
				zap.Int("scan_cap", claimCandidateScanCap))
		}
	}
	if scan.attributedGate != "" {
		shared.LaunchDeferredTotal.Add(shared.LaunchSafetyLabel("gate", scan.attributedGate), 1)
		r.logDeferral(scan.attributedGate, scan.attributedRun)
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

// candidateScanResult carries scanCandidatePages's no-claim findings back
// to its caller: the gate attributed to the highest-priority blocked
// candidate seen (AC-OFFICE-BACKPRESSURE-003.6/.7), and whether the scan
// stopped because it hit claimCandidateScanCap rather than exhausting the
// queued set.
type candidateScanResult struct {
	attributedGate string
	attributedRun  *models.Run
	capHit         bool
}

// scanCandidatePages pages through query in claimCandidatePageSize chunks,
// up to claimCandidateScanCap rows total, evaluating each candidate's gates
// in claim order and committing+returning the first one that clears every
// gate. Returns a nil *models.Run (with the scan's findings) when no
// candidate within the scan cap was claimable.
func (r *Repository) scanCandidatePages(
	ctx context.Context, tx *sqlx.Tx, query string, now, promotionCutoff time.Time,
	limits ClaimSafetyLimits, budgetWindowStart time.Time,
) (*models.Run, candidateScanResult, error) {
	result := candidateScanResult{capHit: true}
	for offset := 0; offset < claimCandidateScanCap; offset += claimCandidatePageSize {
		var page []models.Run
		// Positional placeholder order follows the assembled query text:
		// `scheduled_retry_at <= ?` (WHERE) binds first, then
		// `requested_at < ?` (the promotion CASE in ORDER BY), then the
		// LIMIT/OFFSET pair.
		if err := tx.SelectContext(ctx, &page, tx.Rebind(query), now, promotionCutoff, claimCandidatePageSize, offset); err != nil {
			return nil, result, err
		}
		if len(page) == 0 {
			result.capHit = false
			break
		}
		claimed, err := r.claimFirstEligible(ctx, tx, page, limits, budgetWindowStart, &result)
		if err != nil {
			return nil, result, err
		}
		if claimed != nil {
			return claimed, result, nil
		}
		if len(page) < claimCandidatePageSize {
			// Reached the end of the queued set on this page: no need to
			// issue another (empty) page fetch.
			result.capHit = false
			break
		}
	}
	return nil, result, nil
}

// claimFirstEligible evaluates page in order, claiming and returning the
// first candidate that clears every gate. Every blocked candidate's gate
// is recorded into result.attributedGate/attributedRun, but only the
// first one seen (across every call for one scan) is kept, per
// candidateScanResult's attribution rule.
func (r *Repository) claimFirstEligible(
	ctx context.Context, tx *sqlx.Tx, page []models.Run, limits ClaimSafetyLimits,
	budgetWindowStart time.Time, result *candidateScanResult,
) (*models.Run, error) {
	for i := range page {
		candidate := &page[i]
		if blocked, gate := r.claimGateBlocks(ctx, tx, candidate, limits, budgetWindowStart); blocked {
			if result.attributedGate == "" {
				result.attributedGate = gate
				result.attributedRun = candidate
			}
			continue
		}
		return r.commitClaim(ctx, tx, candidate)
	}
	return nil, nil
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
// row, then commits the transaction the caller began. The UPDATE is a
// compare-and-swap on status='queued': on READ COMMITTED isolation, a
// concurrent cancel can commit between the candidate scan's SELECT and
// this UPDATE, so a blind write would silently resurrect a cancelled
// run as claimed. Zero rows affected means the row was in fact no
// longer eligible: commitClaim returns sql.ErrNoRows, which propagates
// through claimFirstEligible and scanCandidatePages to
// ClaimNextEligibleRun, whose deferred rollback discards this tick's
// gate outcomes along with the abandoned claim. The next tick re-scans
// from scratch rather than resuming mid-page.
func (r *Repository) commitClaim(ctx context.Context, tx *sqlx.Tx, candidate *models.Run) (*models.Run, error) {
	claimedAt := time.Now().UTC()
	res, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ? AND status = 'queued'`,
	), claimedAt, candidate.ID)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, sql.ErrNoRows
	}
	ledgerID := uuid.New().String()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO office_launch_ledger (
			id, run_id, workspace_id, causation_id, routine_id, human_rooted, claimed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`), ledgerID, candidate.ID, candidate.WorkspaceID, candidate.ChainCausationID,
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

// isShutdownCanceled reports whether err (from a gate's DB read) is a
// context cancellation rather than a genuine unreadable-input failure.
// AC-OFFICE-LAUNCH-SAFETY-001.8: "A shutdown-cancelled evaluation shall
// defer the run without recording a gate failure, so a restart is not
// mistaken for a gate that is failing closed."
func isShutdownCanceled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
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
		if isShutdownCanceled(ctx, err) {
			return true
		}
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateAgentCeiling), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateAgentCeiling, false)
		return true
	}
	if agentCap <= 0 {
		// No resolvable agent_profiles row: defer rather than admit an
		// unbounded agent. AC-OFFICE-LAUNCH-SAFETY-001.8 names this
		// alongside an unreadable ceiling input, not an ordinary
		// saturated-pool block, so it records the same gate failure.
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", gateAgentCeiling), 1)
		r.recordGateOutcome(ctx, tx, candidate.WorkspaceID, gateAgentCeiling, false)
		return true
	}
	agentClaimed, err := r.countClaimed(ctx, tx, "agent_profile_id = ?", candidate.AgentProfileID)
	if err != nil {
		if isShutdownCanceled(ctx, err) {
			return true
		}
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
		if isShutdownCanceled(ctx, err) {
			return true
		}
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
		if isShutdownCanceled(ctx, err) {
			return true
		}
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
		if isShutdownCanceled(ctx, err) {
			return true
		}
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
		if isShutdownCanceled(ctx, err) {
			return true
		}
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
	// A missing row (above) is deferred as unbounded-looking input; a
	// stored value of zero or negative is a configuration mistake, not
	// an intentional "no capacity" signal, so it floors to 1 rather than
	// blocking the agent's queue forever.
	if maxSessions < 1 {
		maxSessions = 1
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
