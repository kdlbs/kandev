package service

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/events/bus"
)

// Delegation observes existing board tasks without taking over their workflow engine.
func (s *Service) handleWorkspaceTaskEvent(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskUpdatedData](event)
	if err != nil || data.TaskID == "" {
		return nil
	}
	workspaceID, chiefID, err := s.repo.WorkspaceTaskOwner(ctx, data.TaskID)
	if err != nil || chiefID == "" {
		return nil
	}
	role, err := s.repo.OrchestratorRoleID(ctx, chiefID)
	if err != nil {
		return err
	}
	if !s.isWorkspaceTaskCoordinator(ctx, workspaceID, chiefID, role) {
		return nil
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		return err
	}
	switch fields.State {
	case "REVIEW", "COMPLETED", "DONE", "FAILED", "WAITING_FOR_INPUT", "BLOCKED":
	default:
		return nil
	}
	chief, err := s.repo.GetAgentInstance(ctx, chiefID)
	if err != nil {
		return err
	}
	if chief.WorkspaceID != workspaceID || fields.IsFromOffice {
		return nil
	}
	channel, err := s.repo.EnsureAgentConversation(ctx, chief)
	if err != nil {
		return err
	}
	if role != "" {
		return s.queueWorkspaceTaskCallback(ctx, chiefID, channel.TaskID, fields, *data)
	}
	payload := mustJSON(map[string]any{conversationTaskIDKey: channel.TaskID, "children": []map[string]string{{"identifier": data.TaskID, "state": fields.State}}})
	return s.QueueRun(ctx, chiefID, RunReasonTaskChildrenCompleted, payload, fmt.Sprintf("workspace-task:%s:%s", data.TaskID, event.ID))
}

// Registered orchestrators own their delegated tasks; legacy Office uses one selected chief.
func (s *Service) isWorkspaceTaskCoordinator(ctx context.Context, workspaceID, chiefID, role string) bool {
	if s.externalOrchestration && role != "" {
		return false
	}

	if role != "" {
		return true
	}
	selected, err := s.repo.GetWorkspaceChief(ctx, workspaceID)
	return err == nil && selected == chiefID
}
