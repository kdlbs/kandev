package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
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
		if !models.IsTerminalTaskState(row.State) {
			return false
		}
	}
	return true
}

func childCompletionPayload(rows []models.ChildCompletionRow) engine.OnChildrenCompletedPayload {
	summaries := make([]engine.ChildSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, engine.ChildSummary{
			TaskID:  row.ID,
			Status:  string(row.State),
			Summary: row.Title,
		})
	}
	return engine.OnChildrenCompletedPayload{ChildSummaries: summaries}
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
		b.WriteString(row.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("on_children_completed:%s:%s", parentID, hex.EncodeToString(sum[:]))
}
