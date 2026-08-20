package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) CreateOrGetCompletionIntent(ctx context.Context, intent *models.CompletionIntent) (bool, *models.CompletionIntent, error) {
	if intent == nil || intent.ID == "" || intent.SessionID == "" || intent.TurnID == "" || intent.WorkflowStepID == "" {
		return false, nil, fmt.Errorf("completion intent requires id, session, turn, and step")
	}
	if intent.State == "" {
		intent.State = models.CompletionIntentStatePending
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
	if to == models.CompletionIntentStateSettled || to == models.CompletionIntentStateSuperseded || to == models.CompletionIntentStateRejected {
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
