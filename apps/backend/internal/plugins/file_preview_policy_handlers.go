package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/store"
)

// SetFilePreviewWorkspaceAuthorizer supplies the canonical workspace read check
// for user-owned preview policy. It never grants access to another user's policy.
func (s *Service) SetFilePreviewWorkspaceAuthorizer(authorize func(context.Context, string) error) {
	s.filePreviewWorkspaceAuthorizer = authorize
}

func (s *Service) filePreviewPolicyStore() *filePreviewPolicies {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pluginsDir == "" {
		return nil
	}
	if s.filePreviewPolicies == nil || s.filePreviewPolicies.dir != s.pluginsDir {
		s.filePreviewPolicies = newFilePreviewPolicies(s.pluginsDir)
	}
	return s.filePreviewPolicies
}

func (c *Controller) filePreviewPolicyIdentity(ctx *gin.Context) (filePreviewScope, bool) {
	identity, ok := authn.FromGin(ctx)
	if !ok || identity.UserID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"code": "authentication_required"})
		return filePreviewScope{}, false
	}
	workspace := ctx.Query("workspace_id")
	if !isBoundedApprovalIdentifier(workspace) {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "invalid_workspace"})
		return filePreviewScope{}, false
	}
	if c.svc.filePreviewWorkspaceAuthorizer == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_policy_unavailable"})
		return filePreviewScope{}, false
	}
	if err := c.svc.filePreviewWorkspaceAuthorizer(ctx.Request.Context(), workspace); err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"code": "workspace_access_denied"})
		return filePreviewScope{}, false
	}
	record, err := c.svc.Get(ctx.Param("id"))
	if err != nil || record.InstallationID == "" {
		ctx.JSON(http.StatusNotFound, gin.H{"code": "plugin_unavailable"})
		return filePreviewScope{}, false
	}
	return filePreviewScope{UserID: identity.UserID, WorkspaceID: workspace, InstallationID: record.InstallationID, Digest: filePreviewDigest(record)}, true
}

func filePreviewDigest(record *store.Record) string {
	// Installation replacement rotates InstalledAt even for an identical version.
	data, err := json.Marshal(struct {
		Manifest    any
		InstalledAt any
	}{record.Manifest, record.InstalledAt})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (c *Controller) getFilePreviewPolicy(ctx *gin.Context) {
	scope, ok := c.filePreviewPolicyIdentity(ctx)
	if !ok {
		return
	}
	policies := c.svc.filePreviewPolicyStore()
	if policies == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_policy_unavailable"})
		return
	}
	policy, err := policies.get(scope)
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_policy_unavailable"})
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, policy)
}

func (c *Controller) setFilePreviewPolicy(ctx *gin.Context) {
	if !interactiveFilePreviewRequest(ctx) {
		return
	}
	scope, ok := c.filePreviewPolicyIdentity(ctx)
	if !ok {
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 4096)
	var req struct {
		Mode             string `json:"mode"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if ctx.ShouldBindJSON(&req) != nil || !validFilePreviewMode(req.Mode) {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "invalid_preview_policy"})
		return
	}
	policies := c.svc.filePreviewPolicyStore()
	if policies == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_policy_unavailable"})
		return
	}
	policy, err := policies.set(scope, req.Mode, req.ExpectedRevision)
	if errors.Is(err, ErrApprovalRevisionConflict) {
		ctx.JSON(http.StatusConflict, gin.H{"code": "preview_policy_conflict"})
		return
	}
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_policy_unavailable"})
		return
	}
	c.svc.filePreviewGrantStore().revokeScope(scope)
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, policy)
}

func interactiveFilePreviewRequest(ctx *gin.Context) bool {
	identity, ok := authn.FromGin(ctx)
	if !ok || identity.TokenID != "" || (!identity.Synthetic && identity.SessionID == "") {
		ctx.JSON(http.StatusForbidden, gin.H{"code": "interactive_user_required"})
		return false
	}
	return true
}
