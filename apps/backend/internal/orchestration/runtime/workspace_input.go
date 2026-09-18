package runtime

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) attentionTargetMatches(c *gin.Context, b *models.AssistantBinding, row *models.Attention) bool {
	if _, agent := c.Get("agent_claims"); !agent {
		return true
	}
	workspace := b.WorkspaceID
	if value, linked := c.Get(workspaceSelectionKey); linked {
		workspace = value.(workspaceSelection).Grant.WorkspaceID
	}
	return workspace == row.WorkspaceID
}

func (s *Service) inputWorkspace(ctx context.Context, b *models.AssistantBinding, row *models.Attention, operation string) (*models.AssistantBinding, *models.WorkspaceGrant, error) {
	if row.WorkspaceID == b.WorkspaceID {
		return b, nil, nil
	}
	g, err := s.Repo.WorkspaceGrant(ctx, b.ID, row.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	g, err = s.currentWorkspaceGrant(ctx, b, row.WorkspaceID, g.Revision, operation, "task_input")
	if err != nil {
		return nil, nil, err
	}
	scoped := *b
	scoped.WorkspaceID = row.WorkspaceID
	return &scoped, g, nil
}

func (h *Handler) readScopedInput(c *gin.Context, b *models.AssistantBinding, row *models.Attention) (*models.AttentionInput, error) {
	scoped, _, err := h.Service.inputWorkspace(c.Request.Context(), b, row, workspaceObserve)
	if err != nil {
		return nil, err
	}
	return h.Service.Inputs.ReadInput(c.Request.Context(), scoped, *row)
}
