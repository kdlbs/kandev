package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) CreateOrGetCompletionIntent(ctx context.Context, intent *models.CompletionIntent) (bool, *models.CompletionIntent, error) {
	if intent == nil || intent.ID == "" || intent.SessionID == "" || intent.TurnID == "" || intent.WorkflowStepID == "" {
		return false, nil, fmt.Errorf("completion intent requires id, session, turn, and step")
	}
	if intent.State == "" {
		intent.State = models.CompletionIntentStatePending
	}
	if intent.State != models.CompletionIntentStatePending {
		return false, nil, fmt.Errorf("completion intent must be created in pending state, got %q", intent.State)
	}
	// The turn row is the single source of truth for which task and session
	// own it. Individual foreign keys on task_id/session_id/turn_id each
	// reference a valid row, but do not require those rows to agree with each
	// other, so a caller could otherwise record an intent under an unrelated
	// task. Deriving/validating identity from the turn closes that gap.
	var turnTaskID, turnSessionID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT task_id, task_session_id FROM task_session_turns WHERE id = ?
	`), intent.TurnID).Scan(&turnTaskID, &turnSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil, fmt.Errorf("completion intent turn not found: %s", intent.TurnID)
		}
		return false, nil, fmt.Errorf("resolve completion intent turn identity: %w", err)
	}
	if turnSessionID != intent.SessionID {
		return false, nil, fmt.Errorf("completion intent turn %s belongs to session %s, not %s", intent.TurnID, turnSessionID, intent.SessionID)
	}
	if turnTaskID != intent.TaskID {
		return false, nil, fmt.Errorf("completion intent task %s does not match turn %s's task %s", intent.TaskID, intent.TurnID, turnTaskID)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO session_completion_intents (
			id, task_id, session_id, turn_id, workflow_step_id, agent_execution_id, prompt_generation,
			state, summary, handoff, blockers, requested_at, last_post_signal_activity_at, eligible_at, outcome
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, turn_id, workflow_step_id) DO NOTHING
	`), intent.ID, intent.TaskID, intent.SessionID, intent.TurnID, intent.WorkflowStepID, intent.AgentExecutionID,
		intent.PromptGeneration, intent.State, intent.Summary, intent.Handoff, intent.Blockers, intent.RequestedAt,
		nullTime(intent.LastPostSignalActivityAt), intent.EligibleAt, intent.Outcome)
	if err != nil {
		return false, nil, fmt.Errorf("insert completion intent: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, nil, fmt.Errorf("inspect completion intent insert: %w", err)
	}
	stored, err := r.getCompletionIntentByIdentity(ctx, intent.SessionID, intent.TurnID, intent.WorkflowStepID)
	if err != nil {
		return false, nil, err
	}
	return affected == 1, stored, nil
}

func (r *Repository) GetCompletionIntent(ctx context.Context, id string) (*models.CompletionIntent, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(completionIntentSelect+` WHERE id = ?`), id)
	return scanCompletionIntent(row)
}

// GetCompletionIntentForTurn returns the most recently requested completion
// intent for (sessionID, turnID). The schema's uniqueness constraint is on
// (session_id, turn_id, workflow_step_id), so the same turn can accumulate
// more than one intent row if it signals completion again under a later
// step after a workflow move. Ordering by most recent, rather than first,
// ensures a caller settling "by turn" resolves the intent that reflects
// current reality — an older row for an already-superseded step must not
// shadow a newer pending intent for the step the task is actually on.
func (r *Repository) GetCompletionIntentForTurn(ctx context.Context, sessionID, turnID string) (*models.CompletionIntent, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(completionIntentSelect+`
		WHERE session_id = ? AND turn_id = ?
		ORDER BY requested_at DESC, id DESC
		LIMIT 1`), sessionID, turnID)
	return scanCompletionIntent(row)
}

func (r *Repository) ListDueCompletionIntents(ctx context.Context, now time.Time, limit int) ([]*models.CompletionIntent, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(completionIntentSelect+`
		WHERE state = ? AND eligible_at <= ?
		ORDER BY eligible_at ASC, requested_at ASC, id ASC
		LIMIT ?`), models.CompletionIntentStatePending, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due completion intents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	intents := make([]*models.CompletionIntent, 0)
	for rows.Next() {
		intent, scanErr := scanCompletionIntent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		intents = append(intents, intent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due completion intents: %w", err)
	}
	return intents, nil
}

// CountPendingCompletionIntents supplies the restart-safe pending gauge. The
// reconciler uses this indexed state count instead of deriving a value from
// process-local workers or active-turn caches.
func (r *Repository) CountPendingCompletionIntents(ctx context.Context) (int, error) {
	var count int
	if err := r.ro.GetContext(ctx, &count, r.ro.Rebind(`
		SELECT COUNT(*) FROM session_completion_intents WHERE state = ?
	`), models.CompletionIntentStatePending); err != nil {
		return 0, fmt.Errorf("count pending completion intents: %w", err)
	}
	return count, nil
}

// ClaimCompletionIntentForSettlement performs the pending -> settling
// compare-and-set and stamps eligible_at with the settling lease deadline in
// one statement. A process that crashes (or is killed) after this claim
// commits but before finishing settlement leaves the row visibly "settling"
// forever unless something re-scans it: ListDueCompletionIntents only ever
// selects pending rows, so a bare state flip with no expiry would make the
// claim permanent. Repurposing eligible_at as the settling lease deadline
// (rather than a new column) lets ReclaimAbandonedSettlingCompletionIntents
// use the same due-scan shape as the pending path.
func (r *Repository) ClaimCompletionIntentForSettlement(ctx context.Context, id string, now, leaseUntil time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_completion_intents
		SET state = ?, eligible_at = ?
		WHERE id = ? AND state = ?
	`), models.CompletionIntentStateSettling, leaseUntil, id, models.CompletionIntentStatePending)
	if err != nil {
		return false, fmt.Errorf("claim completion intent for settlement: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect completion intent settlement claim: %w", err)
	}
	return affected == 1, nil
}

// ReleaseCompletionIntentSettlingClaim returns a claimed intent to pending
// immediately, resetting eligible_at to now so the very next due scan
// retries it rather than waiting out the settling lease. Used by in-process
// settlement code that already knows its own claim failed transiently (a
// turn-completion write error, a task/session lookup failure) — as opposed
// to ReclaimAbandonedSettlingCompletionIntents' time-based recovery for a
// claim nothing remains to release.
func (r *Repository) ReleaseCompletionIntentSettlingClaim(ctx context.Context, id string, now time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_completion_intents
		SET state = ?, eligible_at = ?
		WHERE id = ? AND state = ?
	`), models.CompletionIntentStatePending, now, id, models.CompletionIntentStateSettling)
	if err != nil {
		return false, fmt.Errorf("release completion intent settling claim: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect completion intent settling release: %w", err)
	}
	return affected == 1, nil
}

// ReclaimAbandonedSettlingCompletionIntents returns any settling intent whose
// lease (stored in eligible_at by ClaimCompletionIntentForSettlement) has
// expired back to pending, with eligible_at reset to now so the same due
// scan can pick it back up. This is the crash-recovery path: a process that
// died mid-settlement leaves its claim with no in-process handler left to
// release it, and only a lease-based reclaim can ever make that row visible
// to ListDueCompletionIntents again. Must run before every due scan
// (including the startup scan), not only the periodic ticker, since a crash
// during startup recovery is exactly the case this exists to cover.
func (r *Repository) ReclaimAbandonedSettlingCompletionIntents(ctx context.Context, now time.Time) (int, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_completion_intents
		SET state = ?, eligible_at = ?
		WHERE state = ? AND eligible_at <= ?
	`), models.CompletionIntentStatePending, now, models.CompletionIntentStateSettling, now)
	if err != nil {
		return 0, fmt.Errorf("reclaim abandoned settling completion intents: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("inspect reclaimed completion intents: %w", err)
	}
	return int(affected), nil
}

func (r *Repository) RearmCompletionIntent(ctx context.Context, id string, activityAt, eligibleAt time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_completion_intents
		SET last_post_signal_activity_at = ?, eligible_at = ?
		WHERE id = ? AND state = ?
	`), activityAt, eligibleAt, id, models.CompletionIntentStatePending)
	if err != nil {
		return false, fmt.Errorf("rearm completion intent: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect rearmed completion intent: %w", err)
	}
	return affected == 1, nil
}

func (r *Repository) getCompletionIntentByIdentity(ctx context.Context, sessionID, turnID, stepID string) (*models.CompletionIntent, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(completionIntentSelect+` WHERE session_id = ? AND turn_id = ? AND workflow_step_id = ?`), sessionID, turnID, stepID)
	return scanCompletionIntent(row)
}

func (r *Repository) TransitionCompletionIntent(ctx context.Context, id string, from, to models.CompletionIntentState, settledAt time.Time) (bool, error) {
	if !from.CanTransitionTo(to) {
		return false, fmt.Errorf("illegal completion intent transition %q -> %q", from, to)
	}
	var completedAt interface{}
	if to == models.CompletionIntentStateSettled || to == models.CompletionIntentStateReopened ||
		to == models.CompletionIntentStateSuperseded || to == models.CompletionIntentStateRejected {
		completedAt = settledAt
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_completion_intents
		SET state = ?, settled_at = COALESCE(?, settled_at)
		WHERE id = ? AND state = ?
	`), to, completedAt, id, from)
	if err != nil {
		return false, fmt.Errorf("transition completion intent: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

// TransitionCompletionIntentWithControlEvent atomically finishes a completion
// intent's terminal transition and records its authorized session-control
// audit event in one transaction. Manual stale-session settlement previously
// performed the transition and the audit insert as two separate statements:
// a crash (or any failure) between them left a durably settled turn with no
// audit trail of who authorized it, and a retry of the same request would
// see the intent already settled and reject as "completion_evidence_missing"
// instead of ever recording the original authorization. Either both facts
// commit here, or neither does.
func (r *Repository) TransitionCompletionIntentWithControlEvent(
	ctx context.Context, id string, from, to models.CompletionIntentState, settledAt time.Time,
	event *models.SessionControlEvent,
) (bool, error) {
	if !from.CanTransitionTo(to) {
		return false, fmt.Errorf("illegal completion intent transition %q -> %q", from, to)
	}
	if err := validateSessionControlEvent(event); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin completion intent settlement audit tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	transitioned, err := r.transitionCompletionIntentTx(ctx, tx, id, from, to, settledAt)
	if err != nil || !transitioned {
		// A no-op compare-and-set (already transitioned by a concurrent
		// caller) must not record an audit event for a settlement this call
		// did not perform. The deferred Rollback discards the transaction
		// either way.
		return false, err
	}
	if err := insertSessionControlEventTx(ctx, tx, r.db, event); err != nil {
		return false, fmt.Errorf("record completion intent settlement audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit completion intent settlement audit: %w", err)
	}
	return true, nil
}

func (r *Repository) transitionCompletionIntentTx(
	ctx context.Context, tx *sqlx.Tx, id string, from, to models.CompletionIntentState, settledAt time.Time,
) (bool, error) {
	var completedAt interface{}
	if to == models.CompletionIntentStateSettled || to == models.CompletionIntentStateReopened ||
		to == models.CompletionIntentStateSuperseded || to == models.CompletionIntentStateRejected {
		completedAt = settledAt
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE session_completion_intents
		SET state = ?, settled_at = COALESCE(?, settled_at)
		WHERE id = ? AND state = ?
	`), to, completedAt, id, from)
	if err != nil {
		return false, fmt.Errorf("transition completion intent: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect completion intent transition: %w", err)
	}
	return affected == 1, nil
}

func validateSessionControlEvent(event *models.SessionControlEvent) error {
	if event == nil || event.ActorTaskID == "" || event.ActorSessionID == "" ||
		event.TargetTaskID == "" || event.TargetSessionID == "" || event.TargetTurnID == "" ||
		event.AuthorityBasis == "" || event.EvidenceCode == "" || event.Result == "" {
		return fmt.Errorf("session control event requires actor, target, authority, evidence, and result")
	}
	return nil
}

func insertSessionControlEventTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, event *models.SessionControlEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO session_control_events (
			id, actor_task_id, actor_session_id, target_task_id, target_session_id,
			target_turn_id, authority_basis, evidence_code, result, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), event.ID, event.ActorTaskID, event.ActorSessionID, event.TargetTaskID,
		event.TargetSessionID, event.TargetTurnID, event.AuthorityBasis,
		event.EvidenceCode, event.Result, event.CreatedAt)
	return err
}

const completionIntentSelect = `
	SELECT id, task_id, session_id, turn_id, workflow_step_id, agent_execution_id, prompt_generation,
	       state, summary, handoff, blockers, requested_at, last_post_signal_activity_at, eligible_at,
	       settled_at, outcome
	FROM session_completion_intents`

type completionIntentScanner interface{ Scan(...interface{}) error }

func scanCompletionIntent(scanner completionIntentScanner) (*models.CompletionIntent, error) {
	intent := &models.CompletionIntent{}
	var activityAt, settledAt sql.NullTime
	err := scanner.Scan(&intent.ID, &intent.TaskID, &intent.SessionID, &intent.TurnID, &intent.WorkflowStepID,
		&intent.AgentExecutionID, &intent.PromptGeneration, &intent.State, &intent.Summary, &intent.Handoff,
		&intent.Blockers, &intent.RequestedAt, &activityAt, &intent.EligibleAt, &settledAt, &intent.Outcome)
	if err != nil {
		return nil, err
	}
	if activityAt.Valid {
		intent.LastPostSignalActivityAt = activityAt.Time
	}
	if settledAt.Valid {
		intent.SettledAt = &settledAt.Time
	}
	return intent, nil
}

func nullTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value
}
