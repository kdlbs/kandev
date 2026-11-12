package repository

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
	"strings"
	"time"
)

func (r *Repository) insertStepRecord(ctx context.Context, tx *sqlx.Tx, step *models.WorkflowStep, eventsJSON []byte, sessionTargetJSON interface{}, expectedWorkflow *time.Time) error {
	insertQuery := `
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, color,
			prompt, events, allow_manual_move, is_start_step, show_in_command_panel, auto_archive_after_hours, agent_profile_id, profile_session_start_policy, profile_session_end_policy, disable_unclassified_fallback, stage_type, auto_advance_requires_signal, cancel_triggers_turn_complete, complete_task_on_enter, wip_limit, pull_from_step_id, session_target, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	args := []interface{}{step.ID, step.WorkflowID, step.Name, step.Position, step.Color,
		step.Prompt, string(eventsJSON), dialect.BoolToInt(step.AllowManualMove),
		dialect.BoolToInt(step.IsStartStep), dialect.BoolToInt(step.ShowInCommandPanel), step.AutoArchiveAfterHours, step.AgentProfileID, taskmodels.NormalizeWorkflowProfileSessionStartPolicy(string(step.ProfileSessionStartPolicy)), taskmodels.NormalizeWorkflowProfileSessionEndPolicy(string(step.ProfileSessionEndPolicy)), dialect.BoolToInt(step.DisableUnclassifiedFallback), normalizeStageType(step.StageType), dialect.BoolToInt(step.AutoAdvanceRequiresSignal), dialect.BoolToInt(step.CancelTriggersTurnComplete), dialect.BoolToInt(step.CompleteTaskOnEnter), step.WIPLimit, step.PullFromStepID, sessionTargetJSON, step.CreatedAt, step.UpdatedAt}
	if expectedWorkflow != nil {
		insertQuery = strings.Replace(insertQuery, `) VALUES (`, `) SELECT `, 1)
		insertQuery = strings.TrimSuffix(strings.TrimSpace(insertQuery), ")") + ` WHERE EXISTS (SELECT 1 FROM workflows WHERE id = ? AND updated_at = ?)`
		args = append(args, step.WorkflowID, *expectedWorkflow)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(insertQuery), args...)
	if err != nil {
		return err
	}
	if expectedWorkflow != nil {
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if rows == 0 {
			return repoerrors.ErrTaskVersionConflict
		}
	}

	return nil
}
