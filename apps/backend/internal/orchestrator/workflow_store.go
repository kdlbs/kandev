package orchestrator

import (
	"context"
	"errors"
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
type taskUpdatedPublisher func(ctx context.Context, task *models.Task, oldWorkflowIDs ...string)

type taskMovedPublisher func(ctx context.Context, task *models.Task, fromWorkflowID, fromStepID, toStepID, sessionID string)
type taskQueuePromotedPublisher func(ctx context.Context, task *models.Task)

type workflowMoveLimitsRepository interface {
	CountTasksByWorkflowStepExcludingTask(ctx context.Context, stepID, excludeTaskID string) (int, error)
}

type workflowAdmittedCountRepository interface {
	CountAdmittedTasksByWorkflowStep(ctx context.Context, stepID string) (int, error)
}

type workflowLimitedMoveRepository interface {
	UpdateTaskIfWorkflowStepHasCapacity(ctx context.Context, task *models.Task, targetStepID, excludeTaskID string, limit int) error
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

type workflowStepTaskLister interface {
	ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error)
}

type queuedTaskLister interface {
	ListQueuedTasks(ctx context.Context) ([]*models.Task, error)
}

// workflowStore implements engine.TransitionStore by delegating to the
// orchestrator's existing repositories and services.
type workflowStore struct {
	repo                sessionExecutorStore
	workflowStepGetter  WorkflowStepGetter
	agentManager        executor.AgentManagerClient
	publishTaskUpdated  taskUpdatedPublisher
	publishTaskMoved    taskMovedPublisher
	publishTaskPromoted taskQueuePromotedPublisher
	logger              *logger.Logger
	appliedOps          sync.Map
}

func newWorkflowStore(
	repo sessionExecutorStore,
	stepGetter WorkflowStepGetter,
	agentMgr executor.AgentManagerClient,
	publishTaskUpdated taskUpdatedPublisher,
	log *logger.Logger,
	publishers ...interface{},
) *workflowStore {
	var moved taskMovedPublisher
	var promoted taskQueuePromotedPublisher
	for _, publisher := range publishers {
		switch value := publisher.(type) {
		case taskMovedPublisher:
			moved = value
		case func(context.Context, *models.Task, string, string, string, string):
			moved = taskMovedPublisher(value)
		case taskQueuePromotedPublisher:
			promoted = value
		case func(context.Context, *models.Task):
			promoted = taskQueuePromotedPublisher(value)
		}
	}
	return &workflowStore{
		repo:                repo,
		workflowStepGetter:  stepGetter,
		agentManager:        agentMgr,
		publishTaskUpdated:  publishTaskUpdated,
		publishTaskMoved:    moved,
		publishTaskPromoted: promoted,
		logger:              log,
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
	// Keep WorkflowID in sync with the target step's owning workflow. Most
	// callers transition within the same workflow (targetStep.WorkflowID ==
	// task.WorkflowID already), but applyPendingMove uses this path for
	// cross-workflow move_task_kandev hand-offs too — without this, the task
	// would end up with a step ID from a workflow its WorkflowID doesn't match.
	oldWorkflowID := task.WorkflowID
	if targetStep != nil {
		task.WorkflowID = targetStep.WorkflowID
	}
	task.WorkflowStepID = toStepID
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.QueuedAt = nil
	task.UpdatedAt = time.Now().UTC()
	if err := s.updateTransitionTask(ctx, task, targetStep); err != nil {
		return fmt.Errorf("update task workflow step: %w", err)
	}

	// Pass the pre-move workflow ID through so cross-workflow transitions
	// carry old_workflow_id on the task.updated payload — the frontend uses
	// that field to remove the task from its previous workflow's snapshot
	// instead of leaving a stale duplicate until reload.
	s.publishTaskUpdated(ctx, task, oldWorkflowID)

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

func (s *workflowStore) updateTransitionTask(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil || targetStep.WIPLimit <= 0 {
		return s.repo.UpdateTask(ctx, task)
	}
	limitedRepo, ok := s.repo.(workflowLimitedMoveRepository)
	if !ok {
		return fmt.Errorf("WIP limit cannot be checked for workflow step %s", targetStep.ID)
	}
	return limitedRepo.UpdateTaskIfWorkflowStepHasCapacity(ctx, task, targetStep.ID, task.ID, targetStep.WIPLimit)
}

func (s *workflowStore) pullNextTaskOnVacate(ctx context.Context, vacatedStepID, excludeTaskID string) {
	vacatedStep := s.pullEnabledStep(ctx, vacatedStepID)
	if vacatedStep == nil {
		return
	}
	limitsRepo, pullRepo, limitedRepo, ok := s.pullRepositories(vacatedStep.ID)
	if !ok {
		return
	}
	occupants, ok := s.currentWIPOccupants(ctx, limitsRepo, vacatedStep.ID)
	if !ok || (vacatedStep.WIPLimit > 0 && occupants >= vacatedStep.WIPLimit) {
		return
	}
	skipped := map[string]struct{}{excludeTaskID: {}}
	for vacatedStep.WIPLimit <= 0 || occupants < vacatedStep.WIPLimit {
		pulled := s.pullOneFeederTask(ctx, pullRepo, limitedRepo, vacatedStep, occupants, skipped)
		if !pulled {
			return
		}
		occupants++
	}
}

// ReconcileQueuedTasks repairs persisted queues after a restart and after
// workflow-step configuration changes. The destination marker makes this
// bounded to the set of steps that actually have work waiting.
func (s *workflowStore) ReconcileQueuedTasks(ctx context.Context) {
	lister, ok := s.repo.(queuedTaskLister)
	if !ok {
		return
	}
	queued, err := lister.ListQueuedTasks(ctx)
	if err != nil {
		s.logger.Warn("failed to list queued tasks for reconciliation", zap.Error(err))
		return
	}
	seen := make(map[string]struct{}, len(queued))
	for _, task := range queued {
		if task == nil || task.QueuedForStepID == "" {
			continue
		}
		if _, exists := seen[task.QueuedForStepID]; exists {
			continue
		}
		seen[task.QueuedForStepID] = struct{}{}
		s.pullNextTaskOnVacate(ctx, task.QueuedForStepID, "")
	}
}

func (s *workflowStore) pullEnabledStep(ctx context.Context, vacatedStepID string) *wfmodels.WorkflowStep {
	if s.workflowStepGetter == nil || vacatedStepID == "" {
		return nil
	}
	vacatedStep, err := s.workflowStepGetter.GetStep(ctx, vacatedStepID)
	if err != nil || vacatedStep == nil {
		return nil
	}
	if vacatedStep.PullFromStepID == vacatedStep.ID {
		return nil
	}
	return vacatedStep
}

func (s *workflowStore) pullRepositories(stepID string) (workflowMoveLimitsRepository, workflowPullRepository, workflowLimitedMoveRepository, bool) {
	limitsRepo, ok := s.repo.(workflowMoveLimitsRepository)
	if !ok {
		s.logger.Warn("cannot pull feeder task: WIP limit repository unavailable",
			zap.String("step_id", stepID))
		return nil, nil, nil, false
	}
	pullRepo, ok := s.repo.(workflowPullRepository)
	if !ok {
		s.logger.Warn("cannot pull feeder task: pull repository unavailable",
			zap.String("step_id", stepID))
		return nil, nil, nil, false
	}
	limitedRepo, ok := s.repo.(workflowLimitedMoveRepository)
	if !ok {
		s.logger.Warn("cannot pull feeder task: transactional WIP limit repository unavailable",
			zap.String("step_id", stepID))
		return nil, nil, nil, false
	}
	return limitsRepo, pullRepo, limitedRepo, true
}

func (s *workflowStore) currentWIPOccupants(ctx context.Context, limitsRepo workflowMoveLimitsRepository, stepID string) (int, bool) {
	if admittedRepo, ok := s.repo.(workflowAdmittedCountRepository); ok {
		occupants, err := admittedRepo.CountAdmittedTasksByWorkflowStep(ctx, stepID)
		if err != nil {
			s.logger.Warn("cannot pull feeder task: failed to count admitted tasks",
				zap.String("step_id", stepID), zap.Error(err))
			return 0, false
		}
		return occupants, true
	}
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
	limitedRepo workflowLimitedMoveRepository,
	vacatedStep *wfmodels.WorkflowStep,
	position int,
	skipped map[string]struct{},
) bool {
	if candidate := s.nextQueuedSameStepTask(ctx, vacatedStep.ID, skipped); candidate != nil {
		return s.promoteSameStepTask(ctx, candidate, vacatedStep, position, skipped, pullRepo, limitedRepo)
	}
	if vacatedStep.PullFromStepID == "" {
		return false
	}
	if s.publishTaskMoved == nil {
		return false
	}
	for {
		var candidate *models.Task
		var err error
		if queuedRepo, ok := s.repo.(workflowQueuedPullRepository); ok {
			candidate, err = queuedRepo.NextQueuedTaskForStepExcluding(ctx, vacatedStep.PullFromStepID, vacatedStep.ID, skippedTaskIDs(skipped))
		} else {
			candidate, err = pullRepo.NextPullCandidateExcluding(ctx, vacatedStep.PullFromStepID, skippedTaskIDs(skipped))
		}
		if err != nil {
			s.logger.Warn("cannot pull feeder task: failed to select candidate",
				zap.String("step_id", vacatedStep.ID), zap.Error(err))
			return false
		}
		if candidate == nil {
			return false
		}
		if s.feederCandidateBlocked(ctx, candidate.ID) {
			skipped[candidate.ID] = struct{}{}
			continue
		}
		fromWorkflowID := candidate.WorkflowID
		fromStepID := candidate.WorkflowStepID
		candidate.WorkflowID = vacatedStep.WorkflowID
		candidate.WorkflowStepID = vacatedStep.ID
		candidate.Position = position
		candidate.UpdatedAt = time.Now().UTC()
		if promoter, ok := s.repo.(workflowQueuedTaskPromoter); ok {
			claimed, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, candidate, fromStepID, vacatedStep.ID, vacatedStep.WIPLimit)
			if err != nil {
				s.logger.Warn("failed to promote feeder task", zap.String("task_id", candidate.ID), zap.Error(err))
				return false
			}
			if !claimed {
				skipped[candidate.ID] = struct{}{}
				continue
			}
		} else if err := limitedRepo.UpdateTaskIfWorkflowStepHasCapacity(ctx, candidate, vacatedStep.ID, candidate.ID, vacatedStep.WIPLimit); err != nil {
			skipped[candidate.ID] = struct{}{}
			s.logger.Warn("skipping feeder task that could not be pulled",
				zap.String("task_id", candidate.ID), zap.Error(err))
			continue
		}
		s.publishTaskUpdated(ctx, candidate)
		sessionID := ""
		if session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, candidate.ID); err == nil && session != nil {
			sessionID = session.ID
		}
		s.publishTaskMoved(ctx, candidate, fromWorkflowID, fromStepID, vacatedStep.ID, sessionID)
		return true
	}
}

func (s *workflowStore) promoteSameStepTask(ctx context.Context, candidate *models.Task, step *wfmodels.WorkflowStep, position int, skipped map[string]struct{}, pullRepo workflowPullRepository, limitedRepo workflowLimitedMoveRepository) bool {
	fromStepID := candidate.WorkflowStepID
	candidate.WIPAdmitted = true
	candidate.QueuedForStepID = ""
	candidate.QueuedAt = nil
	candidate.Position = position
	candidate.UpdatedAt = time.Now().UTC()
	if promoter, ok := s.repo.(workflowQueuedTaskPromoter); ok {
		claimed, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, candidate, fromStepID, step.ID, step.WIPLimit)
		if err != nil {
			return false
		}
		if !claimed {
			skipped[candidate.ID] = struct{}{}
			return s.pullOneFeederTask(ctx, pullRepo, limitedRepo, step, position, skipped)
		}
	} else if err := limitedRepo.UpdateTaskIfWorkflowStepHasCapacity(ctx, candidate, step.ID, candidate.ID, step.WIPLimit); err != nil {
		return false
	}
	s.publishTaskUpdated(ctx, candidate)
	if s.publishTaskPromoted != nil {
		s.publishTaskPromoted(ctx, candidate)
	}
	return true
}

func (s *workflowStore) nextQueuedSameStepTask(ctx context.Context, stepID string, skipped map[string]struct{}) *models.Task {
	lister, ok := s.repo.(workflowStepTaskLister)
	if !ok {
		return nil
	}
	candidates, err := lister.ListTasksByWorkflowStep(ctx, stepID)
	if err != nil {
		return nil
	}
	var best *models.Task
	for _, candidate := range candidates {
		if candidate == nil || candidate.WIPAdmitted || candidate.QueuedForStepID != stepID {
			continue
		}
		if _, seen := skipped[candidate.ID]; seen {
			continue
		}
		if best == nil || queuedTaskBefore(candidate, best) {
			best = candidate
		}
	}
	return best
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
		case "medium":
			return 2
		case "low":
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

func (s *workflowStore) feederCandidateBlocked(ctx context.Context, taskID string) bool {
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return false
		}
		s.logger.Warn("skipping feeder task after active session lookup failed",
			zap.String("task_id", taskID), zap.Error(err))
		return true
	}
	if session == nil {
		return false
	}
	return session.State == models.TaskSessionStateStarting ||
		session.State == models.TaskSessionStateRunning
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
