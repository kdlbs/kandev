package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ApproveSessionResult contains the result of approving a session
type ApproveSessionResult struct {
	Session      *models.TaskSession
	Task         *models.Task
	WorkflowStep *wfmodels.WorkflowStep
}

type primarySessionTaskStateRepository interface {
	UpdateTaskStateIfPrimarySessionState(
		context.Context,
		string,
		string,
		models.TaskSessionState,
		v1.TaskState,
	) (v1.TaskState, bool, error)
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
	// Approving advances the task's workflow step, so this must be owner-only.
	if err := s.authorizeTaskID(ctx, session.TaskID); err != nil {
		return nil, err
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

	moved, err := s.MoveTaskWithOptions(ctx, result.Session.TaskID, step.WorkflowID, newStepID, 0,
		MoveTaskOptions{
			StepHistoryTrigger:   wfmodels.StepTransitionTriggerApproval,
			StepHistorySessionID: sessionID,
			StepHistoryActor:     wfmodels.StepTransitionActorHuman,
		})
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
	if err := s.authorizeTaskID(ctx, id); err != nil {
		return nil, err
	}
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

// UpdateTaskStateIfNotArchived is UpdateTaskStateIfCurrentIn without the
// prior-state constraint — for writers (IN_PROGRESS runtime reconciliation)
// that legitimately fire from many prior states and only need the
// archived-task freeze guarantee. Publishes task.state_changed only when a
// row changes.
func (s *Service) UpdateTaskStateIfNotArchived(
	ctx context.Context, id string, state v1.TaskState,
) (bool, error) {
	oldState, updated, err := s.tasks.UpdateTaskStateIfNotArchived(ctx, id, state)
	if err != nil || !updated {
		return false, err
	}
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

// UpdateTaskStateIfSessionState transitions task state only while its owning
// session remains in the expected state and the task remains unarchived.
// Publishes task.state_changed only when the guarded write changes state.
func (s *Service) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	return s.updateTaskStateIfSessionState(
		ctx, taskID, sessionID, expectedSessionState, state, false,
	)
}

// UpdateTaskStateIfPrimarySessionState also requires the named session to
// remain primary.
func (s *Service) UpdateTaskStateIfPrimarySessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	return s.updateTaskStateIfSessionState(
		ctx, taskID, sessionID, expectedSessionState, state, true,
	)
}

func (s *Service) updateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
	requirePrimary bool,
) (bool, error) {
	var (
		oldState v1.TaskState
		updated  bool
		err      error
	)
	if requirePrimary {
		updater, ok := s.tasks.(primarySessionTaskStateRepository)
		if !ok {
			return false, errors.New("primary-session task state update is not supported")
		}
		oldState, updated, err = updater.UpdateTaskStateIfPrimarySessionState(
			ctx, taskID, sessionID, expectedSessionState, state,
		)
	} else {
		oldState, updated, err = s.tasks.UpdateTaskStateIfSessionState(
			ctx, taskID, sessionID, expectedSessionState, state,
		)
	}
	if err != nil || !updated {
		return false, err
	}
	if oldState == state {
		return false, nil
	}

	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return true, err
	}
	// Pin the state written by the guarded CAS so a later transition between
	// commit and reload cannot produce a mismatched event payload.
	task.State = state
	s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	return true, nil
}

// UpdateTaskMetadata updates only the metadata of a task (merges with existing)
func (s *Service) UpdateTaskMetadata(ctx context.Context, id string, metadata map[string]interface{}) (*models.Task, error) {
	if err := s.authorizeTaskID(ctx, id); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}

	// Merge metadata (existing keys are preserved, new keys are added/updated)
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	for k, v := range metadata {
		// Deferred launch ownership is server-managed. Preserve it even if a
		// future metadata endpoint forwards the whole request map here.
		if k == models.MetaKeyDeferredLaunch {
			continue
		}
		task.Metadata[k] = v
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		s.logger.Error("failed to update task metadata", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	s.PublishTaskUpdated(ctx, task)
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
	// AllowFailedToCompletedRecovery permits the trusted launch-recovery
	// action to complete a failed task when it moves into a validated terminal
	// workflow step. Ordinary task moves preserve failed and cancelled states.
	AllowFailedToCompletedRecovery bool
	// PreserveDeferredLaunch keeps the deferred launch intent when an internal
	// queue promotion changes workflow steps. Manual moves still clear it.
	PreserveDeferredLaunch bool
	// StepHistoryTrigger overrides the ADR 0015 audit-row trigger recorded for
	// this move. Zero value defaults to StepTransitionTriggerManual — callers
	// driving an approval-gated transition (ApproveSession) set
	// StepTransitionTriggerApproval instead.
	StepHistoryTrigger wfmodels.StepTransitionTrigger
	// StepHistorySessionID pins the ADR 0015 audit-row session_id to a
	// specific session, overriding the primary/active-session resolution
	// MoveTaskWithOptions otherwise uses. ApproveSession sets this to the
	// session it is actually approving — on a task with more than one
	// active session, resolvePrimaryOrActiveSession can pick a different
	// (primary) session than the one being approved.
	StepHistorySessionID string
	// StepHistoryActor identifies the caller. Agent moves must not inherit the
	// owner identity that MCP uses for authorization.
	StepHistoryActor wfmodels.StepTransitionActor
}

type workflowMoveLimitsRepository interface {
	CountTasksByWorkflowStepExcludingTask(ctx context.Context, stepID, excludeTaskID string) (int, error)
}

type workflowAdmittedCountRepository interface {
	CountAdmittedTasksByWorkflowStep(ctx context.Context, stepID string) (int, error)
}

type workflowLimitedMoveRepository interface {
	UpdateTaskIfWorkflowStepHasCapacity(ctx context.Context, task *models.Task, targetStepID, excludeTaskID string, limit int) error
}

type workflowMoveAdmissionRepository interface {
	UpdateTaskWithWorkflowStepAdmission(ctx context.Context, task *models.Task, targetStepID string, limit int) (bool, error)
}

type workflowMoveAdmissionWithStateRepository interface {
	UpdateTaskWithWorkflowStepAdmissionAndState(
		ctx context.Context,
		task *models.Task,
		targetStepID string,
		limit int,
		admittedState *v1.TaskState,
		queueExitPending bool,
	) (bool, error)
}

type workflowQueuedTaskPromoter interface {
	PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx context.Context, task *models.Task, fromStepID, destinationStepID string, limit int) (bool, error)
}

type workflowPullRepository interface {
	NextPullCandidateExcluding(ctx context.Context, stepID string, excludeTaskIDs []string) (*models.Task, error)
}

type workflowQueuedPullRepository interface {
	NextQueuedTaskForStepExcluding(ctx context.Context, feederStepID, destinationStepID string, excludeTaskIDs []string) (*models.Task, error)
}

const (
	priorityMedium = "medium"
	priorityLow    = "low"
)

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
	if err := s.authorizeTaskID(ctx, id); err != nil {
		return nil, err
	}
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
	stateAfterAdmission := *task
	if stepChanged {
		if err := s.syncTaskStateForWorkflowMove(ctx, &stateAfterAdmission, oldStepID, workflowStepID, opts); err != nil {
			return nil, fmt.Errorf("failed to sync task state for workflow move: %w", err)
		}
	}

	task.WorkflowID = workflowID
	task.WorkflowStepID = workflowStepID
	task.Position = position
	if stepChanged {
		if task.Metadata == nil {
			task.Metadata = make(map[string]interface{})
		}
		task.WIPAdmitted = true
		task.QueuedForStepID = ""
		task.QueuedAt = nil
		task.Metadata[models.MetaKeyQueuedMoveExitPending] = map[string]interface{}{
			"from_step_id": oldStepID,
		}
		delete(task.Metadata, models.MetaKeyQueuedMoveExitCompleted)
		delete(task.Metadata, models.MetaKeyQueuePromotionPending)
		delete(task.Metadata, models.MetaKeyManualMoveLifecycleCompleted)
		if !opts.PreserveDeferredLaunch {
			models.DropWIPDeferredLaunch(task)
		}
	}
	task.UpdatedAt = time.Now().UTC()

	// Keep an admitted manual move's lifecycle barrier in the same task write as
	// the move whenever an active session exists. The admission repository removes
	// the queued-exit marker for admitted tasks, so this separate marker carries
	// the barrier across the task.moved event without changing WIP admission.
	sessionID := ""
	if stepChanged {
		if activeSession := s.resolvePrimaryOrActiveSession(ctx, id); activeSession != nil {
			sessionID = activeSession.ID
			task.Metadata[models.MetaKeyManualMoveLifecyclePending] = map[string]interface{}{
				"from_step_id": oldStepID,
			}
		}
	}

	var admittedState *v1.TaskState
	if stepChanged {
		admittedState = &stateAfterAdmission.State
	}
	if !stepChanged && opts.AllowFailedToCompletedRecovery && task.State == v1.TaskStateFailed {
		terminal, err := s.terminalWorkflowStep(ctx, workflowStepID)
		if err != nil {
			return nil, fmt.Errorf("failed to validate recovery target: %w", err)
		}
		if terminal {
			task.State = v1.TaskStateCompleted
		}
	}

	// manual_move only applies when no outer caller already declared a
	// trigger — an mcp_move set by the MCP handler must survive this inner
	// board-move default, since the agent (not a board click) is what caused
	// the move.
	moveCtx := ctx
	if !steptelemetry.HasTrigger(moveCtx) {
		actorKind, actorID := steptelemetry.HumanOrSystemActor(moveCtx)
		moveCtx = steptelemetry.WithAttribution(moveCtx, steptelemetry.Attribution{
			Trigger:   steptelemetry.TriggerManualMove,
			ActorKind: actorKind,
			ActorID:   actorID,
		})
	}

	_, err = s.updateMovedTask(moveCtx, task, oldStepID, targetStep, admittedState)
	if err != nil {
		s.logger.Error("failed to move task", zap.String("task_id", id), zap.Error(err))
		return nil, err
	}

	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil, oldWorkflowID)
	if oldState != task.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	}

	// Publish task.moved event so the orchestrator can process on_exit/on_enter actions
	if stepChanged {
		s.publishTaskMovedEvent(ctx, task, oldWorkflowID, oldStepID, workflowStepID, sessionID)
		historySessionID := opts.StepHistorySessionID
		if historySessionID == "" {
			historySessionID = sessionID
		}
		s.recordManualStepTransition(ctx, historySessionID, oldStepID, workflowStepID, opts.StepHistoryTrigger, opts.StepHistoryActor)
		s.pullNextTaskOnVacate(ctx, oldStepID, task.ID)
		s.pullTasksFromNewFeederWork(ctx, workflowID, workflowStepID)
		refreshed, err := s.tasks.GetTask(ctx, task.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh task after feeder pull: %w", err)
		}
		if refreshed == nil {
			return nil, errors.New("failed to refresh task after feeder pull: repository returned nil task")
		}
		refreshed.Repositories = task.Repositories
		task = refreshed
	}

	s.logger.Info("task moved",
		zap.String("task_id", id),
		zap.String("workflow_id", workflowID),
		zap.String("workflow_step_id", workflowStepID),
		zap.Int("position", position))

	result := &MoveTaskResult{Task: task}

	// Fetch the workflow step info if getter is available
	if s.workflowStepGetter != nil {
		step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
		if err != nil {
			s.logger.Warn("failed to get workflow step for MoveTask response",
				zap.String("workflow_step_id", task.WorkflowStepID),
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

func (s *Service) syncTaskStateForWorkflowMove(ctx context.Context, task *models.Task, oldStepID, newStepID string, opts MoveTaskOptions) error {
	newTerminal, err := s.terminalWorkflowStep(ctx, newStepID)
	if err != nil {
		return err
	}
	if newTerminal {
		if task.State == v1.TaskStateFailed && opts.AllowFailedToCompletedRecovery {
			task.State = v1.TaskStateCompleted
		} else if !models.IsTerminalTaskState(task.State) {
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

func (s *Service) pullNextTaskOnVacate(ctx context.Context, vacatedStepID, excludeTaskID string) {
	// A queue/WIP reconciliation is always wip_pull, unconditionally
	// overriding whatever trigger the caller that vacated the step declared
	// — the vacating move and the resulting pull are two distinct ledger
	// rows with two distinct causes. No single session initiates a pull, so
	// actor kind is always system with no session.
	ctx = steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerWIPPull,
		ActorKind: steptelemetry.ActorSystem,
	})
	vacatedStep := s.reconcilableStep(ctx, vacatedStepID)
	if vacatedStep == nil {
		return
	}
	occupants, ok := s.currentAdmittedOccupants(ctx, vacatedStep.ID)
	if !ok || (vacatedStep.WIPLimit > 0 && occupants >= vacatedStep.WIPLimit) {
		return
	}
	skipped := map[string]struct{}{excludeTaskID: {}}
	for vacatedStep.WIPLimit <= 0 || occupants < vacatedStep.WIPLimit {
		pulled := s.promoteNextQueuedTask(ctx, vacatedStep, occupants, skipped)
		if !pulled {
			return
		}
		occupants++
	}
}

func (s *Service) reconcilableStep(ctx context.Context, vacatedStepID string) *wfmodels.WorkflowStep {
	if s.workflowStepGetter == nil || vacatedStepID == "" {
		return nil
	}
	vacatedStep, err := s.workflowStepGetter.GetStep(ctx, vacatedStepID)
	if err != nil || vacatedStep == nil {
		return nil
	}
	return vacatedStep
}

func (s *Service) currentAdmittedOccupants(ctx context.Context, stepID string) (int, bool) {
	limitsRepo, ok := s.tasks.(workflowAdmittedCountRepository)
	if !ok {
		fallback, fallbackOK := s.tasks.(workflowMoveLimitsRepository)
		if !fallbackOK {
			s.logger.Warn("cannot reconcile queued task: WIP count repository unavailable", zap.String("step_id", stepID))
			return 0, false
		}
		occupants, err := fallback.CountTasksByWorkflowStepExcludingTask(ctx, stepID, "")
		if err != nil {
			s.logger.Warn("cannot reconcile queued task: failed to count step", zap.String("step_id", stepID), zap.Error(err))
			return 0, false
		}
		return occupants, true
	}
	occupants, err := limitsRepo.CountAdmittedTasksByWorkflowStep(ctx, stepID)
	if err != nil {
		s.logger.Warn("cannot reconcile queued task: failed to count step",
			zap.String("step_id", stepID), zap.Error(err))
		return 0, false
	}
	return occupants, true
}

func (s *Service) promoteNextQueuedTask(ctx context.Context, targetStep *wfmodels.WorkflowStep, position int, skipped map[string]struct{}) bool {
	candidate, err := s.nextQueuedCandidate(ctx, targetStep, skipped)
	if err != nil || candidate == nil {
		return false
	}
	fromStepID := candidate.WorkflowStepID
	oldWorkflowID := candidate.WorkflowID
	promotionSessionID := ""
	if fromStepID != targetStep.ID {
		promotionSession, blocked := s.feederCandidateSession(ctx, candidate.ID)
		if blocked {
			skipped[candidate.ID] = struct{}{}
			return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
		}
		if promotionSession != nil {
			promotionSessionID = promotionSession.ID
		}
	}
	if moveLifecyclePending(candidate) {
		skipped[candidate.ID] = struct{}{}
		return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
	}
	if candidate.WorkflowStepID == targetStep.ID {
		return s.promoteSameStepQueuedTask(ctx, candidate, fromStepID, targetStep, position, skipped)
	}
	return s.promoteFeederQueuedTask(ctx, candidate, fromStepID, oldWorkflowID, targetStep, position, skipped, promotionSessionID)
}

func queuedMoveExitPending(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	_, pending := task.Metadata[models.MetaKeyQueuedMoveExitPending]
	if !pending {
		return false
	}
	_, completed := task.Metadata[models.MetaKeyQueuedMoveExitCompleted]
	return !completed
}

func manualMoveLifecyclePending(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	if _, pending := task.Metadata[models.MetaKeyManualMoveLifecyclePending]; !pending {
		return false
	}
	_, completed := task.Metadata[models.MetaKeyManualMoveLifecycleCompleted]
	return !completed
}

func moveLifecyclePending(task *models.Task) bool {
	return queuedMoveExitPending(task) || manualMoveLifecyclePending(task)
}

func (s *Service) promoteSameStepQueuedTask(ctx context.Context, candidate *models.Task, fromStepID string, targetStep *wfmodels.WorkflowStep, position int, skipped map[string]struct{}) bool {
	oldState := candidate.State
	if candidate.Metadata == nil {
		candidate.Metadata = make(map[string]interface{})
	}
	candidate.WIPAdmitted = true
	candidate.QueuedForStepID = ""
	candidate.QueuedAt = nil
	candidate.Position = position
	candidate.Metadata[models.MetaKeyQueuePromotionPending] = true
	if err := s.syncTaskStateForQueuePromotion(ctx, candidate, targetStep); err != nil {
		s.logger.Warn("failed to prepare same-step queued promotion", zap.String("task_id", candidate.ID), zap.Error(err))
		skipped[candidate.ID] = struct{}{}
		return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
	}
	supported, claimed, err := promoteQueuedTaskAtomically(ctx, s.tasks, candidate, fromStepID, targetStep.ID, targetStep.WIPLimit)
	if supported {
		return s.finishAtomicQueuedPromotion(ctx, candidate, targetStep, position, skipped, claimed, err, oldState)
	} else if admissionRepo, ok := s.tasks.(workflowMoveAdmissionRepository); ok {
		claimed, err := admissionRepo.UpdateTaskWithWorkflowStepAdmission(ctx, candidate, targetStep.ID, targetStep.WIPLimit)
		if err != nil {
			s.logger.Warn("failed to promote same-step queued task", zap.String("task_id", candidate.ID), zap.Error(err))
			skipped[candidate.ID] = struct{}{}
			return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
		}
		if !claimed {
			skipped[candidate.ID] = struct{}{}
			return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
		}
	} else if err := s.tasks.UpdateTask(ctx, candidate); err != nil {
		return false
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, candidate, nil)
	if oldState != candidate.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, candidate, &oldState)
	}
	s.publishTaskEvent(ctx, events.TaskQueuePromoted, candidate, nil)
	return true
}

func (s *Service) finishAtomicQueuedPromotion(ctx context.Context, candidate *models.Task, targetStep *wfmodels.WorkflowStep, position int, skipped map[string]struct{}, claimed bool, err error, oldState v1.TaskState) bool {
	if err != nil {
		s.logger.Warn("failed to promote same-step queued task", zap.String("task_id", candidate.ID), zap.Error(err))
		return false
	}
	if !claimed {
		skipped[candidate.ID] = struct{}{}
		return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, candidate, nil)
	if oldState != candidate.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, candidate, &oldState)
	}
	s.publishTaskEvent(ctx, events.TaskQueuePromoted, candidate, nil)
	return true
}

func promoteQueuedTaskAtomically(ctx context.Context, tasks interface{}, task *models.Task, fromStepID, destinationStepID string, limit int) (bool, bool, error) {
	promoter, ok := tasks.(workflowQueuedTaskPromoter)
	if !ok {
		return false, false, nil
	}
	claimed, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, task, fromStepID, destinationStepID, limit)
	return true, claimed, err
}

func (s *Service) promoteFeederQueuedTask(ctx context.Context, candidate *models.Task, fromStepID, oldWorkflowID string, targetStep *wfmodels.WorkflowStep, position int, skipped map[string]struct{}, sessionID string) bool {
	oldState := candidate.State
	if candidate.Metadata == nil {
		candidate.Metadata = make(map[string]interface{})
	}
	candidate.WIPAdmitted = true
	candidate.QueuedForStepID = ""
	candidate.QueuedAt = nil
	candidate.Metadata[models.MetaKeyQueuePromotionPending] = true
	candidate.Position = position
	candidate.WorkflowID = targetStep.WorkflowID
	candidate.WorkflowStepID = targetStep.ID
	if err := s.syncTaskStateForQueuePromotion(ctx, candidate, targetStep); err != nil {
		s.logger.Warn("failed to prepare feeder queued promotion", zap.String("task_id", candidate.ID), zap.Error(err))
		skipped[candidate.ID] = struct{}{}
		return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
	}
	if promoter, ok := s.tasks.(workflowQueuedTaskPromoter); ok {
		claimed, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, candidate, fromStepID, targetStep.ID, targetStep.WIPLimit)
		if err != nil {
			s.logger.Warn("failed to promote feeder queued task", zap.String("task_id", candidate.ID), zap.Error(err))
			return false
		}
		if !claimed {
			skipped[candidate.ID] = struct{}{}
			return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
		}
		s.publishTaskEvent(ctx, events.TaskUpdated, candidate, nil, oldWorkflowID)
		if oldState != candidate.State {
			s.publishTaskEvent(ctx, events.TaskStateChanged, candidate, &oldState)
		}
		s.recordQueuedPromotion(ctx, candidate.ID, fromStepID, targetStep.ID)
		s.publishTaskMovedEvent(ctx, candidate, oldWorkflowID, fromStepID, targetStep.ID, sessionID)
		return true
	} else if admissionRepo, ok := s.tasks.(workflowMoveAdmissionRepository); ok {
		claimed, err := admissionRepo.UpdateTaskWithWorkflowStepAdmission(ctx, candidate, targetStep.ID, targetStep.WIPLimit)
		if err != nil {
			s.logger.Warn("failed to promote feeder queued task", zap.String("task_id", candidate.ID), zap.Error(err))
			skipped[candidate.ID] = struct{}{}
			return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
		}
		if !claimed {
			skipped[candidate.ID] = struct{}{}
			return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
		}
		s.publishTaskEvent(ctx, events.TaskUpdated, candidate, nil, oldWorkflowID)
		if oldState != candidate.State {
			s.publishTaskEvent(ctx, events.TaskStateChanged, candidate, &oldState)
		}
		s.recordQueuedPromotion(ctx, candidate.ID, fromStepID, targetStep.ID)
		s.publishTaskMovedEvent(ctx, candidate, oldWorkflowID, fromStepID, targetStep.ID, sessionID)
		return true
	}
	// ctx here still carries the identity of whoever triggered the move that
	// freed the slot, so MoveTaskWithOptions' authorizeTaskID applies to the
	// promoted candidate too. That is safe by construction: a step's
	// PullFromStepID resolves within the same workflow, a workflow belongs to
	// one workspace, and a workspace has one owner — so the candidate always
	// belongs to the caller who just passed the same check.
	//
	// If a future configuration ever allowed cross-workspace feeder pulls, this
	// would refuse and log below rather than promote. Leave it that way: do not
	// strip the identity to "fix" it. Promoting another user's task into a step
	// they cannot see is the worse outcome, and the atomic promoter above (which
	// the SQLite repository implements, so it is the only path production takes)
	// does not go through a guarded method at all.
	if _, err := s.MoveTaskWithOptions(ctx, candidate.ID, targetStep.WorkflowID, targetStep.ID, position, MoveTaskOptions{PreserveDeferredLaunch: true}); err != nil {
		skipped[candidate.ID] = struct{}{}
		s.logger.Warn("skipping queued task that could not be promoted", zap.String("task_id", candidate.ID), zap.String("to_step_id", targetStep.ID), zap.Error(err))
		return s.promoteNextQueuedTask(ctx, targetStep, position, skipped)
	}
	return true
}

func (s *Service) recordQueuedPromotion(ctx context.Context, taskID, fromStepID, toStepID string) {
	if s.stepHistoryRecorder == nil {
		return
	}
	session := s.resolvePrimaryOrActiveSession(ctx, taskID)
	if session == nil {
		return
	}
	if asyncRecorder, ok := s.stepHistoryRecorder.(asyncStepHistoryRecorder); ok {
		asyncRecorder.EnqueueStepTransition(session.ID, fromStepID, toStepID, wfmodels.StepTransitionTriggerQueuePromotion, nil, nil)
		return
	}
	if err := s.stepHistoryRecorder.CreateStepTransition(ctx, session.ID, fromStepID, toStepID, wfmodels.StepTransitionTriggerQueuePromotion, nil, nil); err != nil {
		s.logger.Warn("failed to record queued task promotion", zap.String("task_id", taskID), zap.Error(err))
	}
}

func (s *Service) feederCandidateSession(ctx context.Context, taskID string) (*models.TaskSession, bool) {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		s.logger.Warn("skipping feeder task after active session lookup failed", zap.String("task_id", taskID), zap.Error(err))
		return nil, true
	}
	var primary, fallback *models.TaskSession
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if isSessionMoveBlocked(session.State) {
			return nil, true
		}
		if !isSessionActive(session.State) {
			continue
		}
		if session.IsPrimary && primary == nil {
			primary = session
		}
		if !session.IsPrimary && fallback == nil {
			fallback = session
		}
	}
	if primary != nil {
		return primary, false
	}
	return fallback, false
}

func (s *Service) nextQueuedCandidate(ctx context.Context, targetStep *wfmodels.WorkflowStep, skipped map[string]struct{}) (*models.Task, error) {
	candidates, err := s.tasks.ListTasksByWorkflowStep(ctx, targetStep.ID)
	if err != nil {
		return nil, err
	}
	sameStep := findSameStepQueuedCandidate(candidates, targetStep.ID, skipped)
	feederTask, err := s.nextFeederQueuedCandidate(ctx, targetStep, skipped)
	if err != nil {
		return nil, err
	}
	if sameStep == nil {
		return feederTask, nil
	}
	if sameStep != nil {
		return sameStep, nil
	}
	return feederTask, nil
}

func findSameStepQueuedCandidate(candidates []*models.Task, stepID string, skipped map[string]struct{}) *models.Task {
	var selected *models.Task
	for _, candidate := range candidates {
		if candidate == nil || candidate.WIPAdmitted || candidate.QueuedForStepID != stepID {
			continue
		}
		if _, seen := skipped[candidate.ID]; seen {
			continue
		}
		if selected == nil || queuedTaskBefore(candidate, selected) {
			selected = candidate
		}
	}
	return selected
}

func (s *Service) nextFeederQueuedCandidate(
	ctx context.Context,
	targetStep *wfmodels.WorkflowStep,
	skipped map[string]struct{},
) (*models.Task, error) {
	if targetStep.PullFromStepID == "" {
		return nil, nil
	}
	excluded := skippedTaskIDs(skipped)
	if pullRepo, ok := s.tasks.(workflowQueuedPullRepository); ok {
		candidate, err := pullRepo.NextQueuedTaskForStepExcluding(
			ctx, targetStep.PullFromStepID, targetStep.ID, excluded,
		)
		if err != nil || candidate != nil {
			return candidate, err
		}
	}
	if legacyPullRepo, ok := s.tasks.(workflowPullRepository); ok {
		return legacyPullRepo.NextPullCandidateExcluding(ctx, targetStep.PullFromStepID, excluded)
	}
	return nil, nil
}

func queuedTaskBefore(left, right *models.Task) bool {
	if left.Position != right.Position {
		return left.Position < right.Position
	}
	priority := func(value string) int {
		switch value {
		case "critical":
			return 0
		case "high":
			return 1
		case priorityMedium:
			return 2
		case priorityLow:
			return 3
		default:
			return 4
		}
	}
	if priority(left.Priority) != priority(right.Priority) {
		return priority(left.Priority) < priority(right.Priority)
	}
	if left.QueuedAt != nil && right.QueuedAt != nil && !left.QueuedAt.Equal(*right.QueuedAt) {
		return left.QueuedAt.Before(*right.QueuedAt)
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.Before(right.CreatedAt)
	}
	return left.ID < right.ID
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

func (s *Service) updateMovedTask(ctx context.Context, task *models.Task, oldStepID string, targetStep *wfmodels.WorkflowStep, admittedState *v1.TaskState) (bool, error) {
	if targetStep == nil || oldStepID == targetStep.ID {
		if err := s.tasks.UpdateTask(ctx, task); err != nil {
			return false, err
		}
		return task.WIPAdmitted, nil
	}
	admissionRepo, ok := s.tasks.(workflowMoveAdmissionRepository)
	if !ok {
		return false, fmt.Errorf("workflow step admission repository unavailable for step %s", targetStep.ID)
	}
	if admissionWithState, ok := s.tasks.(workflowMoveAdmissionWithStateRepository); ok {
		return admissionWithState.UpdateTaskWithWorkflowStepAdmissionAndState(
			ctx, task, targetStep.ID, targetStep.WIPLimit, admittedState, true,
		)
	}

	// Keep compatibility with narrow test/dry-run repositories that expose
	// only the original admission method. Production repositories implement the
	// atomic variant above, so this fallback is never used for real moves.
	admitted, err := admissionRepo.UpdateTaskWithWorkflowStepAdmission(ctx, task, targetStep.ID, targetStep.WIPLimit)
	if err != nil {
		return false, err
	}
	if admitted && admittedState != nil {
		task.State = *admittedState
		delete(task.Metadata, models.MetaKeyQueuedMoveExitPending)
	} else if !admitted {
		if task.Metadata == nil {
			task.Metadata = make(map[string]interface{})
		}
		task.Metadata[models.MetaKeyQueuedMoveExitPending] = true
	}
	if err := s.tasks.UpdateTask(ctx, task); err != nil {
		return false, err
	}
	return admitted, nil
}

func (s *Service) syncTaskStateForQueuePromotion(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil {
		return nil
	}
	terminal, err := s.terminalWorkflowStep(ctx, targetStep.ID)
	if err != nil {
		return fmt.Errorf("sync promoted task state for %s: %w", task.ID, err)
	}
	if terminal {
		if !models.IsTerminalTaskState(task.State) {
			task.State = v1.TaskStateCompleted
		}
		return nil
	}
	if task.State == v1.TaskStateCompleted {
		task.State = v1.TaskStateTODO
	}
	return nil
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
	return targetStep, nil
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

// recordManualStepTransition writes the ADR 0015 audit row for a
// user/agent-initiated move. trigger is normally StepTransitionTriggerManual;
// callers driving an approval-gated transition (ApproveSession) pass
// StepTransitionTriggerApproval — a zero value defaults to Manual. It is a
// no-op when no recorder is wired or when the task has no session to record
// against — session_step_history.session_id is a NOT NULL FK to
// task_sessions, so a session-less move cannot be recorded without a
// schema change. Runtime writes use the workflow service's bounded worker;
// failures are logged and swallowed because this is best-effort telemetry.
func (s *Service) recordManualStepTransition(ctx context.Context, sessionID, fromStepID, toStepID string, trigger wfmodels.StepTransitionTrigger, actors ...wfmodels.StepTransitionActor) {
	if s.stepHistoryRecorder == nil {
		return
	}
	if sessionID == "" {
		s.logger.Debug("skipping manual step transition audit: task has no session",
			zap.String("from_step_id", fromStepID),
			zap.String("to_step_id", toStepID))
		return
	}
	if trigger == "" {
		trigger = wfmodels.StepTransitionTriggerManual
	}
	var actorID *string
	actor := wfmodels.StepTransitionActorHuman
	if len(actors) > 0 {
		actor = actors[0]
	}
	if actor == wfmodels.StepTransitionActorHuman {
		if identity, ok := authn.IdentityFromContext(ctx); ok && identity.UserID != "" {
			actorID = &identity.UserID
		}
	}
	if asyncRecorder, ok := s.stepHistoryRecorder.(asyncStepHistoryRecorder); ok {
		asyncRecorder.EnqueueStepTransition(sessionID, fromStepID, toStepID, trigger, actorID, nil)
		return
	}
	// The step change is already durably persisted by the time this runs.
	// Use a detached, bounded context so a cancelled request context (client
	// disconnect, turn-end) cannot drop the audit row for a transition that
	// already committed.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constants.StepHistoryWriteTimeout)
	defer cancel()
	if err := s.stepHistoryRecorder.CreateStepTransition(
		writeCtx, sessionID, fromStepID, toStepID, trigger, actorID, nil,
	); err != nil {
		s.logger.Warn("failed to record manual step transition",
			zap.String("session_id", sessionID),
			zap.String("from_step_id", fromStepID),
			zap.String("to_step_id", toStepID),
			zap.Error(err))
	}
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
	bulkActorKind, bulkActorID := steptelemetry.HumanOrSystemActor(ctx)
	ctx = steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerBulkMove, ActorKind: bulkActorKind, ActorID: bulkActorID,
	})

	tasks, err := s.validateSelectedMoveBatch(ctx, ids, targetWorkflowID, targetStepID)
	if err != nil {
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

// BulkMoveTasks moves all tasks from a source workflow/step to a target workflow/step.
// If sourceStepID is empty, all tasks in the source workflow are moved.
func (s *Service) BulkMoveTasks(ctx context.Context, sourceWorkflowID, sourceStepID, targetWorkflowID, targetStepID string) (*BulkMoveTasksResult, error) {
	bulkActorKind, bulkActorID := steptelemetry.HumanOrSystemActor(ctx)
	ctx = steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger: steptelemetry.TriggerBulkMove, ActorKind: bulkActorKind, ActorID: bulkActorID,
	})

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
