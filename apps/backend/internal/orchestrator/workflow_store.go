package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/routing"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var ErrTransitionSourceChanged = errors.New("source workflow step changed before transition commit")

// taskUpdatedPublisher is the minimal hook the workflow store needs to emit
// task.updated events. The orchestrator Service binds this to its shared
// publishTaskUpdated helper so the publisher wiring stays in one place.
type taskUpdatedPublisher func(ctx context.Context, task *models.Task, oldWorkflowIDs ...string)

type taskMovedPublisher func(ctx context.Context, task *models.Task, fromWorkflowID, fromStepID, toStepID, sessionID string)
type taskQueuePromotedPublisher func(ctx context.Context, task *models.Task)
type taskStateChangedPublisher func(ctx context.Context, task *models.Task, oldState v1.TaskState)

type guardedTransitionLifecycle func(ctx context.Context, taskID, sessionID, fromStepID, toStepID string, trigger engine.Trigger) (bool, error)

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

// workflowMoveAdmissionCASRepository is the AC-46/48 compare-and-swap
// variant of workflowMoveAdmissionRepository. It is a separate interface
// (rather than a new method on workflowMoveAdmissionRepository) so fakes in
// tests that only exercise the unconditional move path do not need to grow a
// method they never call.
type workflowMoveAdmissionCASRepository interface {
	UpdateTaskWithWorkflowStepAdmissionIfAtStep(
		ctx context.Context, task *models.Task, expectedStepID, targetStepID string, limit int,
	) (applied bool, err error)
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

func queuedMoveExitPending(task *models.Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	if _, pending := task.Metadata[models.MetaKeyQueuedMoveExitPending]; !pending {
		return false
	}
	_, completed := task.Metadata[models.MetaKeyQueuedMoveExitCompleted]
	return !completed
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
	publishStateChanged taskStateChangedPublisher
	logger              *logger.Logger
	stepHistoryRecorder StepHistoryRecorder
	guardedLifecycle    guardedTransitionLifecycle
	appliedOps          sync.Map
	stepCache           *stepSpecCache
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
	var stateChanged taskStateChangedPublisher
	var history StepHistoryRecorder
	for _, publisher := range publishers {
		switch value := publisher.(type) {
		case taskMovedPublisher:
			moved = value
		case func(context.Context, *models.Task, string, string, string, string):
			moved = taskMovedPublisher(value)
		case taskQueuePromotedPublisher:
			promoted = value
		case StepHistoryRecorder:
			// Keep transition-history ownership in the workflow store for
			// queue promotions, which otherwise bypass the normal move API.
			history = value
		case func(context.Context, *models.Task):
			promoted = taskQueuePromotedPublisher(value)
		case taskStateChangedPublisher:
			stateChanged = value
		case func(context.Context, *models.Task, v1.TaskState):
			stateChanged = taskStateChangedPublisher(value)
		}
	}
	return &workflowStore{
		repo:                repo,
		workflowStepGetter:  stepGetter,
		agentManager:        agentMgr,
		publishTaskUpdated:  publishTaskUpdated,
		publishTaskMoved:    moved,
		publishTaskPromoted: promoted,
		publishStateChanged: stateChanged,
		logger:              log,
		stepHistoryRecorder: history,
		stepCache:           newStepSpecCache(),
	}
}

func (s *workflowStore) setGuardedTransitionLifecycle(fn guardedTransitionLifecycle) {
	s.guardedLifecycle = fn
}

// LoadState tolerates a blank sessionID (the AC-62/F38 case of a task with
// zero task_sessions rows) by skipping the session lookup entirely rather
// than erroring: CurrentStepID always comes from the task row, matching the
// sessionID == "" sentinel convention already used elsewhere (AC-16a).
func (s *workflowStore) LoadState(ctx context.Context, taskID, sessionID string) (engine.MachineState, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return engine.MachineState{}, fmt.Errorf("load task %s: %w", taskID, err)
	}

	if sessionID == "" {
		return assembleMachineState(task, nil, false), nil
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

// LoadStep returns stepID's compiled spec, serving it from the process-local
// stepCache when fresh. On a miss, concurrent callers for the same stepID
// coalesce onto one DB read + compile via stepCache's singleflight group. The
// returned StepSpec is shared across callers and must be treated as
// immutable — see stepSpecCache's doc comment.
func (s *workflowStore) LoadStep(ctx context.Context, _, stepID string) (engine.StepSpec, error) {
	fetch := func() (engine.StepSpec, error) {
		step, err := s.workflowStepGetter.GetStep(ctx, stepID)
		if err != nil {
			return engine.StepSpec{}, fmt.Errorf("load step %s: %w", stepID, err)
		}
		if step == nil {
			return engine.StepSpec{}, fmt.Errorf("step %s not found", stepID)
		}
		return engine.CompileStep(step), nil
	}
	if s.stepCache == nil {
		return fetch()
	}
	return s.stepCache.getOrLoadStep(stepID, fetch)
}

// LoadNextStep returns the compiled spec of the step after currentPosition in
// workflowID, serving it from stepCache when fresh and coalescing concurrent
// misses on the same position. See LoadStep on cache sharing/immutability.
func (s *workflowStore) LoadNextStep(ctx context.Context, workflowID string, currentPosition int) (engine.StepSpec, error) {
	fetch := func() (engine.StepSpec, error) {
		step, err := s.workflowStepGetter.GetNextStepByPosition(ctx, workflowID, currentPosition)
		if err != nil {
			return engine.StepSpec{}, fmt.Errorf("load next step after position %d: %w", currentPosition, err)
		}
		if step == nil {
			return engine.StepSpec{}, fmt.Errorf("no next step after position %d in workflow %s", currentPosition, workflowID)
		}
		return engine.CompileStep(step), nil
	}
	if s.stepCache == nil {
		return fetch()
	}
	return s.stepCache.getOrLoadPos(workflowID, posDirectionNext, currentPosition, fetch)
}

// LoadPreviousStep returns the compiled spec of the step before
// currentPosition in workflowID, serving it from stepCache when fresh and
// coalescing concurrent misses on the same position. See LoadStep on cache
// sharing/immutability.
func (s *workflowStore) LoadPreviousStep(ctx context.Context, workflowID string, currentPosition int) (engine.StepSpec, error) {
	fetch := func() (engine.StepSpec, error) {
		step, err := s.workflowStepGetter.GetPreviousStepByPosition(ctx, workflowID, currentPosition)
		if err != nil {
			return engine.StepSpec{}, fmt.Errorf("load previous step before position %d: %w", currentPosition, err)
		}
		if step == nil {
			return engine.StepSpec{}, fmt.Errorf("no previous step before position %d in workflow %s", currentPosition, workflowID)
		}
		return engine.CompileStep(step), nil
	}
	if s.stepCache == nil {
		return fetch()
	}
	return s.stepCache.getOrLoadPos(workflowID, posDirectionPrev, currentPosition, fetch)
}

func (s *workflowStore) ApplyTransition(ctx context.Context, taskID, sessionID, fromStepID, toStepID string, trigger engine.Trigger) error {
	current, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load transition source step: %w", err)
	}
	if current.WorkflowStepID == toStepID {
		return s.recordAlreadySatisfiedTransition(ctx, current, toStepID)
	}
	if fromStepID == "" {
		// Legacy/internal callers used an empty source to mean "the task's
		// current persisted step". Resolve that generation before entering the
		// CAS arbiter instead of turning the empty sentinel into an unconditional
		// write. A concurrent lane change after this read is still rejected.
		fromStepID = current.WorkflowStepID
	}
	// ApplyTransition is already called from a lifecycle owner. Commit through
	// the raw CAS path so the optional guardedLifecycle bridge cannot re-enter
	// on_exit/on_enter around the same physical transition.
	applied, err := s.applyTransitionIfAtStepAllowMissingTarget(ctx, taskID, sessionID, fromStepID, toStepID, trigger)
	if err != nil {
		return err
	}
	if !applied {
		return ErrTransitionSourceChanged
	}
	return nil
}

func (s *workflowStore) recordAlreadySatisfiedTransition(ctx context.Context, task *models.Task, targetStepID string) error {
	recorder, ok := s.repo.(interface {
		RecordWorkflowRouteOperation(context.Context, routing.Operation) error
	})
	if !ok {
		s.logger.Warn("workflow route operation recorder unavailable for already-satisfied transition",
			zap.String("task_id", task.ID), zap.String("target_step_id", targetStepID))
		return nil
	}
	operation, ok := routing.FromContext(ctx)
	if !ok {
		return nil
	}
	operation.TaskID = task.ID
	operation.WorkspaceID = task.WorkspaceID
	operation.ObservedStepID = task.WorkflowStepID
	operation.TargetStepID = targetStepID
	operation.Outcome = routing.OutcomeAlreadySatisfied
	if err := recorder.RecordWorkflowRouteOperation(ctx, operation); err != nil {
		return fmt.Errorf("record already-satisfied transition: %w", err)
	}
	return nil
}

func (s *workflowStore) ApplyDeferredMoveTransition(ctx context.Context, taskID, sessionID, fromStepID, toStepID, moveID string) error {
	ctx, err := s.rehydrateDeferredRouteOperation(ctx, taskID, fromStepID, toStepID, moveID)
	if err != nil {
		return err
	}
	applied, err := s.applyTransitionIfAtStepWithMoveID(
		ctx, taskID, sessionID, fromStepID, toStepID, engine.TriggerOnEnter, moveID, false,
	)
	if err != nil {
		return err
	}
	if !applied {
		return ErrTransitionSourceChanged
	}
	return nil
}

func (s *workflowStore) rehydrateDeferredRouteOperation(
	ctx context.Context,
	taskID, fromStepID, toStepID, moveID string,
) (context.Context, error) {
	if moveID == "" {
		return ctx, nil
	}
	reader, ok := s.repo.(interface {
		GetWorkflowRouteOperation(context.Context, string) (routing.Operation, bool, error)
	})
	if !ok {
		return ctx, nil
	}
	operation, found, err := reader.GetWorkflowRouteOperation(ctx, moveID)
	if err != nil {
		return ctx, fmt.Errorf("load deferred route operation: %w", err)
	}
	if !found {
		return ctx, nil
	}
	if operation.TaskID != taskID || operation.ExpectedStepID != fromStepID || operation.TargetStepID != toStepID {
		return ctx, fmt.Errorf("%w: %s", routing.ErrOperationIdentityConflict, moveID)
	}
	return routing.WithOperation(ctx, operation), nil
}

func (s *workflowStore) MarkDeferredMoveApplied(ctx context.Context, taskID, moveID string) error {
	if moveID == "" {
		return nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task for deferred move identity: %w", err)
	}
	ctx, err = s.rehydrateAlreadySatisfiedRouteOperation(ctx, task, moveID)
	if err != nil {
		return err
	}
	if err := markDeferredMoveApplied(task, moveID); err != nil {
		return err
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("persist deferred move identity: %w", err)
	}
	return nil
}

func (s *workflowStore) rehydrateAlreadySatisfiedRouteOperation(
	ctx context.Context,
	task *models.Task,
	moveID string,
) (context.Context, error) {
	reader, ok := s.repo.(interface {
		GetWorkflowRouteOperation(context.Context, string) (routing.Operation, bool, error)
	})
	if !ok {
		return ctx, nil
	}
	operation, found, err := reader.GetWorkflowRouteOperation(ctx, moveID)
	if err != nil {
		return ctx, fmt.Errorf("load already-satisfied route operation: %w", err)
	}
	if !found {
		return ctx, nil
	}
	if operation.TaskID != task.ID || operation.TargetStepID != task.WorkflowStepID {
		return ctx, fmt.Errorf("%w: %s", routing.ErrOperationIdentityConflict, moveID)
	}
	return routing.WithOperation(ctx, operation), nil
}

func (s *workflowStore) applyTransition(ctx context.Context, taskID, sessionID, fromStepID, toStepID string, trigger engine.Trigger, moveID string) error {
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
	if err := markDeferredMoveApplied(task, moveID); err != nil {
		return err
	}

	oldWorkflowID := task.WorkflowID
	if targetStep != nil {
		task.WorkflowID = targetStep.WorkflowID
	}
	task.WorkflowStepID = toStepID
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.QueuedAt = nil
	if task.Metadata != nil {
		delete(task.Metadata, models.MetaKeyQueuedMoveExitPending)
		delete(task.Metadata, models.MetaKeyQueuedMoveExitCompleted)
		delete(task.Metadata, models.MetaKeyQueuePromotionPending)
	}
	task.UpdatedAt = time.Now().UTC()
	// engine_transition applies only when no outer caller already declared a
	// trigger — applyPendingMove sets mcp_deferred_move before reaching this
	// path, and that must survive rather than be overwritten.
	transitionCtx := ctx
	if !steptelemetry.HasTrigger(transitionCtx) {
		transitionCtx = engineTransitionAttribution(transitionCtx, sessionID, trigger)
	}
	// Only attach a PendingAllocation when the caller already opted in by
	// wrapping ctx with a ResultHolder (applyEngineTransition does this for
	// the engine-driven on_turn_complete path). Attaching it unconditionally
	// would allocate workflow_step_entries rows for every transition into a
	// step with engine-owned on_enter actions, including the callers (manual
	// move, deferred move) that don't yet dispatch through them this Build
	// round — see docs/specs/workflow-on-enter-action-dispatch/spec.md and
	// the task plan's scope note for why those entry paths are deferred.
	if targetStep != nil {
		if _, wantsAllocation := stepentry.ResultHolderFromContext(ctx); wantsAllocation {
			if pending, ok := stepentry.BuildPendingAllocation(targetStep.ID, targetStep.Events.OnEnter); ok {
				transitionCtx = stepentry.WithPendingAllocation(transitionCtx, pending)
			}
		}
	}
	if err := s.updateTransitionTask(transitionCtx, task, targetStep); err != nil {
		return fmt.Errorf("update task workflow step: %w", err)
	}

	// Pass the pre-move workflow ID through so cross-workflow transitions
	// carry old_workflow_id on the task.updated payload — the frontend uses
	// that field to remove the task from its previous workflow's snapshot
	// instead of leaving a stale duplicate until reload.
	s.publishTaskUpdated(ctx, task, oldWorkflowID)

	if task.QueuedForStepID == "" {
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	s.logger.Info("workflow transition applied",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step_id", fromStepID),
		zap.String("to_step_id", toStepID))

	s.pullNextTaskOnVacate(ctx, fromStepID, taskID)

	return nil
}

// ApplyTransitionIfAtStep is the AC-46/48 compare-and-swap variant of
// ApplyTransition used by the workflow engine's guarded-transition
// re-evaluation apply path (Engine.reevaluateGuardedTransitions). The service
// can install a lifecycle bridge so this CAS goes through the same
// orchestrator hooks as other engine transitions. applied=false means the
// task's persisted step no longer equals expectedStepID by the time the
// underlying repository checked — a concurrent transition already moved it —
// which is not an error.
func (s *workflowStore) ApplyTransitionIfAtStep(
	ctx context.Context, taskID, sessionID, expectedStepID, toStepID string, trigger engine.Trigger,
) (bool, error) {
	if s.guardedLifecycle != nil {
		return s.guardedLifecycle(ctx, taskID, sessionID, expectedStepID, toStepID, trigger)
	}
	return s.applyTransitionIfAtStep(ctx, taskID, sessionID, expectedStepID, toStepID, trigger)
}

func (s *workflowStore) applyTransitionIfAtStep(
	ctx context.Context, taskID, sessionID, expectedStepID, toStepID string, trigger engine.Trigger,
) (bool, error) {
	return s.applyTransitionIfAtStepWithMoveID(ctx, taskID, sessionID, expectedStepID, toStepID, trigger, "", false)
}

func (s *workflowStore) applyTransitionIfAtStepAllowMissingTarget(
	ctx context.Context, taskID, sessionID, expectedStepID, toStepID string, trigger engine.Trigger,
) (bool, error) {
	return s.applyTransitionIfAtStepWithMoveID(ctx, taskID, sessionID, expectedStepID, toStepID, trigger, "", true)
}

func (s *workflowStore) applyTransitionIfAtStepWithMoveID(
	ctx context.Context, taskID, sessionID, expectedStepID, toStepID string, trigger engine.Trigger, moveID string,
	allowMissingTarget bool,
) (bool, error) {
	transitionCtx := ctx
	if !steptelemetry.HasTrigger(transitionCtx) {
		transitionCtx = engineTransitionAttribution(transitionCtx, sessionID, trigger)
	}
	if _, ok := routing.FromContext(transitionCtx); !ok {
		operationID := moveID
		producer := routing.ProducerDeferredMove
		if operationID == "" {
			operationID = "workflow:" + uuid.NewString()
			producer = routing.ProducerWorkflow
		}
		attribution := steptelemetry.FromContext(transitionCtx)
		transitionCtx = routing.WithOperation(transitionCtx, routing.Operation{
			ID: operationID, TaskID: taskID, Producer: producer,
			ExpectedStepID: expectedStepID, TargetStepID: toStepID,
			SessionID: sessionID, ActorKind: string(attribution.ActorKind), ActorID: attribution.ActorID,
		})
	}
	task, oldWorkflowID, applied, err := s.applyTransitionIfAtStepRawOptions(
		transitionCtx, taskID, expectedStepID, toStepID, moveID, allowMissingTarget,
	)
	if err != nil {
		return false, err
	}
	if !applied {
		return false, nil
	}

	s.publishTaskUpdated(ctx, task, oldWorkflowID)

	if task.QueuedForStepID == "" {
		if err := s.repo.UpdateSessionReviewStatus(ctx, sessionID, ""); err != nil {
			s.logger.Warn("failed to clear session review status",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}

	s.logger.Info("workflow transition applied (compare-and-swap)",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.String("from_step_id", expectedStepID),
		zap.String("to_step_id", toStepID))

	s.pullNextTaskOnVacate(ctx, expectedStepID, taskID)

	return true, nil
}

// applyTransitionIfAtStepRaw commits the guarded move without publishing
// events or changing session state. The service lifecycle wrapper uses this
// commit point after credential preflight and on_exit, then performs the
// remaining transition lifecycle after the CAS succeeds.
func (s *workflowStore) applyTransitionIfAtStepRaw(
	ctx context.Context, taskID, expectedStepID, toStepID string, moveIDs ...string,
) (*models.Task, string, bool, error) {
	moveID := ""
	if len(moveIDs) > 0 {
		moveID = moveIDs[0]
	}
	return s.applyTransitionIfAtStepRawOptions(ctx, taskID, expectedStepID, toStepID, moveID, false)
}

func (s *workflowStore) applyTransitionIfAtStepRawOptions(
	ctx context.Context, taskID, expectedStepID, toStepID, moveID string, allowMissingTarget bool,
) (*models.Task, string, bool, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, "", false, fmt.Errorf("load task for CAS transition: %w", err)
	}
	ctx = routeObservationContext(ctx, task, toStepID)
	targetStep, err := s.resolveCASTargetStep(ctx, task, toStepID, allowMissingTarget)
	if err != nil {
		return nil, "", false, err
	}
	if err := markDeferredMoveApplied(task, moveID); err != nil {
		return nil, task.WorkflowID, false, err
	}
	casRepo, ok := s.repo.(workflowMoveAdmissionCASRepository)
	if !ok {
		return nil, "", false, fmt.Errorf("workflow step CAS admission repository unavailable for step %s", toStepID)
	}

	oldWorkflowID := task.WorkflowID
	task.WorkflowID = targetStep.WorkflowID
	task.WorkflowStepID = toStepID
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.QueuedAt = nil
	if task.Metadata != nil {
		delete(task.Metadata, models.MetaKeyQueuedMoveExitPending)
		delete(task.Metadata, models.MetaKeyQueuedMoveExitCompleted)
		delete(task.Metadata, models.MetaKeyQueuePromotionPending)
	}
	task.UpdatedAt = time.Now().UTC()
	if _, wantsAllocation := stepentry.ResultHolderFromContext(ctx); wantsAllocation {
		if pending, ok := stepentry.BuildPendingAllocation(targetStep.ID, targetStep.Events.OnEnter); ok {
			ctx = stepentry.WithPendingAllocation(ctx, pending)
		}
	}

	applied, err := casRepo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(
		ctx, task, expectedStepID, toStepID, targetStep.WIPLimit,
	)
	if err != nil {
		return nil, "", false, fmt.Errorf("update task workflow step (CAS): %w", err)
	}
	if !applied {
		if recorder, ok := s.repo.(interface {
			RecordWorkflowRouteOperation(context.Context, routing.Operation) error
		}); ok {
			operation, hasOperation := routing.FromContext(ctx)
			if hasOperation {
				if current, loadErr := s.repo.GetTask(ctx, taskID); loadErr == nil && current != nil {
					operation.ObservedStepID = current.WorkflowStepID
				}
				operation.Outcome = routing.OutcomeStaleSource
				if recordErr := recorder.RecordWorkflowRouteOperation(ctx, operation); recordErr != nil {
					s.logger.Warn("failed to record stale workflow route operation",
						zap.String("task_id", taskID), zap.String("operation_id", operation.ID), zap.Error(recordErr))
				}
			}
		}
		return nil, oldWorkflowID, false, nil
	}
	return task, oldWorkflowID, true, nil
}

func routeObservationContext(ctx context.Context, task *models.Task, targetStepID string) context.Context {
	operation, ok := routing.FromContext(ctx)
	if !ok {
		return ctx
	}
	operation.TaskID = task.ID
	operation.WorkspaceID = task.WorkspaceID
	operation.ObservedStepID = task.WorkflowStepID
	operation.TargetStepID = targetStepID
	return routing.WithOperation(ctx, operation)
}

func (s *workflowStore) resolveCASTargetStep(
	ctx context.Context, task *models.Task, targetStepID string, allowMissing bool,
) (*wfmodels.WorkflowStep, error) {
	targetStep, err := s.workflowStepGetter.GetStep(ctx, targetStepID)
	if err != nil {
		return nil, fmt.Errorf("load target step for CAS transition: %w", err)
	}
	if targetStep != nil {
		return targetStep, nil
	}
	if !allowMissing {
		return nil, fmt.Errorf("target step %s not found for CAS transition", targetStepID)
	}
	// ApplyTransition historically accepted test/adapter getters that did not
	// materialize the target step. Preserve that compatibility while still
	// performing a source CAS; guarded/engine paths continue to fail closed.
	return &wfmodels.WorkflowStep{ID: targetStepID, WorkflowID: task.WorkflowID}, nil
}

func markDeferredMoveApplied(task *models.Task, moveID string) error {
	if moveID == "" {
		return nil
	}
	applied, _ := task.Metadata[models.MetaKeyAppliedDeferredMoves].(map[string]interface{})
	if _, exists := applied[moveID]; exists {
		return errDeferredMoveAlreadyApplied
	}
	if applied == nil {
		applied = make(map[string]interface{})
	}
	applied[moveID] = true
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	task.Metadata[models.MetaKeyAppliedDeferredMoves] = applied
	return nil
}

func (s *workflowStore) updateTransitionTask(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) error {
	if targetStep == nil {
		return s.repo.UpdateTask(ctx, task)
	}
	admissionRepo, ok := s.repo.(workflowMoveAdmissionRepository)
	if !ok {
		return fmt.Errorf("workflow step admission repository unavailable for step %s", targetStep.ID)
	}
	_, err := admissionRepo.UpdateTaskWithWorkflowStepAdmission(ctx, task, targetStep.ID, targetStep.WIPLimit)
	return err
}

func (s *workflowStore) pullNextTaskOnVacate(ctx context.Context, vacatedStepID, excludeTaskID string) {
	// A queue/WIP reconciliation is always wip_pull, unconditionally
	// overriding whatever trigger the caller that vacated the step declared
	// (or absent, for the ReconcileQueuedTasks restart sweep) — the vacating
	// move and the resulting pull are two distinct ledger rows with two
	// distinct causes, and no single session initiates a pull.
	ctx = steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerWIPPull,
		ActorKind: steptelemetry.ActorSystem,
	})
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
		if queuedMoveExitPending(candidate) {
			skipped[candidate.ID] = struct{}{}
			continue
		}
		if s.feederCandidateBlocked(ctx, candidate.ID) {
			skipped[candidate.ID] = struct{}{}
			continue
		}
		fromWorkflowID := candidate.WorkflowID
		fromStepID := candidate.WorkflowStepID
		candidate.WorkflowID = vacatedStep.WorkflowID
		candidate.WorkflowStepID = vacatedStep.ID
		if candidate.Metadata == nil {
			candidate.Metadata = make(map[string]interface{})
		}
		candidate.Metadata[models.MetaKeyQueuePromotionPending] = true
		candidate.Position = position
		oldState, stateChanged, err := s.syncQueuedPromotionState(ctx, candidate, vacatedStep)
		if err != nil {
			s.logger.Warn("skipping feeder task: failed to prepare promotion state",
				zap.String("task_id", candidate.ID), zap.Error(err))
			skipped[candidate.ID] = struct{}{}
			continue
		}
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
		} else if admissionRepo, ok := s.repo.(workflowMoveAdmissionRepository); ok {
			claimed, err := admissionRepo.UpdateTaskWithWorkflowStepAdmission(ctx, candidate, vacatedStep.ID, vacatedStep.WIPLimit)
			if err != nil {
				s.logger.Warn("failed to promote feeder task", zap.String("task_id", candidate.ID), zap.Error(err))
				skipped[candidate.ID] = struct{}{}
				continue
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
		if stateChanged && s.publishStateChanged != nil {
			s.publishStateChanged(ctx, candidate, oldState)
		}
		sessionID := ""
		if session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, candidate.ID); err == nil && session != nil {
			sessionID = session.ID
		}
		if s.stepHistoryRecorder != nil && sessionID != "" {
			if asyncRecorder, ok := s.stepHistoryRecorder.(asyncStepHistoryRecorder); ok {
				asyncRecorder.EnqueueStepTransition(sessionID, fromStepID, vacatedStep.ID, wfmodels.StepTransitionTriggerQueuePromotion, nil, nil)
			} else if err := s.stepHistoryRecorder.CreateStepTransition(ctx, sessionID, fromStepID, vacatedStep.ID, wfmodels.StepTransitionTriggerQueuePromotion, nil, nil); err != nil {
				s.logger.Warn("failed to record queue promotion transition", zap.String("task_id", candidate.ID), zap.Error(err))
			}
		}
		s.publishTaskMoved(ctx, candidate, fromWorkflowID, fromStepID, vacatedStep.ID, sessionID)
		return true
	}
}

func (s *workflowStore) promoteSameStepTask(ctx context.Context, candidate *models.Task, step *wfmodels.WorkflowStep, position int, skipped map[string]struct{}, pullRepo workflowPullRepository, limitedRepo workflowLimitedMoveRepository) bool {
	fromStepID := candidate.WorkflowStepID
	if candidate.Metadata == nil {
		candidate.Metadata = make(map[string]interface{})
	}
	candidate.WIPAdmitted = true
	candidate.QueuedForStepID = ""
	candidate.QueuedAt = nil
	candidate.Metadata[models.MetaKeyQueuePromotionPending] = true
	candidate.Position = position
	candidate.UpdatedAt = time.Now().UTC()
	oldState, stateChanged, err := s.syncQueuedPromotionState(ctx, candidate, step)
	if err != nil {
		s.logger.Warn("skipping queued task: failed to prepare promotion state",
			zap.String("task_id", candidate.ID), zap.Error(err))
		skipped[candidate.ID] = struct{}{}
		return s.pullOneFeederTask(ctx, pullRepo, limitedRepo, step, position, skipped)
	}
	if promoter, ok := s.repo.(workflowQueuedTaskPromoter); ok {
		claimed, err := promoter.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, candidate, fromStepID, step.ID, step.WIPLimit)
		if err != nil {
			s.logger.Warn("failed to promote same-step queued task", zap.String("task_id", candidate.ID), zap.Error(err))
			return false
		}
		if !claimed {
			skipped[candidate.ID] = struct{}{}
			return s.pullOneFeederTask(ctx, pullRepo, limitedRepo, step, position, skipped)
		}
	} else if admissionRepo, ok := s.repo.(workflowMoveAdmissionRepository); ok {
		claimed, err := admissionRepo.UpdateTaskWithWorkflowStepAdmission(ctx, candidate, step.ID, step.WIPLimit)
		if err != nil {
			s.logger.Warn("failed to promote same-step queued task", zap.String("task_id", candidate.ID), zap.Error(err))
			skipped[candidate.ID] = struct{}{}
			return s.pullOneFeederTask(ctx, pullRepo, limitedRepo, step, position, skipped)
		}
		if !claimed {
			skipped[candidate.ID] = struct{}{}
			return s.pullOneFeederTask(ctx, pullRepo, limitedRepo, step, position, skipped)
		}
	} else if err := limitedRepo.UpdateTaskIfWorkflowStepHasCapacity(ctx, candidate, step.ID, candidate.ID, step.WIPLimit); err != nil {
		s.logger.Warn("failed to promote same-step queued task", zap.String("task_id", candidate.ID), zap.Error(err))
		skipped[candidate.ID] = struct{}{}
		return s.pullOneFeederTask(ctx, pullRepo, limitedRepo, step, position, skipped)
	}
	s.publishTaskUpdated(ctx, candidate)
	if stateChanged && s.publishStateChanged != nil {
		s.publishStateChanged(ctx, candidate, oldState)
	}
	if s.publishTaskPromoted != nil {
		s.publishTaskPromoted(ctx, candidate)
	}
	return true
}

func (s *workflowStore) syncQueuedPromotionState(ctx context.Context, task *models.Task, targetStep *wfmodels.WorkflowStep) (v1.TaskState, bool, error) {
	if targetStep == nil || s.workflowStepGetter == nil {
		return task.State, false, nil
	}
	next, err := s.workflowStepGetter.GetNextStepByPosition(ctx, targetStep.WorkflowID, targetStep.Position)
	if err != nil {
		return task.State, false, fmt.Errorf("load next step after %s: %w", targetStep.ID, err)
	}
	oldState := task.State
	if wfmodels.IsTerminalStep(targetStep, next) {
		if !models.IsTerminalTaskState(task.State) {
			task.State = v1.TaskStateCompleted
		}
		return oldState, oldState != task.State, nil
	}
	if task.State == v1.TaskStateCompleted {
		task.State = v1.TaskStateTODO
	}
	return oldState, oldState != task.State, nil
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
		if queuedMoveExitPending(candidate) {
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
