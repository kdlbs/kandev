package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// SetCompletionPolicies registers domain policies before the repository serves
// requests.
func (r *Repository) SetCompletionPolicies(policies ...completion.Policy) {
	r.completionPolicies = append([]completion.Policy(nil), policies...)
}

// CompletionPoliciesConfigured reports whether a policy hook was registered.
func (r *Repository) CompletionPoliciesConfigured() bool {
	return len(r.completionPolicies) > 0
}

// CheckCompletionAdmission runs the completion gate and registered domain
// policies through a read-only reader transaction. The write path repeats
// these checks under its task lock before committing.
func (r *Repository) CheckCompletionAdmission(
	ctx context.Context,
	current, candidate *models.Task,
	admission *completion.Admission,
) error {
	tx, err := r.beginTaskCompletionGateRead(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.preflightTaskCompletionGateTx(ctx, tx, current, candidate); err != nil {
		return err
	}
	if err := r.admitCompletionTx(ctx, tx, admission, candidate); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) preflightTaskCompletionGateTx(
	ctx context.Context,
	tx *sql.Tx,
	current, candidate *models.Task,
) error {
	if current == nil || candidate == nil {
		return nil
	}
	completingMove, err := r.isCompletingTaskMoveTx(ctx, tx, current, candidate)
	if err != nil {
		return err
	}
	if !completingMove {
		return nil
	}
	if err := r.checkRequiredSubtaskAssignmentsTx(ctx, tx, current.ID); err != nil {
		return err
	}
	if !needsTaskCompletionGate(current, candidate) {
		return nil
	}
	return r.checkTaskCompletionMoveOverrideTx(ctx, tx, current, candidate)
}

func needsTaskCompletionGate(current, candidate *models.Task) bool {
	if taskTransitionsToCompleted(current, candidate) {
		return true
	}
	stepID := candidate.WorkflowStepID
	if stepID == "" || (!candidate.IsEphemeral && !candidate.WIPAdmitted && candidate.QueuedForStepID == stepID) {
		return false
	}
	// A task queued in the destination enters it when the queue marker is
	// cleared and admission is granted, even if its step ID stays the same.
	return current.WorkflowID != candidate.WorkflowID || current.WorkflowStepID != stepID ||
		(!current.WIPAdmitted && current.QueuedForStepID == stepID && candidate.WIPAdmitted)
}

func taskTransitionsToCompleted(current, candidate *models.Task) bool {
	return candidate.State == v1.TaskStateCompleted && current.State != v1.TaskStateCompleted
}

// completeAutomaticStepEntryTx applies completion before the task revision
// changes. Office destinations use the Office approval and status pipeline.
func (r *Repository) completeAutomaticStepEntryTx(ctx context.Context, tx *sql.Tx, task *models.Task) error {
	if models.IsTerminalTaskState(task.State) {
		return nil
	}
	completing, err := r.isCompletingWorkflowStepTx(ctx, tx, task.WorkflowID, task.WorkflowStepID)
	if err != nil || !completing {
		return err
	}
	query := `SELECT ` + isFromOfficeProjection("destination") + ` FROM (
		SELECT workspace_id, CAST(? AS TEXT) AS workflow_id, project_id FROM tasks WHERE id = ?
	) destination`
	var office bool
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), task.WorkflowID, task.ID).Scan(&office); err != nil {
		return err
	}
	if !office {
		task.State = v1.TaskStateCompleted
	}
	return nil
}

func (r *Repository) isCompletingTaskMoveTx(
	ctx context.Context,
	tx *sql.Tx,
	current, candidate *models.Task,
) (bool, error) {
	if taskTransitionsToCompleted(current, candidate) {
		return true, nil
	}
	if candidate.WorkflowID == "" || candidate.WorkflowStepID == "" {
		return false, nil
	}
	return r.isCompletingWorkflowStepTx(ctx, tx, candidate.WorkflowID, candidate.WorkflowStepID)
}

// readTaskCompletionMoveSourceTx reads admission fields after the task is locked.
func (r *Repository) readTaskCompletionMoveSourceTx(
	ctx context.Context, tx *sql.Tx, taskID, workflowID, stepID string,
) (models.Task, error) {
	current := models.Task{ID: taskID, WorkflowID: workflowID, WorkflowStepID: stepID}
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT state, execution_grade, wip_admitted = 1, COALESCE(queued_for_step_id, '') FROM tasks WHERE id = ?
	`), taskID).Scan(&current.State, &current.ExecutionGrade, &current.WIPAdmitted, &current.QueuedForStepID)
	return current, err
}

func (r *Repository) checkRequiredSubtaskAssignmentsTx(ctx context.Context, tx *sql.Tx, taskID string) error {
	required, assigned, err := r.readRequiredSubtaskAssignmentsTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if required > assigned {
		return fmt.Errorf("%w: required subtask assignments incomplete (%d/%d)", repoerrors.ErrTaskCompletionGateBlocked, assigned, required)
	}
	return nil
}

func (r *Repository) checkTaskCompletionMoveOverrideTx(
	ctx context.Context,
	tx *sql.Tx,
	current, candidate *models.Task,
) error {
	snapshot, err := r.readTaskCompletionGateTx(ctx, tx, current.ID, false)
	if err != nil {
		return err
	}
	if len(snapshot.Criteria) == 0 || len(snapshot.Blockers) == 0 {
		return nil
	}
	_, err = validateTaskCompletionMoveOverride(ctx, current, candidate, snapshot)
	return err
}

func validateTaskCompletionMoveOverride(
	ctx context.Context, current, candidate *models.Task, snapshot *models.TaskCompletionGateSnapshot,
) (models.TaskCompletionMoveOverride, error) {
	override, ok := models.TaskCompletionMoveOverrideFromContext(ctx)
	if !ok {
		return override, repoerrors.ErrTaskCompletionGateBlocked
	}
	if override.TaskID != current.ID || override.WorkspaceID != snapshot.WorkspaceID ||
		override.ExpectedRevision != snapshot.Revision ||
		override.SourceWorkflowID != current.WorkflowID || override.SourceStepID != current.WorkflowStepID ||
		override.TargetWorkflowID != candidate.WorkflowID || override.TargetStepID != candidate.WorkflowStepID ||
		strings.TrimSpace(override.ActorID) == "" || strings.TrimSpace(override.Reason) == "" {
		return override, repoerrors.ErrTaskCompletionCriteriaConflict
	}
	return override, nil
}

func (r *Repository) admitCompletionTx(
	ctx context.Context,
	tx *sql.Tx,
	admission *completion.Admission,
	candidate *models.Task,
) error {
	if admission == nil {
		return nil
	}
	if err := validateCompletionAdmission(admission, candidate); err != nil {
		return err
	}
	current, err := (taskHierarchyReader{r, tx}).GetTask(ctx, candidate.ID)
	if err != nil {
		return err
	}
	if current.WorkspaceID != admission.Expected.WorkspaceID {
		return repoerrors.ErrWorkflowChangeConflict
	}
	// A move owns placement, not workspace or project membership. Rebase the
	// candidate on the membership observed under the transaction lock.
	candidate.WorkspaceID = current.WorkspaceID
	candidate.ProjectID = current.ProjectID
	if err := validateCompletionAdmissionScope(current, candidate, admission); err != nil {
		return err
	}
	if err := r.validateAdmissionStepTx(ctx, tx, current, candidate, admission); err != nil {
		return err
	}

	for _, policy := range r.completionPolicies {
		if err := policy.AdmitCompletion(ctx, tx, current, candidate, *admission); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) validateAdmissionStepTx(
	ctx context.Context,
	tx *sql.Tx,
	current, candidate *models.Task,
	admission *completion.Admission,
) error {
	switch admission.Kind {
	case completion.Move:
		return r.validateMoveAdmissionStepTx(ctx, tx, current, candidate)
	case completion.SetState:
		return r.validateSetStateAdmissionStepTx(ctx, tx, current, candidate)
	default:
		return nil
	}
}

func (r *Repository) validateMoveAdmissionStepTx(
	ctx context.Context,
	tx *sql.Tx,
	current, candidate *models.Task,
) error {
	// Queued moves have not entered the target step yet, so they do not
	// complete a task or consume completion approval. The assignment gate
	// still runs in the read-only preflight for a completing destination.
	if !candidate.IsEphemeral && !candidate.WIPAdmitted {
		return nil
	}
	completing, err := r.isCompletingWorkflowStepTx(ctx, tx, candidate.WorkflowID, candidate.WorkflowStepID)
	if err != nil {
		return err
	}
	completesTask := taskTransitionsToCompleted(current, candidate)
	if completesTask && !completing {
		return repoerrors.ErrWorkflowChangeConflict
	}
	if completing && !completesTask && !models.IsTerminalTaskState(current.State) {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}

func (r *Repository) validateSetStateAdmissionStepTx(
	ctx context.Context,
	tx *sql.Tx,
	current, candidate *models.Task,
) error {
	if !taskTransitionsToCompleted(current, candidate) || candidate.WorkflowStepID == "" {
		return nil
	}
	return r.requireWorkflowStepTx(ctx, tx, candidate)
}

func validateCompletionAdmission(admission *completion.Admission, candidate *models.Task) error {
	if candidate == nil || admission.TaskID == "" || admission.TaskID != candidate.ID ||
		admission.Actor != workflowmodels.StepTransitionActorHuman {
		return repoerrors.ErrWorkflowChangeConflict
	}
	switch admission.Kind {
	case completion.Move, completion.SetState, completion.FinalStepDone, completion.Fields:
		return nil
	default:
		return repoerrors.ErrWorkflowChangeConflict
	}
}

func validateCompletionAdmissionScope(current, candidate *models.Task, admission *completion.Admission) error {
	if admission.Kind != completion.Fields &&
		(current.WorkflowID != admission.Expected.WorkflowID || current.WorkflowStepID != admission.Expected.StepID) {
		return repoerrors.ErrWorkflowChangeConflict
	}
	if admission.Kind != completion.Fields && admission.Expected.TaskState != "" &&
		string(current.State) != admission.Expected.TaskState {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}

func (r *Repository) requireWorkflowStepTx(ctx context.Context, tx *sql.Tx, candidate *models.Task) error {
	var exists bool
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT EXISTS(SELECT 1 FROM workflow_steps WHERE workflow_id = ? AND id = ?)
	`), candidate.WorkflowID, candidate.WorkflowStepID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return repoerrors.ErrWorkflowChangeConflict
	}
	return nil
}

func (r *Repository) workflowStepPredicateTx(
	ctx context.Context,
	tx *sql.Tx,
	predicate, workflowID, stepID string,
) (bool, error) {
	if stepID == "" {
		return false, sql.ErrNoRows
	}
	query := `SELECT ` + predicate + ` FROM workflow_steps s WHERE s.workflow_id = ? AND s.id = ?`
	var matches bool
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), workflowID, stepID).Scan(&matches); err != nil {
		return false, err
	}
	return matches, nil
}

func (r *Repository) isCompletingWorkflowStepTx(ctx context.Context, tx *sql.Tx, workflowID, stepID string) (bool, error) {
	completing, err := r.workflowStepPredicateTx(
		ctx, tx, workflowmodels.CompletingStepSQL("s"), workflowID, stepID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return completing, err
}

func (r *Repository) refreshHumanAdmissionLimitTx(
	ctx context.Context,
	tx *sql.Tx,
	task *models.Task,
	limit int,
	admission *completion.Admission,
) (int, error) {
	if admission == nil || task.WorkflowStepID == "" {
		return limit, nil
	}
	var current int
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT wip_limit FROM workflow_steps WHERE id = ? AND workflow_id = ?
	`), task.WorkflowStepID, task.WorkflowID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("workflow step %q is unavailable for completion admission", task.WorkflowStepID)
	}
	return current, err
}

// CompleteTaskOnFinalStep records a final-step completion with typed
// admission. Registered policies validate the write in the same transaction.
func (r *Repository) CompleteTaskOnFinalStep(
	ctx context.Context,
	id string,
	admission *completion.Admission,
) (*models.Task, v1.TaskState, bool, error) {
	if admission == nil || admission.Kind != completion.FinalStepDone {
		return nil, "", false, repoerrors.ErrWorkflowChangeConflict
	}
	return r.updateHumanTaskState(ctx, id, v1.TaskStateCompleted, admission)
}

func (r *Repository) updateHumanTaskState(
	ctx context.Context,
	id string,
	state v1.TaskState,
	admission *completion.Admission,
) (*models.Task, v1.TaskState, bool, error) {
	tx, err := r.db.BeginTx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, "", false, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := r.lockTaskFieldUpdate(ctx, tx, id, models.TaskFieldUpdate{})
	if err != nil {
		return nil, "", false, err
	}
	previousState := task.State
	task.State = state
	if err := r.admitCompletionTx(ctx, tx, admission, task); err != nil {
		return nil, "", false, err
	}
	if err := r.guardTaskCompletionStateTx(ctx, tx, id, previousState, task.State, task.WorkflowID, task.WorkflowStepID); err != nil {
		return nil, "", false, err
	}
	task.UpdatedAt = r.nowUTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`), task.State, task.UpdatedAt, id); err != nil {
		return nil, "", false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", false, err
	}
	return task, previousState, previousState != state, nil
}
