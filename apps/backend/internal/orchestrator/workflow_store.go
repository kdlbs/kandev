package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// taskUpdatedPublisher is the minimal hook the workflow store needs to emit
// task.updated events. The orchestrator Service binds this to its shared
// publishTaskUpdated helper so the publisher wiring stays in one place.
type taskUpdatedPublisher func(ctx context.Context, task *models.Task)

type taskMovedPublisher func(ctx context.Context, task *models.Task, fromWorkflowID, fromStepID, toStepID, sessionID string)

type workflowMoveLimitsRepository interface {
	CountTasksByWorkflowStepExcludingTask(ctx context.Context, stepID, excludeTaskID string) (int, error)
}

type workflowPullRepository interface {
	NextPullCandidate(ctx context.Context, stepID, excludeTaskID string) (*models.Task, error)
}

// workflowStore implements engine.TransitionStore by delegating to the
// orchestrator's existing repositories and services.
type workflowStore struct {
	repo               sessionExecutorStore
	workflowStepGetter WorkflowStepGetter
	agentManager       executor.AgentManagerClient
	publishTaskUpdated taskUpdatedPublisher
	publishTaskMoved   taskMovedPublisher
	logger             *logger.Logger
	appliedOps         sync.Map
}

func newWorkflowStore(
	repo sessionExecutorStore,
	stepGetter WorkflowStepGetter,
	agentMgr executor.AgentManagerClient,
	publishTaskUpdated taskUpdatedPublisher,
	log *logger.Logger,
	publishTaskMoved ...taskMovedPublisher,
) *workflowStore {
	var moved taskMovedPublisher
	if len(publishTaskMoved) > 0 {
		moved = publishTaskMoved[0]
	}
	return &workflowStore{
		repo:               repo,
		workflowStepGetter: stepGetter,
		agentManager:       agentMgr,
		publishTaskUpdated: publishTaskUpdated,
		publishTaskMoved:   moved,
		logger:             log,
	}
}

func (s *workflowStore) LoadState(ctx context.Context, taskID, sessionID string) (engine.MachineState, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return engine.MachineState{}, fmt.Errorf("load task %s: %w", taskID, err)
	}

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return engine.MachineState{}, fmt.Errorf("load session %s: %w", sessionID, err)
	}

	isPassthrough := false
	if s.agentManager != nil {
		isPassthrough = s.agentManager.IsPassthroughSession(ctx, sessionID)
	}

	return assembleMachineState(task, session, isPassthrough), nil
}

func (s *workflowStore) LoadStep(ctx context.Context, _, stepID string) (engine.StepSpec, error) {
	step, err := s.workflowStepGetter.GetStep(ctx, stepID)
	if err != nil {
		return engine.StepSpec{}, fmt.Errorf("load step %s: %w", stepID, err)
	}
	return engine.CompileStep(step), nil
}

func (s *workflowStore) LoadNextStep(ctx context.Context, workflowID string, currentPosition int) (engine.StepSpec, error) {
	step, err := s.workflowStepGetter.GetNextStepByPosition(ctx, workflowID, currentPosition)
	if err != nil {
		return engine.StepSpec{}, fmt.Errorf("load next step after position %d: %w", currentPosition, err)
	}
	if step == nil {
		return engine.StepSpec{}, fmt.Errorf("no next step after position %d in workflow %s", currentPosition, workflowID)
	}
	return engine.CompileStep(step), nil
}

func (s *workflowStore) LoadPreviousStep(ctx context.Context, workflowID string, currentPosition int) (engine.StepSpec, error) {
	step, err := s.workflowStepGetter.GetPreviousStepByPosition(ctx, workflowID, currentPosition)
	if err != nil {
		return engine.StepSpec{}, fmt.Errorf("load previous step before position %d: %w", currentPosition, err)
	}
	if step == nil {
		return engine.StepSpec{}, fmt.Errorf("no previous step before position %d in workflow %s", currentPosition, workflowID)
	}
	return engine.CompileStep(step), nil
}

func (s *workflowStore) ApplyTransition(ctx context.Context, taskID, sessionID, fromStepID, toStepID string, _ engine.Trigger) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task for transition: %w", err)
	}
	targetStep, err := s.workflowStepGetter.GetStep(ctx, toStepID)
	if err != nil {
		return fmt.Errorf("load target step for transition: %w", err)
	}
	if err := s.validateWIPLimit(ctx, task, targetStep); err != nil {
		return err
	}

	task.WorkflowStepID = toStepID
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("update task workflow step: %w", err)
	}

	s.publishTaskUpdated(ctx, task)

	if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
		s.logger.Warn("failed to clear session review status",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	s.logger.Info("workflow transition applied",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step_id", fromStepID),
		zap.String("to_step_id", toStepID))

	s.pullNextTaskOnVacate(ctx, fromStepID, taskID)

	return nil
}

func (s *workflowStore) validateWIPLimit(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil || targetStep.WIPLimit <= 0 || task.WorkflowStepID == targetStep.ID {
		return nil
	}
	limitsRepo, ok := s.repo.(workflowMoveLimitsRepository)
	if !ok {
		return fmt.Errorf("WIP limit cannot be checked for workflow step %s", targetStep.ID)
	}
	occupants, err := limitsRepo.CountTasksByWorkflowStepExcludingTask(ctx, targetStep.ID, task.ID)
	if err != nil {
		return fmt.Errorf("count target workflow step tasks: %w", err)
	}
	if occupants >= targetStep.WIPLimit {
		return fmt.Errorf("WIP limit exceeded for workflow step %s: limit %d already occupied", targetStep.ID, targetStep.WIPLimit)
	}
	return nil
}

func (s *workflowStore) pullNextTaskOnVacate(ctx context.Context, vacatedStepID, excludeTaskID string) {
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
	for occupants < vacatedStep.WIPLimit {
		pulled := s.pullOneFeederTask(ctx, pullRepo, vacatedStep, occupants, excludeTaskID)
		if !pulled {
			return
		}
		occupants++
	}
}

func (s *workflowStore) pullEnabledStep(ctx context.Context, vacatedStepID string) *wfmodels.WorkflowStep {
	if s.workflowStepGetter == nil || s.publishTaskMoved == nil || vacatedStepID == "" {
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

func (s *workflowStore) pullRepositories(stepID string) (workflowMoveLimitsRepository, workflowPullRepository, bool) {
	limitsRepo, ok := s.repo.(workflowMoveLimitsRepository)
	if !ok {
		s.logger.Warn("cannot pull feeder task: WIP limit repository unavailable",
			zap.String("step_id", stepID))
		return nil, nil, false
	}
	pullRepo, ok := s.repo.(workflowPullRepository)
	if !ok {
		s.logger.Warn("cannot pull feeder task: pull repository unavailable",
			zap.String("step_id", stepID))
		return nil, nil, false
	}
	return limitsRepo, pullRepo, true
}

func (s *workflowStore) currentWIPOccupants(ctx context.Context, limitsRepo workflowMoveLimitsRepository, stepID string) (int, bool) {
	occupants, err := limitsRepo.CountTasksByWorkflowStepExcludingTask(ctx, stepID, "")
	if err != nil {
		s.logger.Warn("cannot pull feeder task: failed to count vacated step",
			zap.String("step_id", stepID), zap.Error(err))
		return 0, false
	}
	return occupants, true
}

func (s *workflowStore) pullOneFeederTask(
	ctx context.Context,
	pullRepo workflowPullRepository,
	vacatedStep *wfmodels.WorkflowStep,
	position int,
	excludeTaskID string,
) bool {
	candidate, err := pullRepo.NextPullCandidate(ctx, vacatedStep.PullFromStepID, excludeTaskID)
	if err != nil {
		s.logger.Warn("cannot pull feeder task: failed to select candidate",
			zap.String("step_id", vacatedStep.ID), zap.Error(err))
		return false
	}
	if candidate == nil {
		return false
	}
	fromWorkflowID := candidate.WorkflowID
	fromStepID := candidate.WorkflowStepID
	candidate.WorkflowID = vacatedStep.WorkflowID
	candidate.WorkflowStepID = vacatedStep.ID
	candidate.Position = position
	candidate.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, candidate); err != nil {
		s.logger.Warn("failed to pull feeder task into vacated WIP-limited step",
			zap.String("task_id", candidate.ID), zap.Error(err))
		return false
	}
	s.publishTaskUpdated(ctx, candidate)
	sessionID := ""
	if session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, candidate.ID); err == nil && session != nil {
		sessionID = session.ID
	}
	s.publishTaskMoved(ctx, candidate, fromWorkflowID, fromStepID, vacatedStep.ID, sessionID)
	return true
}

func (s *workflowStore) PersistData(ctx context.Context, sessionID string, data map[string]any) error {
	// Read existing workflow_data to merge new keys into it.
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session for data persist: %w", err)
	}
	existing, _ := session.Metadata["workflow_data"].(map[string]interface{})
	if existing == nil {
		existing = make(map[string]interface{})
	}
	for k, v := range data {
		existing[k] = v
	}
	// Use SetSessionMetadataKey (json_set) to atomically set workflow_data
	// without clobbering other metadata keys (plan_mode, prepare_result).
	if err := s.repo.SetSessionMetadataKey(ctx, sessionID, "workflow_data", existing); err != nil {
		return fmt.Errorf("persist workflow data: %w", err)
	}
	return nil
}

func (s *workflowStore) IsOperationApplied(_ context.Context, operationID string) (bool, error) {
	if operationID == "" {
		return false, nil
	}
	_, ok := s.appliedOps.Load(operationID)
	return ok, nil
}

func (s *workflowStore) MarkOperationApplied(_ context.Context, operationID string) error {
	if operationID == "" {
		return nil
	}
	s.appliedOps.Store(operationID, true)
	return nil
}

// Verify interface compliance at compile time.
var _ engine.TransitionStore = (*workflowStore)(nil)
