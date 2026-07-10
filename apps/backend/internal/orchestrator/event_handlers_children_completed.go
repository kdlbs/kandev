package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

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

	appliedTransition := s.applyEngineTransition(
		ctx,
		parentID,
		session,
		result,
		engine.TriggerOnChildrenCompleted,
		parent.Description,
		true,
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
