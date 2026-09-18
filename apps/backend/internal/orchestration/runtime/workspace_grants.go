package runtime

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
	"slices"
)

type WorkspaceGrantReader interface {
	WorkspaceGrantOptions(context.Context) ([]models.WorkspaceGrantOption, error)
	WorkspaceGrantAccess(context.Context, string, bool) (models.WorkspaceGrantOption, error)
}

func (h *Handler) registerWorkspaceGrantRoutes(g *gin.RouterGroup) {
	g.GET("/assistant/workspace-links", h.workspaceGrants)
	g.GET("/runtime/workspace-links", h.runtimeWorkspaceLinks)
	g.GET("/assistant/workspace-options", h.workspaceGrantOptions)
	g.PUT("/assistant/workspace-links/:workspaceId", h.saveWorkspaceGrant)
	g.DELETE("/assistant/workspace-links/:workspaceId", h.revokeWorkspaceGrant)
	g.GET("/assistant/workspace-links/:workspaceId/events", h.workspaceGrantEvents)
	g.POST("/assistant/workspace-links/:workspaceId/forget", h.forgetWorkspaceContext)
	g.GET("/assistant/workspace-exports", h.workspaceExports)
}

type workspaceGrantRequest struct {
	ExpectedBindingVersion int64                         `json:"expected_binding_version"`
	ExpectedRevision       int64                         `json:"expected_revision"`
	Receiver               models.WorkspaceGrantReceiver `json:"receiver"`
	Scope                  models.WorkspaceGrantScope    `json:"scope"`
}

func (h *Handler) saveWorkspaceGrant(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req workspaceGrantRequest
	if c.ShouldBindJSON(&req) != nil || req.ExpectedRevision < 0 {
		c.AbortWithStatus(400)
		return
	}
	if req.ExpectedBindingVersion != b.Version {
		c.AbortWithStatus(409)
		return
	}
	if err := req.Scope.Validate(); err != nil {
		c.JSON(422, gin.H{errorResponseKey: err.Error()})
		return
	}
	receiver, err := h.Service.workspaceGrantReceiver(c.Request.Context(), b)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	if req.Receiver.ProfileID != receiver.ProfileID || req.Receiver.ProfileRevision != receiver.ProfileRevision || req.Receiver.AuthorityRevision != receiver.AuthorityRevision {
		c.AbortWithStatus(409)
		return
	}
	workspace := c.Param("workspaceId")
	if workspace == b.WorkspaceID {
		c.AbortWithStatus(422)
		return
	}
	reader, ok := h.Service.Manager.(WorkspaceGrantReader)
	if !ok {
		c.AbortWithStatus(503)
		return
	}
	if _, err = reader.WorkspaceGrantAccess(c.Request.Context(), workspace, slices.Contains(req.Scope.Operations, workspaceCoordinate)); err != nil {
		c.AbortWithStatus(404)
		return
	}
	g := &models.WorkspaceGrant{WorkspaceID: workspace, BindingVersion: b.Version, Scope: req.Scope, ReceiverProfileID: receiver.ProfileID, ReceiverProfileRevision: receiver.ProfileRevision, AuthorityRevision: receiver.AuthorityRevision}
	if err = h.Service.Repo.SaveWorkspaceGrant(c.Request.Context(), b, g, req.ExpectedRevision); err != nil {
		workspaceGrantFailure(c, err)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, g)
}

func (h *Handler) revokeWorkspaceGrant(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req workspaceGrantRequest
	if c.ShouldBindJSON(&req) != nil {
		c.AbortWithStatus(400)
		return
	}
	if req.ExpectedBindingVersion != b.Version {
		c.AbortWithStatus(409)
		return
	}
	if err := h.Service.Repo.RevokeWorkspaceGrant(c.Request.Context(), b, c.Param("workspaceId"), req.ExpectedRevision); err != nil {
		workspaceGrantFailure(c, err)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, gin.H{"revoked": true, revisionResponseKey: req.ExpectedRevision + 1})
}

func workspaceGrantFailure(c *gin.Context, err error) {
	if errors.Is(err, models.ErrConflict) {
		c.AbortWithStatus(409)
	} else {
		c.AbortWithStatus(503)
	}
}
