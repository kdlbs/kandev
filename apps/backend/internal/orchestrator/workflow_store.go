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
)

// taskUpdatedPublisher is the minimal hook the workflow store needs to emit
// task.updated events. The orchestrator Service binds this to its shared
// publishTaskUpdated helper so the publisher wiring stays in one place.
type taskUpdatedPublisher func(ctx context.Context, task *models.Task)

// workflowStore implements engine.TransitionStore by delegating to the
// orchestrator's existing repositories and services.
type workflowStore struct {
	repo               sessionExecutorStore
	workflowStepGetter WorkflowStepGetter
	agentManager       executor.AgentManagerClient
	publishTaskUpdated taskUpdatedPublisher
	logger             *logger.Logger
	appliedOps         sync.Map
}

func newWorkflowStore(
	repo sessionExecutorStore,
	stepGetter WorkflowStepGetter,
	agentMgr executor.AgentManagerClient,
	publishTaskUpdated taskUpdatedPublisher,
	log *logger.Logger,
) *workflowStore {
	return &workflowStore{
		repo:               repo,
		workflowStepGetter: stepGetter,
		agentManager:       agentMgr,
		publishTaskUpdated: publishTaskUpdated,
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

	return nil
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
