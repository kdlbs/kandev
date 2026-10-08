package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

// CopyWorkspaceStepsTx validates the entire included graph before inserting
// any step. Unknown references never retain a source workspace identity.
func (r *Repository) CopyWorkspaceStepsTx(ctx context.Context, tx *sqlx.Tx, workflowIDs map[string]string) ([]*models.WorkflowStep, error) {
	steps, err := r.readWorkspaceCloneSteps(ctx, tx, workflowIDs)
	if err != nil {
		return nil, err
	}
	stepIDs := make(map[string]string, len(steps))
	for _, step := range steps {
		stepIDs[step.ID] = uuid.NewString()
	}
	for _, step := range steps {
		if err := validateCloneStepReferences(step, stepIDs); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	for _, step := range steps {
		step.ID, step.WorkflowID = stepIDs[step.ID], workflowIDs[step.WorkflowID]
		step.Events = models.RemapStepEvents(step.Events, stepIDs)
		step.SessionTarget = models.RemapWorkflowSessionTarget(step.SessionTarget, stepIDs)
		step.PullFromStepID = models.RemapStepID(step.PullFromStepID, stepIDs)
		step.OrderRevision = 0
		step.CreatedAt, step.UpdatedAt = now, now
		if err := r.insertCloneStep(ctx, tx, step); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

func (r *Repository) readWorkspaceCloneSteps(ctx context.Context, tx *sqlx.Tx, workflowIDs map[string]string) ([]*models.WorkflowStep, error) {
	ids := make([]string, 0, len(workflowIDs))
	for id := range workflowIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var steps []*models.WorkflowStep
	for _, id := range ids {
		rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT `+stepSelectColumns+` FROM workflow_steps WHERE workflow_id = ? ORDER BY position, id`), id)
		if err != nil {
			return nil, err
		}
		batch, err := r.scanSteps(rows)
		closeErr := rows.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		steps = append(steps, batch...)
	}
	return steps, nil
}

func validateCloneStepReferences(step *models.WorkflowStep, stepIDs map[string]string) error {
	refs := models.CollectStepEventReferences(step.Events)
	if len(refs.TaskIDs) != 0 {
		return fmt.Errorf("%w: workflow step %q references an existing task", repoerrors.ErrWorkspaceCloneConfiguration, step.Name)
	}
	if err := models.ValidateWorkflowSessionTarget(step.SessionTarget); err != nil {
		return fmt.Errorf("%w: workflow step %q has an invalid session target", repoerrors.ErrWorkspaceCloneConfiguration, step.Name)
	}
	refs.StepIDs = append(refs.StepIDs, step.PullFromStepID)
	if step.SessionTarget != nil {
		refs.StepIDs = append(refs.StepIDs, step.SessionTarget.StepID)
	}
	for _, id := range refs.StepIDs {
		if id == "" {
			continue
		}
		if _, ok := stepIDs[id]; !ok {
			return fmt.Errorf("%w: workflow step %q references an excluded or missing step", repoerrors.ErrWorkspaceCloneConfiguration, step.Name)
		}
	}
	return nil
}

func (r *Repository) insertCloneStep(ctx context.Context, tx *sqlx.Tx, step *models.WorkflowStep) error {
	events, err := json.Marshal(step.Events)
	if err != nil {
		return err
	}
	target, err := marshalSessionTarget(step.SessionTarget)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position, color, prompt, events, allow_manual_move, is_start_step, show_in_command_panel, auto_archive_after_hours, agent_profile_id, profile_session_start_policy, profile_session_end_policy, disable_unclassified_fallback, stage_type, auto_advance_requires_signal, cancel_triggers_turn_complete, complete_task_on_enter, wip_limit, pull_from_step_id, session_target, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		step.ID, step.WorkflowID, step.Name, step.Position, step.Color, step.Prompt, string(events),
		dialect.BoolToInt(step.AllowManualMove), dialect.BoolToInt(step.IsStartStep), dialect.BoolToInt(step.ShowInCommandPanel),
		step.AutoArchiveAfterHours, step.AgentProfileID, step.ProfileSessionStartPolicy, step.ProfileSessionEndPolicy,
		dialect.BoolToInt(step.DisableUnclassifiedFallback), normalizeStageType(step.StageType),
		dialect.BoolToInt(step.AutoAdvanceRequiresSignal), dialect.BoolToInt(step.CancelTriggersTurnComplete),
		dialect.BoolToInt(step.CompleteTaskOnEnter), step.WIPLimit, step.PullFromStepID, target, step.CreatedAt, step.UpdatedAt)
	return err
}
