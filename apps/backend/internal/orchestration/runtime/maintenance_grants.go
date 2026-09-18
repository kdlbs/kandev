package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

type maintenanceGrantRequest struct {
	ExpectedBindingVersion int64                   `json:"expected_binding_version"`
	ExpectedRevision       int64                   `json:"expected_revision"`
	CandidateRevision      int64                   `json:"candidate_revision"`
	Scope                  models.MaintenanceScope `json:"scope"`
	ExpiresAt              time.Time               `json:"expires_at"`
}

func (h *Handler) saveMaintenanceGrant(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req maintenanceGrantRequest
	if c.ShouldBindJSON(&req) != nil || req.ExpectedRevision < 0 {
		c.AbortWithStatus(400)
		return
	}
	if b.Version != req.ExpectedBindingVersion {
		c.AbortWithStatus(409)
		return
	}
	if err := req.Scope.Validate(); err != nil {
		c.JSON(422, gin.H{errorResponseKey: err.Error()})
		return
	}
	grant, err := h.Service.prepareMaintenanceGrant(c.Request.Context(), b, c.Param("id"), req)
	if err != nil {
		c.JSON(422, gin.H{errorResponseKey: err.Error()})
		return
	}
	if err = h.Service.Repo.SaveMaintenanceGrant(c.Request.Context(), b, grant, req.ExpectedRevision, req.CandidateRevision); err != nil {
		if errors.Is(err, models.ErrConflict) {
			c.AbortWithStatus(409)
		} else {
			c.AbortWithStatus(503)
		}
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, grant)
}

func (s *Service) prepareMaintenanceGrant(ctx context.Context, b *models.AssistantBinding, id string, req maintenanceGrantRequest) (*models.MaintenanceGrant, error) {
	candidate, err := s.Repo.ImprovementCandidate(ctx, b.ID, id)
	if err != nil || candidate.WorkspaceID != b.WorkspaceID || candidate.State != "proposed" || candidate.Revision != req.CandidateRevision {
		return nil, fmt.Errorf("maintenance_proposal_superseded")
	}
	reader, ok := s.Manager.(MaintenanceScopeReader)
	if !ok || s.Maintenance == nil {
		return nil, fmt.Errorf("maintenance_sandbox_unavailable")
	}
	source, err := reader.ValidateMaintenanceScope(ctx, b.WorkspaceID, req.Scope)
	if err != nil {
		return nil, fmt.Errorf("maintenance_resources_unavailable")
	}
	profile, err := s.contextProfileRevision(ctx, b.WorkspaceID, req.Scope.ProfileID)
	if err != nil {
		return nil, err
	}
	authority, err := s.assistantAuthority(ctx, b.ConversationID)
	if err != nil || authority == nil {
		return nil, fmt.Errorf("assistant_authority_unavailable")
	}
	scope, base, err := s.Maintenance.Inspect(ctx, source, req.Scope)
	if err != nil {
		return nil, err
	}
	return &models.MaintenanceGrant{CandidateID: id, BindingVersion: b.Version, AuthorityRevision: authority.Revision, ProfileRevision: profile,
		Scope: scope, BaseOID: base, ExpiresAt: req.ExpiresAt}, nil
}

func (h *Handler) revokeMaintenanceGrant(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req maintenanceGrantRequest
	if c.ShouldBindJSON(&req) != nil {
		c.AbortWithStatus(400)
		return
	}
	if b.Version != req.ExpectedBindingVersion {
		c.AbortWithStatus(409)
		return
	}
	if err := h.Service.Repo.RevokeMaintenanceGrant(c.Request.Context(), b, c.Param("id"), req.ExpectedRevision); err != nil {
		if errors.Is(err, models.ErrConflict) {
			c.AbortWithStatus(409)
		} else {
			c.AbortWithStatus(503)
		}
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, gin.H{"revoked": true, revisionResponseKey: req.ExpectedRevision + 1})
}

func (h *Handler) improvement(c *gin.Context) {
	b, ok := h.improvementBinding(c)
	if !ok {
		return
	}
	row, err := h.Service.Repo.ImprovementCandidate(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil || row.WorkspaceID != b.WorkspaceID {
		c.AbortWithStatus(404)
		return
	}
	grant, err := h.Service.Repo.MaintenanceGrant(c.Request.Context(), b.ID, row.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.AbortWithStatus(503)
		return
	}
	visible, next, ok := h.improvementEvidencePage(c, b, row)
	if !ok {
		return
	}
	validation, err := h.Service.Repo.MaintenanceValidation(c.Request.Context(), b.ID, row.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.AbortWithStatus(503)
		return
	}
	review, err := h.Service.Repo.ImprovementReview(c.Request.Context(), b.ID, row.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.AbortWithStatus(503)
		return
	}
	c.JSON(200, gin.H{"candidate": row, "grant": grant, "evidence": visible, "evidence_next_cursor": next, "validation": validation, "review": review})
}
