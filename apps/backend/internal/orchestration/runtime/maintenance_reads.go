package runtime

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) maintenanceReadGrant(c *gin.Context, b *models.AssistantBinding) (*models.MaintenanceGrant, bool) {
	current, err := h.Service.Repo.AssistantBindingByID(c.Request.Context(), b.ID)
	if err != nil || current.Version != b.Version {
		c.AbortWithStatus(409)
		return nil, false
	}
	version, err := strconv.ParseInt(c.Query("expected_binding_version"), 10, 64)
	grantRevision, grantErr := strconv.ParseInt(c.Query("grant_revision"), 10, 64)
	if err != nil || grantErr != nil || version != b.Version {
		c.AbortWithStatus(409)
		return nil, false
	}
	g, err := h.Service.Repo.MaintenanceGrant(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil || g.RevokedAt != nil || g.BindingVersion != b.Version || g.Revision != grantRevision || !g.ExpiresAt.After(h.Service.attentionNow()) {
		c.AbortWithStatus(403)
		return nil, false
	}
	row, err := h.Service.Repo.ImprovementCandidate(c.Request.Context(), b.ID, g.CandidateID)
	if err != nil || row.WorkspaceID != b.WorkspaceID || row.State == "rejected" {
		c.AbortWithStatus(404)
		return nil, false
	}
	if _, err = h.Service.currentMaintenanceResources(c.Request.Context(), b, g); err != nil {
		c.AbortWithStatus(403)
		return nil, false
	}
	return g, true
}

func (h *Handler) maintenanceFile(c *gin.Context) {
	b, ok := h.improvementBinding(c)
	if !ok {
		return
	}
	g, ok := h.maintenanceReadGrant(c, b)
	if !ok {
		return
	}
	file, err := h.Service.Maintenance.Read(c.Request.Context(), *g, c.Query("path"))
	if err != nil {
		c.AbortWithStatus(422)
		return
	}
	// Recheck before returning content if revocation raced the filesystem read.
	if _, ok = h.maintenanceReadGrant(c, b); !ok {
		return
	}
	c.JSON(200, file)
}

func (h *Handler) maintenanceArtifact(c *gin.Context) {
	b, ok := h.improvementBinding(c)
	if !ok {
		return
	}
	var g *models.MaintenanceGrant
	if _, runtime := c.Get("agent_claims"); runtime {
		g, ok = h.maintenanceReadGrant(c, b)
	} else {
		g, ok = h.humanMaintenanceArtifactGrant(c, b)
	}
	if !ok {
		return
	}
	artifact, err := h.Service.Maintenance.Review(c.Request.Context(), *g)
	if err != nil {
		c.AbortWithStatus(422)
		return
	}
	if _, runtime := c.Get("agent_claims"); runtime {
		if _, ok = h.maintenanceReadGrant(c, b); !ok {
			return
		}
	} else if _, ok = h.humanMaintenanceArtifactGrant(c, b); !ok {
		return
	}
	c.JSON(200, artifact)
}

func (h *Handler) humanMaintenanceArtifactGrant(c *gin.Context, b *models.AssistantBinding) (*models.MaintenanceGrant, bool) {
	g, err := h.Service.Repo.MaintenanceGrant(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil || g.WorkspaceID != b.WorkspaceID || g.OwnerUserID != b.OwnerUserID {
		c.AbortWithStatus(404)
		return nil, false
	}
	reader, ok := h.Service.Manager.(MaintenanceReviewReader)
	if !ok || h.Service.Maintenance == nil || reader.ValidateMaintenanceReview(c.Request.Context(), b.WorkspaceID, g.Scope.RepositoryID) != nil {
		c.AbortWithStatus(403)
		return nil, false
	}
	current, err := h.Service.Repo.AssistantBindingByID(c.Request.Context(), b.ID)
	if err != nil || current.Version != b.Version {
		c.AbortWithStatus(409)
		return nil, false
	}
	return g, true
}
