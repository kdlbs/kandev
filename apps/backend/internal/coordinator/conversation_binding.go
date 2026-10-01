package coordinator

import (
	"context"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// stampToolPolicy writes the conversation's tool binding into metadata. With
// phase 2 off nothing is written, so the conversation keeps the phase-1 seven.
func (s *Service) stampToolPolicy(metadata map[string]interface{}, c *Coordinator, conversationTaskID string) error {
	if !s.phase2 {
		return nil
	}
	encoded, err := mcpprofile.MarshalCoordinatorToolPolicy(mcpprofile.CoordinatorToolPolicy{
		Version:            1,
		CoordinatorID:      c.ID,
		WorkspaceID:        c.WorkspaceID,
		ConversationTaskID: conversationTaskID,
		PolicyRevision:     c.PolicyRevision,
		ToolNames:          ToolNamesPhase31(s.policyFor(c), true, s.phase3, s.phase31),
	})
	if err != nil {
		return err
	}
	metadata[mcpprofile.CoordinatorToolPolicyMetadataKey] = encoded
	return nil
}

// bindConversationTask fills the created task's own id into its binding: the
// id does not exist until the task is created, and a binding with no
// conversation task id is invalid at execution time.
func (s *Service) bindConversationTask(ctx context.Context, task *taskmodels.Task, c *Coordinator, metadata map[string]interface{}) error {
	if !s.phase2 {
		return nil
	}
	if err := s.stampToolPolicy(metadata, c, task.ID); err != nil {
		return err
	}
	_, err := s.conversationTasks.UpdateTask(ctx, task.ID, &taskservice.UpdateTaskRequest{
		Metadata:              metadata,
		AllowReservedMetadata: true,
	})
	return err
}
