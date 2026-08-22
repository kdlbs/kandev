package handlers

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

// enrichTasksWithPendingActions populates dto.TaskDTO.TaskPendingAction and
// dto.TaskDTO.PrimarySessionPendingAction for each task, using the same
// GetPendingActionsForSessions projection and derivation the HTTP task list
// already applies (task/dto/dto.go's TaskPendingAction /
// PrimarySessionPendingAction fields), so an MCP list_tasks_kandev call finds
// every blocked task in a workflow without a follow-up per-task lookup.
// Both fields stay JSON null for a task with no blocked session, matching the
// HTTP DTO (*string, no omitempty).
//
// dtos must be the same length and order as tasks — the caller builds both
// from one loop over the same task slice.
func (h *Handlers) enrichTasksWithPendingActions(ctx context.Context, tasks []*models.Task, dtos []dto.TaskDTO) {
	if len(tasks) == 0 {
		return
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	sessionsByTask, err := h.taskSvc.BatchGetSessionsForTasks(ctx, taskIDs)
	if err != nil {
		h.logger.Warn("failed to load sessions for list_tasks_kandev pending actions", zap.Error(err))
		return
	}
	actionsBySession, err := pendingActionsForInputCapableSessions(ctx, h.taskSvc, sessionsByTask)
	if err != nil {
		h.logger.Warn("failed to load pending actions for list_tasks_kandev", zap.Error(err))
		return
	}
	for i, t := range tasks {
		sessions := sessionsByTask[t.ID]
		dtos[i].TaskPendingAction = dto.TaskPendingActionPtr(sessions, actionsBySession)
		dtos[i].PrimarySessionPendingAction = dto.PendingActionPtr(primarySessionID(sessions), actionsBySession)
	}
}

// pendingActionsForInputCapableSessions collects the pending-action
// projection for every input-capable session across the given per-task
// session groups in one batched call, mirroring
// internal/task/handlers.pendingActionsForInputCapableSessions.
func pendingActionsForInputCapableSessions(
	ctx context.Context,
	svc pendingActionsForSessionsGetter,
	sessionsByTask map[string][]*models.TaskSession,
) (map[string]models.TaskPendingAction, error) {
	sessionIDs := make([]string, 0)
	for _, sessions := range sessionsByTask {
		for _, session := range sessions {
			if dto.IsInputCapableSession(session) {
				sessionIDs = append(sessionIDs, session.ID)
			}
		}
	}
	if len(sessionIDs) == 0 {
		return map[string]models.TaskPendingAction{}, nil
	}
	return svc.GetPendingActionsForSessions(ctx, sessionIDs)
}

// pendingActionsForSessionsGetter is the narrow slice of *service.Service
// this file depends on, so it can be exercised with a fake in tests without
// standing up a full service.
type pendingActionsForSessionsGetter interface {
	GetPendingActionsForSessions(ctx context.Context, sessionIDs []string) (map[string]models.TaskPendingAction, error)
}

// primarySessionID returns the ID of the primary session in sessions, or nil
// if none is marked primary.
func primarySessionID(sessions []*models.TaskSession) *string {
	for _, s := range sessions {
		if s.IsPrimary {
			id := s.ID
			return &id
		}
	}
	return nil
}
