package runtime

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (s *Service) Validate(ctx context.Context, workspace, id string) error {
	owner, err := s.Repo.PersonaUserOwner(ctx, id)
	if err != nil {
		return err
	}
	// Core automations currently retain no human-owner grant. Recheck here at
	// both configuration and delivery; a previously shared target can be claimed.
	if owner != "" {
		return fmt.Errorf("private assistant automations require durable owner authorization")
	}
	role, err := s.Repo.OrchestratorRoleID(ctx, id)
	if err != nil || role == "" {
		return fmt.Errorf("orchestrator is unavailable")
	}
	a, err := s.Personas.GetAgentInstance(ctx, id)
	if err != nil || a.WorkspaceID != workspace {
		return fmt.Errorf("orchestrator must belong to this workspace")
	}
	_, _, err = s.executionSelection(ctx, a)
	return err
}
func (s *Service) Send(ctx context.Context, workspace, id, deliveryID, prompt string) (string, error) {
	if err := s.Validate(ctx, workspace, id); err != nil {
		return "", err
	}
	a, err := s.Personas.GetAgentInstance(ctx, id)
	if err != nil {
		return "", err
	}
	if paused(a) {
		return "", fmt.Errorf("orchestrator is paused or disabled")
	}
	if len(prompt) > 32000 {
		return "", fmt.Errorf("expanded automation prompt exceeds 32000 bytes")
	}
	conversation, err := s.Repo.EnsureAgentConversation(ctx, a)
	if err != nil {
		return "", err
	}
	comment := &models.TaskComment{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("automation:"+deliveryID)).String(), TaskID: conversation.TaskID, AuthorType: authorTypeUser, AuthorID: "automation", Source: "automation", Body: prompt}
	if err := s.Repo.PutComment(ctx, comment); err != nil {
		return "", err
	}
	err = s.QueueTurn(ctx, id, conversation.TaskID, "automation", "automation:"+deliveryID, map[string]any{"comment_id": comment.ID})
	return conversation.TaskID, err
}
