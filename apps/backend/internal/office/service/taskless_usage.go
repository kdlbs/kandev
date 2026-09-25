package service

import (
	"context"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// Usage can arrive after completion, but must still identify the exact durable
// attempt. Read attribution from its owner rather than mutable agent state.
func (s *Service) tasklessUsageFields(ctx context.Context, data *PromptUsageData) (*sqlite.TaskExecutionFields, error) {
	session, err := s.repo.GetRunSession(ctx, data.RunSessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.ExecutionID == "" || session.ExecutionID != data.AgentExecutionID ||
		session.Attempt != data.RunAttempt || session.AgentProfileID != data.AgentProfileID ||
		session.WorkspaceID != data.WorkspaceID {
		return nil, nil
	}
	data.SessionID = session.ID
	return &sqlite.TaskExecutionFields{WorkspaceID: session.WorkspaceID, AssigneeAgentProfileID: session.AgentProfileID}, nil
}
