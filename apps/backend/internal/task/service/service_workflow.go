package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ApproveSessionResult contains the result of approving a session
type ApproveSessionResult struct {
	Session      *models.TaskSession
	Task         *models.Task
	WorkflowStep *wfmodels.WorkflowStep
}

// ApproveSession approves a session's current step and moves it to the next step.
// It reads the step's on_turn_complete actions to determine where to transition.
// If no transition actions are configured, it falls back to the next step by position.
func (s *Service) ApproveSession(ctx context.Context, sessionID string) (*ApproveSessionResult, error) {
	result := &ApproveSessionResult{}

	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	result.Session = session

	// Get the task to find its current workflow step
	task, err := s.tasks.GetTask(ctx, session.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Get the current workflow step to check for transition targets
	if task.WorkflowStepID != "" && s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for approval transition",
				zap.String("workflow_step_id", task.WorkflowStepID),
				zap.Error(err))
		} else if err := s.applyApprovalStepTransition(ctx, sessionID, step, result); err != nil {
			return nil, err
		}
	}

	if err := s.sessions.UpdateSessionReviewStatus(ctx, sessionID, "approved"); err != nil {
		return nil, fmt.Errorf("failed to update review status: %w", err)
	}
	if session, err := s.sessions.GetTaskSession(ctx, sessionID); err == nil {
		result.Session = session
	}

	return result, nil
}

// applyApprovalStepTransition resolves the next workflow step and updates session/task accordingly.
func (s *Service) applyApprovalStepTransition(ctx context.Context, sessionID string, step *wfmodels.WorkflowStep, result *ApproveSessionResult) error {
	newStepID := s.resolveApprovalNextStep(ctx, step)

	if newStepID == "" {
		s.logger.Info("session approved but no next step found (may be at final step)",
			zap.String("session_id", sessionID),
			zap.String("current_step", step.ID),
			zap.String("current_step_name", step.Name))
		return nil
	}

	moved, err := s.MoveTask(ctx, result.Session.TaskID, step.WorkflowID, newStepID, 0)
	if err != nil {
		return fmt.Errorf("failed to move task to next step after approval: %w", err)
	}
	result.Task = moved.Task
	result.WorkflowStep = moved.WorkflowStep

	// Reload session with new step
	result.Session, _ = s.sessions.GetTaskSession(ctx, sessionID)

	// Get the new workflow step for the response
	if result.WorkflowStep == nil {
		newStep, err := s.workflowStepGetter.GetStep(ctx, newStepID)
		if err == nil {
			result.WorkflowStep = newStep
		}
	}

	s.logger.Info("session approved and moved to next step",
		zap.String("session_id", sessionID),
		zap.String("from_step", step.ID),
		zap.String("to_step", newStepID))
	return nil
}

// resolveApprovalNextStep determines the target step ID from a step's on_turn_complete actions,
// falling back to the next step by position when no actions are configured.
func (s *Service) resolveApprovalNextStep(ctx context.Context, step *wfmodels.WorkflowStep) string {
	var newStepID string
	for _, action := range step.Events.OnTurnComplete {
		switch action.Type {
		case "move_to_next":
			nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
			if err != nil {
				s.logger.Warn("failed to get next step by position",
					zap.String("workflow_id", step.WorkflowID),
					zap.Int("current_position", step.Position),
					zap.Error(err))
			} else if nextStep != nil {
				newStepID = nextStep.ID
			}
		case "move_to_step":
			if stepID, ok := action.Config["step_id"].(string); ok && stepID != "" {
				newStepID = stepID
			}
		}
		if newStepID != "" {
			return newStepID
		}
	}

	// Fall back to next step by position if no transition actions found
	if len(step.Events.OnTurnComplete) == 0 {
		nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
		if err != nil {
			s.logger.Warn("failed to get next step by position for fallback",
				zap.String("workflow_id", step.WorkflowID),
				zap.Int("current_position", step.Position),
				zap.Error(err))
		} else if nextStep != nil {
			s.logger.Info("using next step by position for approval transition (fallback)",
				zap.String("current_step", step.Name),
				zap.String("next_step", nextStep.Name))
			newStepID = nextStep.ID
		}
	}

	return newStepID
}

// UpdateTaskState updates the state of a task, moves it to the matching column,
// and publishes a task.state_changed event
func (s *Service) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	oldState := task.State

	// Skip no-op state transitions to avoid duplicate events.
	if oldState == state {
		return task, nil
	}

	if err := s.tasks.UpdateTaskState(ctx, id, state); err != nil {
		s.logger.Error("failed to update task state", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Reload task to get updated state
	task, err = s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	s.logger.Info("task state updated",
		zap.String("task_id", id),
		zap.String("workflow_step_id", task.WorkflowStepID),
		zap.String("state", string(task.State)))

	s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	s.logger.Info("task state changed",
		zap.String("task_id", id),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(state)))

	return task, nil
}

// UpdateTaskStateIfCurrentIn transitions state only when the task is currently
// in one of the allowed values. Publishes task.state_changed only when a row
// changes.
func (s *Service) UpdateTaskStateIfCurrentIn(
	ctx context.Context, id string, state v1.TaskState, allowed []v1.TaskState,
) (bool, error) {
	oldState, updated, err := s.tasks.UpdateTaskStateIfCurrentIn(ctx, id, state, allowed)
	if err != nil || !updated {
		return false, err
	}
	// Unreachable for current callers (allowed never includes state) — kept so a
	// future caller that includes state in allowed still skips a duplicate publish.
	if oldState == state {
		return false, nil
	}

	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return true, err
	}
	// The CAS wrote `state`; pin it on the payload so a concurrent transition
	// between commit and read cannot publish a mismatched new_state.
	task.State = state

	s.logger.Info("task state updated",
		zap.String("task_id", id),
		zap.String("workflow_step_id", task.WorkflowStepID),
		zap.String("state", string(state)))

	s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	s.logger.Info("task state changed",
		zap.String("task_id", id),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(state)))

	return true, nil
}

// UpdateTaskMetadata updates only the metadata of a task (merges with existing)
func (s *Service) UpdateTaskMetadata(ctx context.Context, id string, metadata map[string]interface{}) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Merge metadata (existing keys are preserved, new keys are added/updated)
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	for k, v := range metadata {
		task.Metadata[k] = v
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to update task metadata", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	s.logger.Debug("task metadata updated", zap.String("task_id", id), zap.Any("metadata", metadata))
	return task, nil
}

// MoveTaskResult contains the result of a MoveTask operation.
type MoveTaskResult struct {
	Task         *models.Task
	WorkflowStep *wfmodels.WorkflowStep
}

// MoveTaskOptions controls non-default move behavior for trusted callers.
type MoveTaskOptions struct {
	AllowActivePrimarySession bool
}

type workflowMoveLimitsRepository interface {
	CountTasksByWorkflowStepExcludingTask(ctx context.Context, stepID, excludeTaskID string) (int, error)
}

type workflowLimitedMoveRepository interface {
	UpdateTaskIfWorkflowStepHasCapacity(ctx context.Context, task *models.Task, targetStepID, excludeTaskID string, limit int) error
}

type workflowPullRepository interface {
	NextPullCandidateExcluding(ctx context.Context, stepID string, excludeTaskIDs []string) (*models.Task, error)
}

// MoveTask moves a task to a different workflow step and position
func (s *Service) MoveTask(ctx context.Context, id string, workflowID string, workflowStepID string, position int) (*MoveTaskResult, error) {
	return s.MoveTaskWithOptions(ctx, id, workflowID, workflowStepID, position, MoveTaskOptions{})
}

// MoveTaskWithOptions moves a task with explicit caller options.
func (s *Service) MoveTaskWithOptions(
	ctx context.Context,
	id string,
	workflowID string,
	workflowStepID string,
	position int,
	opts MoveTaskOptions,
) (*MoveTaskResult, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	targetStep, err := s.validateTaskMove(ctx, task, workflowID, workflowStepID, opts)
	if err != nil {
		return nil, err
	}

	oldWorkflowID := task.WorkflowID
	oldStepID := task.WorkflowStepID
	oldState := task.State
	stepChanged := oldStepID != workflowStepID

	task.WorkflowID = workflowID
	task.WorkflowStepID = workflowStepID
	task.Position = position
	if err := s.syncTaskStateForWorkflowMove(ctx, task, oldStepID, workflowStepID); err != nil {
		return nil, fmt.Errorf("failed to sync task state for workflow move: %w", err)
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.updateMovedTask(ctx, task, oldStepID, targetStep); err != nil {
		s.logger.Error("failed to move task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	// Resolve active session for the task.moved event (needed for on_exit/on_enter).
	sessionID := ""
	if activeSession := s.resolvePrimaryOrActiveSession(ctx, id); activeSession != nil {
		sessionID = activeSession.ID
	}

	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil, oldWorkflowID)
	if oldState != task.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	}

	// Publish task.moved event so the orchestrator can process on_exit/on_enter actions
	if stepChanged {
		s.publishTaskMovedEvent(ctx, task, oldWorkflowID, oldStepID, workflowStepID, sessionID)
		s.pullNextTaskOnVacate(ctx, oldStepID, task.ID)
	}

	s.logger.Info("task moved",
		zap.String("task_id", id),
		zap.String("workflow_id", workflowID),
		zap.String("workflow_step_id", workflowStepID),
		zap.Int("position", position))

	result := &MoveTaskResult{Task: task}

	// Fetch the workflow step info if getter is available
	if s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for MoveTask response",
				zap.String("workflow_step_id", workflowStepID),
				zap.Error(err))
			// Don't fail the operation, just log and continue
		} else {
			result.WorkflowStep = step
		}
	}

	return result, nil
}

func (s *Service) terminalWorkflowStep(ctx context.Context, workflowStepID string) (bool, error) {
	if s.workflowStepGetter == nil || workflowStepID == "" {
		return false, nil
	}
	step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil {
		return false, fmt.Errorf("failed to get workflow step %s: %w", workflowStepID, err)
	}
	if step == nil {
		return false, nil
	}
	nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
	if err != nil {
		return false, fmt.Errorf("failed to get next workflow step after %s: %w", workflowStepID, err)
	}
	return wfmodels.IsTerminalStep(step, nextStep), nil
}

func (s *Service) syncTaskStateForWorkflowMove(ctx context.Context, task *models.Task, oldStepID, newStepID string) error {
	newTerminal, err := s.terminalWorkflowStep(ctx, newStepID)
	if err != nil {
		return err
	}
	if newTerminal {
		if !models.IsTerminalTaskState(task.State) {
			task.State = v1.TaskStateCompleted
		}
		return nil
	}
	if oldStepID == newStepID || task.State != v1.TaskStateCompleted {
		return nil
	}
	oldTerminal, err := s.terminalWorkflowStep(ctx, oldStepID)
	if err != nil {
		return err
	}
	if oldTerminal {
		task.State = v1.TaskStateTODO
	}
	return nil
}

func (s *Service) syncTaskStateForBulkWorkflowMove(ctx context.Context, task *models.Task, oldStepID, targetStepID string, targetIsTerminal, sourceIsTerminal, sourceTerminalKnown bool) error {
	if targetIsTerminal {
		if !models.IsTerminalTaskState(task.State) {
			task.State = v1.TaskStateCompleted
		}
		return nil
	}
	if oldStepID == targetStepID || task.State != v1.TaskStateCompleted {
		return nil
	}

	oldTerminal := sourceIsTerminal
	if !sourceTerminalKnown {
		var err error
		oldTerminal, err = s.terminalWorkflowStep(ctx, oldStepID)
		if err != nil {
			return err
		}
	}
	if oldTerminal {
		task.State = v1.TaskStateTODO
	}
	return nil
}

func (s *Service) pullNextTaskOnVacate(ctx context.Context, vacatedStepID, excludeTaskID string) {
	vacatedStep := s.pullEnabledStep(ctx, vacatedStepID)
	if vacatedStep == nil {
		return
	}
	limitsRepo, pullRepo, ok := s.pullRepositories(vacatedStep.ID)
	if !ok {
		return
	}
	occupants, ok := s.currentWIPOccupants(ctx, limitsRepo, vacatedStep.ID)
	if !ok || occupants >= vacatedStep.WIPLimit {
		return
	}
	skipped := map[string]struct{}{excludeTaskID: {}}
	for occupants < vacatedStep.WIPLimit {
		pulled := s.pullOneFeederTask(ctx, pullRepo, vacatedStep, occupants, skipped)
		if !pulled {
			return
		}
		occupants++
	}
}

func (s *Service) pullEnabledStep(ctx context.Context, vacatedStepID string) *wfmodels.WorkflowStep {
	if s.workflowStepGetter == nil || vacatedStepID == "" {
		return nil
	}
	vacatedStep, err := s.workflowStepGetter.GetStep(ctx, vacatedStepID)
	if err != nil || vacatedStep == nil || vacatedStep.WIPLimit <= 0 || vacatedStep.PullFromStepID == "" {
		return nil
	}
	if vacatedStep.PullFromStepID == vacatedStep.ID {
		return nil
	}
	return vacatedStep
}

func (s *Service) pullRepositories(stepID string) (workflowMoveLimitsRepository, workflowPullRepository, bool) {
	limitsRepo, ok := s.tasks.(workflowMoveLimitsRepository)
	if !ok {
		s.logger.Warn("cannot pull feeder task: WIP limit repository unavailable",
			zap.String("step_id", stepID))
		return nil, nil, false
	}
	pullRepo, ok := s.tasks.(workflowPullRepository)
	if !ok {
		s.logger.Warn("cannot pull feeder task: pull repository unavailable",
			zap.String("step_id", stepID))
		return nil, nil, false
	}
	return limitsRepo, pullRepo, true
}

func (s *Service) currentWIPOccupants(ctx context.Context, limitsRepo workflowMoveLimitsRepository, stepID string) (int, bool) {
	occupants, err := limitsRepo.CountTasksByWorkflowStepExcludingTask(ctx, stepID, "")
	if err != nil {
		s.logger.Warn("cannot pull feeder task: failed to count vacated step",
			zap.String("step_id", stepID), zap.Error(err))
		return 0, false
	}
	return occupants, true
}

func (s *Service) pullOneFeederTask(
	ctx context.Context,
	pullRepo workflowPullRepository,
	vacatedStep *wfmodels.WorkflowStep,
	position int,
	skipped map[string]struct{},
) bool {
	for {
		candidate, err := pullRepo.NextPullCandidateExcluding(ctx, vacatedStep.PullFromStepID, skippedTaskIDs(skipped))
		if err != nil {
			s.logger.Warn("cannot pull feeder task: failed to select candidate",
				zap.String("step_id", vacatedStep.ID), zap.Error(err))
			return false
		}
		if candidate == nil {
			return false
		}
		if _, seen := skipped[candidate.ID]; seen {
			continue
		}
		if _, err := s.MoveTask(ctx, candidate.ID, vacatedStep.WorkflowID, vacatedStep.ID, position); err != nil {
			skipped[candidate.ID] = struct{}{}
			s.logger.Warn("skipping feeder task that could not be pulled",
				zap.String("task_id", candidate.ID),
				zap.String("from_step_id", vacatedStep.PullFromStepID),
				zap.String("to_step_id", vacatedStep.ID),
				zap.Error(err))
			continue
		}
		return true
	}
}

func skippedTaskIDs(skipped map[string]struct{}) []string {
	ids := make([]string, 0, len(skipped))
	for id := range skipped {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Service) updateMovedTask(ctx context.Context, task *models.Task, oldStepID string, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil || targetStep.WIPLimit <= 0 || oldStepID == targetStep.ID {
		return s.tasks.UpdateTask(ctx, task)
	}
	limitedRepo, ok := s.tasks.(workflowLimitedMoveRepository)
	if !ok {
		return fmt.Errorf("WIP limit cannot be checked for workflow step %s", targetStep.ID)
	}
	return limitedRepo.UpdateTaskIfWorkflowStepHasCapacity(ctx, task, targetStep.ID, task.ID, targetStep.WIPLimit)
}

func (s *Service) validateTaskMove(ctx context.Context, task *models.Task, workflowID, workflowStepID string, opts MoveTaskOptions) (*wfmodels.WorkflowStep, error) {
	if task.ArchivedAt != nil {
		return nil, fmt.Errorf("archived tasks cannot be moved")
	}
	if err := s.validateMoveSessions(ctx, task.ID, opts); err != nil {
		return nil, err
	}
	targetWorkflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target workflow: %w", err)
	}
	if targetWorkflow.WorkspaceID != task.WorkspaceID {
		return nil, fmt.Errorf("target workflow is in a different workspace")
	}
	if s.workflowStepGetter == nil {
		return nil, nil
	}
	targetStep, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target workflow step: %w", err)
	}
	if targetStep.WorkflowID != workflowID {
		return nil, fmt.Errorf("target workflow step does not belong to target workflow")
	}
	if err := s.validateMoveWIPLimit(ctx, task, targetStep); err != nil {
		return nil, err
	}
	return targetStep, nil
}

func (s *Service) validateMoveWIPLimit(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil || targetStep.WIPLimit <= 0 || task.WorkflowStepID == targetStep.ID {
		return nil
	}
	limitsRepo, ok := s.tasks.(workflowMoveLimitsRepository)
	if !ok {
		return fmt.Errorf("WIP limit cannot be checked for workflow step %s", targetStep.ID)
	}
	occupants, err := limitsRepo.CountTasksByWorkflowStepExcludingTask(ctx, targetStep.ID, task.ID)
	if err != nil {
		return fmt.Errorf("failed to count target workflow step tasks: %w", err)
	}
	if occupants >= targetStep.WIPLimit {
		return fmt.Errorf("WIP limit exceeded for workflow step %s: limit %d already occupied", targetStep.ID, targetStep.WIPLimit)
	}
	return nil
}

func (s *Service) validateMoveSessions(ctx context.Context, taskID string, opts MoveTaskOptions) error {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to list task sessions: %w", err)
	}
	for _, session := range sessions {
		if isSessionMoveBlocked(session.State) {
			if opts.AllowActivePrimarySession && session.IsPrimary {
				continue
			}
			return fmt.Errorf("task has an active session (%s)", session.State)
		}
	}
	return nil
}

func isSessionMoveBlocked(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateStarting ||
		state == models.TaskSessionStateRunning
}

// resolvePrimaryOrActiveSession returns the primary session if it is in an active
// state, otherwise falls back to the most recently started active session.
func (s *Service) resolvePrimaryOrActiveSession(ctx context.Context, taskID string) *models.TaskSession {
	primary, _ := s.sessions.GetPrimarySessionByTaskID(ctx, taskID)
	if primary != nil && isSessionActive(primary.State) {
		return primary
	}
	active, err := s.sessions.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil || active == nil {
		return nil
	}
	return active
}

func isSessionActive(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateCreated ||
		state == models.TaskSessionStateStarting ||
		state == models.TaskSessionStateRunning ||
		state == models.TaskSessionStateWaitingForInput
}

// CountTasksByWorkflow returns the number of tasks in a workflow
func (s *Service) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	return s.tasks.CountTasksByWorkflow(ctx, workflowID)
}

// CountTasksByWorkflowStep returns the number of tasks in a workflow step
func (s *Service) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	return s.tasks.CountTasksByWorkflowStep(ctx, stepID)
}

// BulkMoveTasksResult contains the result of a BulkMoveTasks operation.
type BulkMoveTasksResult struct {
	MovedCount int
}

// BulkMoveSelectedTasks moves an explicit task list to a target workflow step.
// The list order is treated as the visible UI order; tasks already in the
// target step are skipped. Validation reads tasks one at a time because the UI
// sends small selected batches; the move is not transactional if task state
// changes between pre-validation and an individual MoveTask call.
func (s *Service) BulkMoveSelectedTasks(ctx context.Context, taskIDs []string, targetWorkflowID, targetStepID string) (*BulkMoveTasksResult, error) {
	ids := uniqueTaskIDs(taskIDs)
	if len(ids) == 0 {
		return &BulkMoveTasksResult{MovedCount: 0}, nil
	}

	tasks, err := s.validateSelectedMoveBatch(ctx, ids, targetWorkflowID, targetStepID)
	if err != nil {
		return nil, err
	}
	if err := s.validateBulkMoveWIPCapacity(ctx, tasks, targetStepID); err != nil {
		return nil, err
	}

	nextPosition, err := s.tasks.CountTasksByWorkflowStep(ctx, targetStepID)
	if err != nil {
		return nil, fmt.Errorf("failed to count target workflow step tasks: %w", err)
	}

	movedCount := 0
	for _, task := range tasks {
		if task.WorkflowID == targetWorkflowID && task.WorkflowStepID == targetStepID {
			continue
		}
		if _, err := s.MoveTask(ctx, task.ID, targetWorkflowID, targetStepID, nextPosition+movedCount); err != nil {
			return nil, fmt.Errorf("failed to move task %s: %w", task.ID, err)
		}
		movedCount++
	}

	return &BulkMoveTasksResult{MovedCount: movedCount}, nil
}

func uniqueTaskIDs(taskIDs []string) []string {
	seen := make(map[string]struct{}, len(taskIDs))
	result := make([]string, 0, len(taskIDs))
	for _, id := range taskIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (s *Service) validateSelectedMoveBatch(ctx context.Context, taskIDs []string, targetWorkflowID, targetStepID string) ([]*models.Task, error) {
	tasks := make([]*models.Task, 0, len(taskIDs))
	for _, id := range taskIDs {
		task, err := s.tasks.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		if task.WorkflowID != targetWorkflowID || task.WorkflowStepID != targetStepID {
			if _, err := s.validateTaskMove(ctx, task, targetWorkflowID, targetStepID, MoveTaskOptions{}); err != nil {
				return nil, fmt.Errorf("task %s cannot be moved: %w", id, err)
			}
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (s *Service) validateBulkMoveWIPCapacity(ctx context.Context, tasks []*models.Task, targetStepID string) error {
	if s.workflowStepGetter == nil {
		return nil
	}
	targetStep, err := s.workflowStepGetter.GetStep(ctx, targetStepID)
	if err != nil {
		return fmt.Errorf("failed to get target workflow step: %w", err)
	}
	if targetStep.WIPLimit <= 0 {
		return nil
	}
	incoming := 0
	for _, task := range tasks {
		if task.WorkflowStepID != targetStepID {
			incoming++
		}
	}
	if incoming == 0 {
		return nil
	}
	limitsRepo, ok := s.tasks.(workflowMoveLimitsRepository)
	if !ok {
		return fmt.Errorf("WIP limit cannot be checked for workflow step %s", targetStep.ID)
	}
	occupants, err := limitsRepo.CountTasksByWorkflowStepExcludingTask(ctx, targetStep.ID, "")
	if err != nil {
		return fmt.Errorf("failed to count target workflow step tasks: %w", err)
	}
	if occupants+incoming > targetStep.WIPLimit {
		return fmt.Errorf("WIP limit exceeded for workflow step %s: limit %d, occupied %d, moving %d",
			targetStep.ID, targetStep.WIPLimit, occupants, incoming)
	}
	return nil
}

// BulkMoveTasks moves all tasks from a source workflow/step to a target workflow/step.
// If sourceStepID is empty, all tasks in the source workflow are moved.
func (s *Service) BulkMoveTasks(ctx context.Context, sourceWorkflowID, sourceStepID, targetWorkflowID, targetStepID string) (*BulkMoveTasksResult, error) {
	// Get the tasks to move
	var tasks []*models.Task
	var err error
	if sourceStepID != "" {
		tasks, err = s.tasks.ListTasksByWorkflowStep(ctx, sourceStepID)
	} else {
		tasks, err = s.tasks.ListTasks(ctx, sourceWorkflowID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks for bulk move: %w", err)
	}

	if len(tasks) == 0 {
		return &BulkMoveTasksResult{MovedCount: 0}, nil
	}
	if err := s.validateBulkMoveWIPCapacity(ctx, tasks, targetStepID); err != nil {
		return nil, err
	}

	for i, task := range tasks {
		if _, err := s.MoveTask(ctx, task.ID, targetWorkflowID, targetStepID, i); err != nil {
			return nil, fmt.Errorf("failed to move task %s: %w", task.ID, err)
		}
	}

	s.logger.Info("bulk moved tasks",
		zap.String("source_workflow_id", sourceWorkflowID),
		zap.String("source_step_id", sourceStepID),
		zap.String("target_workflow_id", targetWorkflowID),
		zap.String("target_step_id", targetStepID),
		zap.Int("moved_count", len(tasks)))

	return &BulkMoveTasksResult{MovedCount: len(tasks)}, nil
}
