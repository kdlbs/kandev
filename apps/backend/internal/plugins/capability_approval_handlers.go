package plugins

import (
	"errors"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const maxCapabilityApprovalRequestBytes = 64 * 1024

type capabilityApprovalContextDTO struct {
	InstallationID        string                       `json:"installation_id"`
	WorkspaceID           string                       `json:"workspace_id"`
	ManifestDigest        string                       `json:"manifest_digest"`
	DeclaredCapabilityIDs []string                     `json:"declared_capability_ids"`
	Approval              *CapabilityApprovalDTO       `json:"approval"`
	AuditEvents           []CapabilityApprovalEventDTO `json:"audit_events"`
	RequiresReview        bool                         `json:"requires_review"`
}

type updateCapabilityApprovalRequest struct {
	WorkspaceID      string   `json:"workspace_id"`
	ExpectedRevision uint64   `json:"expected_revision"`
	ManifestDigest   string   `json:"manifest_digest"`
	CapabilityIDs    []string `json:"capability_ids"`
	Reason           string   `json:"reason"`
	AuditID          string   `json:"audit_id"`
}

type revokeCapabilityApprovalRequest struct {
	WorkspaceID      string `json:"workspace_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Reason           string `json:"reason"`
	AuditID          string `json:"audit_id"`
}

func (c *Controller) getCapabilityApprovals(ctx *gin.Context) {
	_, workspaceID, ok := c.capabilityApprovalRequestIdentity(ctx, ctx.Query("workspace_id"))
	if !ok {
		return
	}
	c.svc.approvalEffectMu.Lock()
	defer c.svc.approvalEffectMu.Unlock()
	record, err := c.svc.Get(ctx.Param("id"))
	if err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	declared, err := ManifestCapabilityIDs(record.Manifest)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "plugin capability declaration is invalid"})
		return
	}
	response := capabilityApprovalContextDTO{
		InstallationID:        record.InstallationID,
		WorkspaceID:           workspaceID,
		ManifestDigest:        ManifestCapabilityDigest(record.Manifest),
		DeclaredCapabilityIDs: declared,
		AuditEvents:           []CapabilityApprovalEventDTO{},
	}
	if approval, found, getErr := c.svc.GetCapabilityApproval(record.InstallationID, workspaceID); getErr != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "capability approval state is unavailable"})
		return
	} else if found {
		response.Approval = &approval
		response.RequiresReview = approval.ManifestDigest != response.ManifestDigest
	}
	events, err := c.svc.ListCapabilityApprovalEvents(record.InstallationID, workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "capability approval audit is unavailable"})
		return
	}
	response.AuditEvents = events
	if len(events) > 0 && events[len(events)-1].Type == string(CapabilityApprovalEventUpgradeReview) {
		response.RequiresReview = true
	}
	ctx.JSON(http.StatusOK, response)
}

func (c *Controller) updateCapabilityApprovals(ctx *gin.Context) {
	var request updateCapabilityApprovalRequest
	if err := c.bindCapabilityApprovalJSON(ctx, &request); err != nil {
		return
	}
	identity, workspaceID, ok := c.capabilityApprovalRequestIdentity(ctx, request.WorkspaceID)
	if !ok {
		return
	}
	if request.ExpectedRevision == math.MaxUint64 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "approval revision is invalid"})
		return
	}
	record, err := c.svc.Get(ctx.Param("id"))
	if err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	if request.ManifestDigest != ManifestCapabilityDigest(record.Manifest) {
		ctx.JSON(http.StatusConflict, gin.H{"error": "plugin manifest changed; reload before saving approval"})
		return
	}
	if len(request.CapabilityIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "select at least one declared capability or revoke access"})
		return
	}
	approval, err := c.svc.GrantCapabilityApproval(
		record.InstallationID, workspaceID, request.ExpectedRevision+1,
		request.ManifestDigest, request.CapabilityIDs, identity.UserID, request.Reason, request.AuditID,
	)
	if err != nil {
		c.writeCapabilityApprovalMutationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, approval)
}

func (c *Controller) revokeCapabilityApprovals(ctx *gin.Context) {
	var request revokeCapabilityApprovalRequest
	if err := c.bindCapabilityApprovalJSON(ctx, &request); err != nil {
		return
	}
	identity, workspaceID, ok := c.capabilityApprovalRequestIdentity(ctx, request.WorkspaceID)
	if !ok {
		return
	}
	record, err := c.svc.Get(ctx.Param("id"))
	if err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	approval, err := c.svc.RevokeCapabilityApproval(
		record.InstallationID, workspaceID, request.ExpectedRevision, identity.UserID, request.Reason, request.AuditID,
	)
	if err != nil {
		c.writeCapabilityApprovalMutationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, approval)
}

func (c *Controller) bindCapabilityApprovalJSON(ctx *gin.Context, destination any) error {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxCapabilityApprovalRequestBytes)
	if err := ctx.ShouldBindJSON(destination); err != nil {
		statusCode := http.StatusBadRequest
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			statusCode = http.StatusRequestEntityTooLarge
		}
		ctx.JSON(statusCode, gin.H{"error": "invalid approval request"})
		return err
	}
	return nil
}

func (c *Controller) capabilityApprovalRequestIdentity(ctx *gin.Context, workspaceID string) (authn.Identity, string, bool) {
	identity, authenticated := authn.FromGin(ctx)
	if !authenticated || identity.UserID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return authn.Identity{}, "", false
	}
	if !isBoundedApprovalIdentifier(workspaceID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return authn.Identity{}, "", false
	}
	if c.svc.capabilityApprovalWorkspaceAuthorizer == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace approval authorization is unavailable"})
		return authn.Identity{}, "", false
	}
	if err := c.svc.capabilityApprovalWorkspaceAuthorizer(ctx.Request.Context(), workspaceID); err != nil {
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		} else {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "workspace management permission is required"})
		}
		return authn.Identity{}, "", false
	}
	return identity, workspaceID, true
}

func (c *Controller) writeCapabilityApprovalMutationError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrApprovalLedgerUnavailable):
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "capability approval state is unavailable"})
	case errors.Is(err, ErrApprovalRevisionConflict), errors.Is(err, ErrApprovalIdempotencyConflict),
		errors.Is(err, ErrApprovalInstallationTombstoned):
		ctx.JSON(http.StatusConflict, gin.H{"error": "capability approval changed; reload before retrying"})
	case errors.Is(err, ErrApprovalNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "capability approval not found"})
	case errors.Is(err, store.ErrNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "plugin installation not found"})
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid capability approval"})
	}
}
