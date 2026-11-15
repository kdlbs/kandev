package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	officeHumanDoneIntent   = "office_done"
	officeHumanFieldsIntent = "fields"
)

// SetOfficeHumanCompletionPendingReader is configured before request admission.
func (r *Repository) SetOfficeHumanCompletionPendingReader(reader func(context.Context, *sql.Tx, string, string) ([]string, error)) {
	r.officeHumanCompletionPendingReader = reader
}
func (r *Repository) OfficeHumanCompletionGuardConfigured() bool {
	return r.officeHumanCompletionPendingReader != nil
}

func (r *Repository) guardOfficeHumanCompletionTx(ctx context.Context, tx *sql.Tx, candidate *models.Task) error {
	intent, human := models.OfficeHumanCompletionIntentFromContext(ctx)
	if !human || r.officeHumanCompletionPendingReader == nil {
		return nil
	}
	if intent.TaskID != candidate.ID {
		return repoerrors.ErrWorkflowChangeConflict
	}
	current, err := (taskHierarchyReader{r, tx}).GetTask(ctx, candidate.ID)
	if err != nil {
		return err
	}
	if err := prepareOfficeHumanCompletionCandidate(current, candidate, intent); err != nil {
		return err
	}
	if !current.IsFromOffice || intent.Kind == officeHumanFieldsIntent {
		return nil
	}
	if intent.Kind == "set_state" && !isNewOfficeCompletion(current, candidate) {
		return nil
	}
	if err := r.validateOfficeCompletionPolicyTx(ctx, tx, current, candidate, intent); err != nil {
		return err
	}
	if !isNewOfficeCompletion(current, candidate) {
		return nil
	}
	if err := r.validateOfficeCompletionWorkflowScopesTx(ctx, tx, current, candidate); err != nil {
		return err
	}
	pending, err := r.officeHumanCompletionPendingReader(ctx, tx, candidate.ID, candidate.WorkflowStepID)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return &repoerrors.OfficeApprovalsPendingError{AgentProfileIDs: pending}
	}
	return nil
}

func isNewOfficeCompletion(current, candidate *models.Task) bool {
	return candidate.State == v1.TaskStateCompleted && current.State != v1.TaskStateCompleted
}

func prepareOfficeHumanCompletionCandidate(current, candidate *models.Task, intent models.OfficeHumanCompletionIntent) error {
	if current.WorkspaceID != intent.ExpectedWorkspaceID || (intent.Kind != officeHumanFieldsIntent && (current.WorkflowID != intent.ExpectedWorkflowID || current.WorkflowStepID != intent.ExpectedStepID)) {
		return repoerrors.ErrWorkflowChangeConflict
	}
	// A move does not own project reassignment. Preserve the locked membership
	// instead of restoring a service snapshot over a concurrent project write.
	candidate.ProjectID = current.ProjectID
	candidate.IsFromOffice = current.IsFromOffice
	if intent.Kind == officeHumanDoneIntent && !current.IsFromOffice {
		return repoerrors.ErrWorkflowChangeConflict
	}
	if !current.IsFromOffice {
		return nil
	}
	// Move preparation derives state-dependent metadata from its source.
	if (intent.Kind == "move" || (intent.Kind == officeHumanDoneIntent && current.WorkflowStepID != candidate.WorkflowStepID)) && string(current.State) != intent.ExpectedTaskState {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}

func (r *Repository) CompleteOfficeTaskAsHuman(ctx context.Context, id, workspaceID, workflowID, stepID string) (*models.Task, error) {
	ctx = models.WithOfficeHumanCompletionIntent(ctx, models.OfficeHumanCompletionIntent{TaskID: id, ExpectedWorkspaceID: workspaceID, ExpectedWorkflowID: workflowID, ExpectedStepID: stepID, Kind: officeHumanDoneIntent})
	return r.updateHumanTaskState(ctx, id, v1.TaskStateCompleted, true)
}

func (r *Repository) UpdateTaskStateAsHuman(ctx context.Context, id string, state v1.TaskState) (*models.Task, error) {
	return r.updateHumanTaskState(ctx, id, state, false)
}

func (r *Repository) updateHumanTaskState(ctx context.Context, id string, state v1.TaskState, officeExplicit bool) (*models.Task, error) {
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := r.lockTaskFieldUpdate(ctx, tx, id, models.TaskFieldUpdate{})
	if err != nil {
		return nil, err
	}
	if officeExplicit && !task.IsFromOffice {
		return nil, repoerrors.ErrWorkflowChangeConflict
	}
	if officeExplicit && task.WorkflowStepID != "" {
		var final string
		if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT id FROM workflow_steps WHERE workflow_id=? ORDER BY position DESC,id DESC LIMIT 1`), task.WorkflowID).Scan(&final); err != nil {
			return nil, err
		}
		if final != task.WorkflowStepID {
			return nil, repoerrors.ErrWorkflowChangeConflict
		}
	}
	previousState := task.State
	task.State = state
	if err := r.guardOfficeHumanCompletionTx(ctx, tx, task); err != nil {
		return nil, err
	}
	if !officeExplicit {
		if err := r.guardTaskCompletionTransitionTx(ctx, tx, id, previousState, task.State, task.WorkflowID, task.WorkflowStepID, task.WorkflowID, task.WorkflowStepID); err != nil {
			return nil, err
		}
	}
	task.UpdatedAt = r.nowUTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET state=?,updated_at=? WHERE id=?`), task.State, task.UpdatedAt, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func (r *Repository) lockOfficeCompletionTaskScope(ctx context.Context, tx *sql.Tx, id string) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return r.lockTaskHierarchy(ctx, tx, nil, []string{id})
	}
	before, err := r.officeCompletionTaskScope(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := r.lockTaskHierarchy(ctx, tx, []string{before[0]}, nil); err != nil {
		return err
	}
	after, err := r.officeCompletionTaskScope(ctx, tx, id)
	if err != nil {
		return err
	}
	if before != after {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}
func (r *Repository) officeCompletionTaskScope(ctx context.Context, tx *sql.Tx, id string) ([3]string, error) {
	var scope [3]string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(workspace_id,''),COALESCE(workflow_id,''),COALESCE(workflow_step_id,'') FROM tasks WHERE id=?`), id).Scan(&scope[0], &scope[1], &scope[2])
	return scope, err
}

type officeCompletionStepPolicy struct {
	terminal bool
	limit    int
	finalID  string
}

func (r *Repository) officeCompletionStepPolicyTx(ctx context.Context, tx *sql.Tx, workflowID, stepID string) (officeCompletionStepPolicy, error) {
	var policy officeCompletionStepPolicy
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT
  (s.complete_task_on_enter=1 AND NOT EXISTS(SELECT 1 FROM workflow_steps n WHERE n.workflow_id=s.workflow_id AND n.position>s.position)),
  s.wip_limit,(SELECT id FROM workflow_steps f WHERE f.workflow_id=s.workflow_id ORDER BY f.position DESC,f.id DESC LIMIT 1)
  FROM workflow_steps s WHERE s.id=? AND s.workflow_id=?`), stepID, workflowID).Scan(&policy.terminal, &policy.limit, &policy.finalID)
	if errors.Is(err, sql.ErrNoRows) {
		return policy, repoerrors.ErrWorkflowChangeConflict
	}
	return policy, err
}
func (r *Repository) validateOfficeCompletionPolicyTx(ctx context.Context, tx *sql.Tx, current, candidate *models.Task, intent models.OfficeHumanCompletionIntent) error {
	if candidate.WorkflowStepID == "" {
		return nil
	}
	policy, err := r.officeCompletionStepPolicyTx(ctx, tx, candidate.WorkflowID, candidate.WorkflowStepID)
	if err != nil {
		return err
	}
	if intent.Kind == officeHumanDoneIntent && candidate.WorkflowStepID != policy.finalID {
		return repoerrors.ErrWorkflowChangeConflict
	}
	if intent.Kind == "set_state" {
		return nil
	}
	if current.WorkflowStepID == candidate.WorkflowStepID {
		return nil
	}
	if candidate.QueuedForStepID != "" {
		candidate.State = current.State
		return nil
	}
	if candidate.State == v1.TaskStateCompleted && current.State != v1.TaskStateCompleted && !policy.terminal {
		return repoerrors.ErrWorkflowChangeConflict
	}
	if policy.terminal && !models.IsTerminalTaskState(current.State) && candidate.State != v1.TaskStateCompleted {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}

// refreshOfficeAdmissionLimitTx evaluates actual capacity only for human Office
// admission. Engine and non-Office callers retain their existing policy path.
func (r *Repository) refreshOfficeAdmissionLimitTx(ctx context.Context, tx *sql.Tx, task *models.Task, limit int) (int, error) {
	if _, human := models.OfficeHumanCompletionIntentFromContext(ctx); !human || !r.OfficeHumanCompletionGuardConfigured() {
		return limit, nil
	}
	current, err := (taskHierarchyReader{r, tx}).GetTask(ctx, task.ID)
	if err != nil {
		return 0, err
	}
	if !current.IsFromOffice {
		return limit, nil
	}
	policy, err := r.officeCompletionStepPolicyTx(ctx, tx, task.WorkflowID, task.WorkflowStepID)
	return policy.limit, err
}

func (r *Repository) lockWorkflowCompletionPolicyTx(ctx context.Context, tx *sqlx.Tx, id string) error {
	var workspace string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM workflows WHERE id=?`), id).Scan(&workspace)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := r.lockTaskHierarchy(ctx, tx, []string{workspace}, nil); err != nil {
		return err
	}
	var current string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM workflows WHERE id=?`), id).Scan(&current); err != nil {
		return err
	}
	if current != workspace {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}

func (r *Repository) validateOfficeCompletionWorkflowScopesTx(ctx context.Context, tx *sql.Tx, current, candidate *models.Task) error {
	for _, workflowID := range []string{current.WorkflowID, candidate.WorkflowID} {
		if workflowID == "" {
			continue
		}
		var workspace string
		err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workspace_id FROM workflows WHERE id=?`), workflowID).Scan(&workspace)
		if errors.Is(err, sql.ErrNoRows) {
			return repoerrors.ErrWorkflowChangeConflict
		}
		if err != nil {
			return err
		}
		if workspace != current.WorkspaceID {
			return repoerrors.ErrWorkflowChangeConflict
		}
	}
	if current.WorkflowStepID != "" {
		if _, err := r.officeCompletionStepPolicyTx(ctx, tx, current.WorkflowID, current.WorkflowStepID); err != nil {
			return err
		}
	}
	return nil
}
