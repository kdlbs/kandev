package runtime

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
	"net/url"
	"strconv"
)

func (h *Handler) contextObjectiveTarget(c *gin.Context, b *models.AssistantBinding, workspace string) bool {
	o, err := h.Service.Repo.Objective(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil {
		c.AbortWithStatus(422)
		return false
	}
	if o.WorkspaceID != workspace {
		c.AbortWithStatus(403)
		return false
	}
	return true
}

func (s *Service) objectiveWorkspace(ctx context.Context, b *models.AssistantBinding, o *models.Objective) (int64, error) {
	if o.WorkspaceID == b.WorkspaceID {
		return 0, nil
	}
	g, err := s.Repo.WorkspaceGrant(ctx, b.ID, o.WorkspaceID)
	if err != nil {
		return 0, err
	}
	g, err = s.currentWorkspaceGrant(ctx, b, o.WorkspaceID, g.Revision, workspaceCoordinate, "handoff")
	if err != nil {
		return 0, err
	}
	return g.Revision, nil
}

func (s *Service) fillWorkspaceContext(ctx context.Context, b *models.AssistantBinding, p *models.ContextPacket, rows []*models.AgentMemory) error {
	if p.WorkspaceID == b.WorkspaceID {
		if err := s.contextCredentials(ctx, b, p); err != nil {
			return err
		}
		return fillContextMemory(p, b, rows)
	}
	// Only explicitly user-wide preferences accompany a foreign handoff. Home
	// workspace memory and account-bound credential descriptors never migrate.
	shared := []*models.AgentMemory{}
	for _, row := range rows {
		if row.Scope == authorTypeUser {
			shared = append(shared, row)
		}
	}
	if err := fillContextMemory(p, b, shared); err != nil {
		return err
	}
	p.MemoryReference += "&workspace_id=" + url.QueryEscape(p.WorkspaceID) + "&workspace_grant_revision=" + strconv.FormatInt(p.WorkspaceGrantRevision, 10)
	return nil
}

func (h *Handler) authorizeObjectiveEvidence(c *gin.Context, req objectiveUpdate) error {
	value, linked := c.Get(workspaceSelectionKey)
	if !linked || (req.Status != "complete" && len(req.Evidence) == 0) {
		return nil
	}
	selection := value.(workspaceSelection)
	if err := h.Service.recordSelectedExport(c.Request.Context(), selection, "task_result"); err != nil {
		return rejectOperation(403, "task result export required")
	}
	return nil
}
