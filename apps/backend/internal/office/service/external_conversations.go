package service

import (
	"context"
	"github.com/kandev/kandev/internal/events/bus"
)

// Registered conversations are exclusively owned by the standalone runtime.
func (s *Service) skipExternalConversation(handler bus.EventHandler) bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		external, err := s.isExternalConversation(ctx, event)
		if err != nil {
			return err
		}
		if external {
			return nil
		}
		return handler(ctx, event)
	}
}
func (s *Service) isExternalConversation(ctx context.Context, event *bus.Event) (bool, error) {
	if !s.externalOrchestration {
		return false, nil
	}
	data, err := decodeEventData[TaskUpdatedData](event)
	if err != nil || data.TaskID == "" {
		return false, nil
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		return false, nil
	}
	role, err := s.repo.OrchestratorRoleID(ctx, fields.AssigneeAgentProfileID)
	return role != "", err
}
