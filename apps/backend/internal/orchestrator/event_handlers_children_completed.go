package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (s *Service) handleTaskStateChanged(ctx context.Context, data watcher.TaskEventData) {
	if data.NewState == nil || !models.IsTerminalTaskState(*data.NewState) {
		return
	}

	s.processParentChildrenCompletedForTaskState(ctx, data.TaskID, *data.NewState)
	// Peer dependencies react to the same terminal transitions, but with a
	// stricter definition of "done": on_children_completed counts FAILED as
	// terminal, dependency resolution requires success. Keep the two separate.
	s.handleTaskDependenciesForTerminalState(ctx, data.TaskID, *data.NewState)
}

func (s *Service) processParentChildrenCompletedForTaskState(ctx context.Context, taskID string, state v1.TaskState) {
	if !models.IsTerminalTaskState(state) {
		return
	}

	child, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.logger.Warn("on_children_completed: failed to load changed child task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if child.ParentID == "" {
		return
	}

	s.processOnChildrenCompleted(ctx, child.ParentID)
}

func (s *Service) processParentChildrenCompletedForTerminalStepMove(ctx context.Context, taskID, stepID string) {
	if !s.workflowStepIsTerminal(ctx, stepID) {
		return
	}
	s.markTaskCompletedForTerminalStep(ctx, taskID, stepID)
}

func (s *Service) markTaskCompletedForTerminalStep(ctx context.Context, taskID, terminalStepID string) {
	s.taskRuntimeStateMu.Lock()
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		s.taskRuntimeStateMu.Unlock()
		s.logger.Warn("terminal step completion: failed to load task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if terminalStepID != "" && task.WorkflowStepID != terminalStepID {
		s.taskRuntimeStateMu.Unlock()
		return
	}
	if models.IsTerminalTaskState(task.State) {
		s.taskRuntimeStateMu.Unlock()
		return
	}
	if task.IsFromOffice {
		s.taskRuntimeStateMu.Unlock()
		s.markOfficeTaskCompletedForTerminalStep(ctx, taskID)
		return
	}
	oldState := task.State
	task.State = v1.TaskStateCompleted
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		s.taskRuntimeStateMu.Unlock()
		s.logger.Warn("terminal step completion: failed to mark task completed",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	s.taskRuntimeStateMu.Unlock()
	s.publishTaskUpdated(ctx, task)
	s.publishTaskStateChanged(ctx, task, oldState)
	s.processParentChildrenCompletedForTaskState(ctx, taskID, v1.TaskStateCompleted)
}

// markOfficeTaskCompletedForTerminalStep routes an Office task's terminal
// step completion through Office's own status pipeline (UpdateTaskStatus)
// instead of a raw state write, so the approval gate runs (tasks-01.md:76).
// UpdateTaskStatus persists a redirect to in_review and then returns a
// typed *dashboard.ApprovalsPendingError when the gate fires — the write
// has already succeeded, so that return is not a failure and is not
// logged as one, and no completion side-effects fire below: the redirect
// is not a completion.
//
// A nil error means the gate did not redirect, so the seam persisted
// "done" (state = COMPLETED). That path must still carry the same
// side-effects the raw completion write does — publishing task.updated /
// task.state_changed and driving the parent's on_children_completed
// trigger — or dependency resolution, the parent workflow transition, and
// every task.updated subscriber silently stop seeing Office completions.
//
// The caller's taskRuntimeStateMu check-then-act is not atomic across this
// call (the seam runs Office's reactivity pipeline and publishes, so
// holding that global lock across it risks lock inversion). Two
// deliveries for the same task can therefore both pass the caller's check
// before either has written. lockOfficeTerminalCompletion serializes by
// task ID instead, and the task is re-read and re-checked once inside
// that lock: whichever delivery gets there first does the write and fires
// the side effects below; a second, now-redundant delivery finds the task
// already terminal and returns without calling the seam a second time.
//
// The lock is released as soon as the write (or redirect) is durable and
// before any of the side effects below run, for the same reason the caller
// releases taskRuntimeStateMu early: processParentChildrenCompletedForTaskState
// walks up into the parent's own on_children_completed handling, which can
// itself reach this same lock family (for an Office parent) or
// lockChildCompletionOperation. Holding this task's lock across that call
// would invert lock order against a concurrent completion elsewhere in the
// same ancestry and risk deadlock.
func (s *Service) markOfficeTaskCompletedForTerminalStep(ctx context.Context, taskID string) {
	if s.officeTaskStatusUpdater == nil {
		// No seam wired (partial wiring, or a test): skip the write rather
		// than falling through to the raw path, which would bypass the gate.
		return
	}

	unlock := s.lockOfficeTerminalCompletion(taskID)

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		unlock()
		s.logger.Warn("terminal step completion: failed to reload office task",
			zap.String("task_id", taskID),
			zap.Error(err))
		return
	}
	if models.IsTerminalTaskState(task.State) {
		unlock()
		return
	}
	oldState := task.State

	err = s.officeTaskStatusUpdater.UpdateTaskStatus(ctx, dashboard.TaskStatusUpdateRequest{
		TaskID:    task.ID,
		NewStatus: "done",
	})
	var pending *dashboard.ApprovalsPendingError
	if err != nil {
		unlock()
		if !errors.As(err, &pending) {
			s.logger.Warn("terminal step completion: office status update failed",
				zap.String("task_id", task.ID),
				zap.Error(err))
		}
		return
	}
	unlock()

	task.State = v1.TaskStateCompleted
	task.UpdatedAt = time.Now().UTC()
	s.publishTaskUpdated(ctx, task)
	s.publishTaskStateChanged(ctx, task, oldState)
	s.processParentChildrenCompletedForTaskState(ctx, task.ID, v1.TaskStateCompleted)
}

// lockOfficeTerminalCompletion serializes markOfficeTaskCompletedForTerminalStep
// calls for the same task ID, mirroring lockChildCompletionOperation.
func (s *Service) lockOfficeTerminalCompletion(taskID string) func() {
	s.officeTerminalCompletionLocksMu.Lock()
	if s.officeTerminalCompletionLocks == nil {
		s.officeTerminalCompletionLocks = make(map[string]*childCompletionOperationLock)
	}
	entry := s.officeTerminalCompletionLocks[taskID]
	if entry == nil {
		entry = &childCompletionOperationLock{}
		s.officeTerminalCompletionLocks[taskID] = entry
	}
	entry.refs++
	s.officeTerminalCompletionLocksMu.Unlock()

	entry.mu.Lock()
	return func() {
		s.officeTerminalCompletionLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.officeTerminalCompletionLocks, taskID)
		}
		s.officeTerminalCompletionLocksMu.Unlock()
		entry.mu.Unlock()
	}
}

func (s *Service) processOnChildrenCompleted(ctx context.Context, parentID string) bool {
	if parentID == "" || s.workflowEngine == nil || s.workflowStore == nil || s.workflowStepGetter == nil {
		return false
	}

	rows, ok := s.readyChildCompletionRows(ctx, parentID)
	if !ok {
		return false
	}

	parent, session, ok := s.parentCompletionContext(ctx, parentID)
	if !ok {
		return false
	}

	operationID := childCompletionOperationID(parentID, rows)
	unlock := s.lockChildCompletionOperation(operationID)
	defer unlock()

	if s.childCompletionAlreadyApplied(ctx, parentID, operationID) {
		return false
	}

	result, ok := s.evaluateChildrenCompleted(ctx, parent, session, rows)
	if !ok {
		return false
	}

	if !result.Transitioned {
		s.markChildCompletionApplied(ctx, parentID, operationID, "non-transition")
		return false
	}

	appliedTransition := s.applyEngineTransitionWithMode(
		ctx,
		parentID,
		session,
		result,
		engine.TriggerOnChildrenCompleted,
		parent.Description,
		transitionLifecycleWithOnEnter,
	)
	if !appliedTransition {
		return false
	}
	s.markChildCompletionApplied(ctx, parentID, operationID, "transition")
	return true
}

func (s *Service) readyChildCompletionRows(ctx context.Context, parentID string) ([]models.ChildCompletionRow, bool) {
	rows, err := s.repo.ListChildCompletionRows(ctx, parentID)
	if err != nil {
		s.logger.Warn("on_children_completed: failed to list active children",
			zap.String("parent_task_id", parentID),
			zap.Error(err))
		return nil, false
	}
	s.annotateTerminalChildSteps(ctx, rows)
	if len(rows) == 0 || !allChildrenTerminal(rows) {
		return nil, false
	}
	return rows, true
}

func (s *Service) parentCompletionContext(ctx context.Context, parentID string) (*models.Task, *models.TaskSession, bool) {
	parent, err := s.repo.GetTask(ctx, parentID)
	if err != nil {
		s.logger.Warn("on_children_completed: failed to load parent task",
			zap.String("parent_task_id", parentID),
			zap.Error(err))
		return nil, nil, false
	}
	if parent.WorkflowStepID == "" || parent.IsEphemeral {
		return nil, nil, false
	}

	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, parentID)
	if err != nil {
		s.logger.Debug("on_children_completed: parent has no active session",
			zap.String("parent_task_id", parentID),
			zap.Error(err))
		return nil, nil, false
	}
	return parent, session, true
}

func (s *Service) childCompletionAlreadyApplied(ctx context.Context, parentID, operationID string) bool {
	applied, err := s.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil {
		s.logger.Warn("on_children_completed: failed to check operation idempotency",
			zap.String("parent_task_id", parentID),
			zap.String("operation_id", operationID),
			zap.Error(err))
		return true
	}
	return applied
}

type childCompletionOperationLock struct {
	mu   sync.Mutex
	refs int
}

func (s *Service) lockChildCompletionOperation(operationID string) func() {
	s.childCompletionLocksMu.Lock()
	if s.childCompletionLocks == nil {
		s.childCompletionLocks = make(map[string]*childCompletionOperationLock)
	}
	entry := s.childCompletionLocks[operationID]
	if entry == nil {
		entry = &childCompletionOperationLock{}
		s.childCompletionLocks[operationID] = entry
	}
	entry.refs++
	s.childCompletionLocksMu.Unlock()

	entry.mu.Lock()
	return func() {
		s.childCompletionLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.childCompletionLocks, operationID)
		}
		s.childCompletionLocksMu.Unlock()
		entry.mu.Unlock()
	}
}

func (s *Service) evaluateChildrenCompleted(
	ctx context.Context,
	parent *models.Task,
	session *models.TaskSession,
	rows []models.ChildCompletionRow,
) (engine.HandleResult, bool) {
	state := s.buildMachineState(ctx, parent, session)
	result, err := s.workflowEngine.HandleTrigger(ctx, engine.HandleInput{
		TaskID:         parent.ID,
		SessionID:      session.ID,
		Trigger:        engine.TriggerOnChildrenCompleted,
		EvaluateOnly:   true,
		PreloadedState: &state,
		Payload:        childCompletionPayload(rows),
	})
	if err != nil {
		s.logger.Warn("on_children_completed: workflow engine error",
			zap.String("parent_task_id", parent.ID),
			zap.String("session_id", session.ID),
			zap.Error(err))
		return engine.HandleResult{}, false
	}
	return result, true
}

func (s *Service) markChildCompletionApplied(ctx context.Context, parentID, operationID, phase string) {
	if err := s.workflowStore.MarkOperationApplied(ctx, operationID); err != nil {
		s.logger.Warn("on_children_completed: failed to mark operation",
			zap.String("parent_task_id", parentID),
			zap.String("operation_id", operationID),
			zap.String("phase", phase),
			zap.Error(err))
	}
}

func allChildrenTerminal(rows []models.ChildCompletionRow) bool {
	for _, row := range rows {
		if !models.IsTerminalTaskState(row.State) && !row.TerminalWorkflowStep {
			return false
		}
	}
	return true
}

func (s *Service) annotateTerminalChildSteps(ctx context.Context, rows []models.ChildCompletionRow) {
	if s.workflowStepGetter == nil {
		return
	}
	cache := make(map[string]bool)
	for i := range rows {
		if models.IsTerminalTaskState(rows[i].State) || rows[i].WorkflowStepID == "" {
			continue
		}
		terminal, ok := cache[rows[i].WorkflowStepID]
		if !ok {
			terminal = s.workflowStepIsTerminal(ctx, rows[i].WorkflowStepID)
			cache[rows[i].WorkflowStepID] = terminal
		}
		rows[i].TerminalWorkflowStep = terminal
	}
}

func (s *Service) workflowStepIsTerminal(ctx context.Context, workflowStepID string) bool {
	if s.workflowStepGetter == nil || workflowStepID == "" {
		return false
	}
	step, err := s.workflowStepGetter.GetStep(ctx, workflowStepID)
	if err != nil || step == nil {
		return false
	}
	nextStep, err := s.workflowStepGetter.GetNextStepByPosition(ctx, step.WorkflowID, step.Position)
	if err != nil {
		s.logger.Warn("failed to get next workflow step for terminal check",
			zap.String("workflow_step_id", workflowStepID),
			zap.Error(err))
		return false
	}
	return wfmodels.IsTerminalStep(step, nextStep)
}

func childCompletionPayload(rows []models.ChildCompletionRow) engine.OnChildrenCompletedPayload {
	summaries := make([]engine.ChildSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, engine.ChildSummary{
			TaskID:  row.ID,
			Status:  childCompletionStatus(row),
			Summary: row.Title,
		})
	}
	return engine.OnChildrenCompletedPayload{ChildSummaries: summaries}
}

func childCompletionStatus(row models.ChildCompletionRow) string {
	if row.TerminalWorkflowStep && !models.IsTerminalTaskState(row.State) {
		return string(v1.TaskStateCompleted)
	}
	return string(row.State)
}

func childCompletionOperationID(parentID string, rows []models.ChildCompletionRow) string {
	var b strings.Builder
	b.WriteString(parentID)
	for _, row := range rows {
		b.WriteString("|")
		b.WriteString(row.ID)
		b.WriteString(":")
		b.WriteString(string(row.State))
		b.WriteString(":")
		b.WriteString(row.WorkflowStepID)
		b.WriteString(":")
		if row.TerminalWorkflowStep {
			b.WriteString("terminal")
		} else {
			b.WriteString("active")
		}
		b.WriteString(":")
		b.WriteString(row.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("on_children_completed:%s:%s", parentID, hex.EncodeToString(sum[:]))
}
