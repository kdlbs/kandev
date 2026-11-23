package plugins

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type prepareFilePreviewRequest struct {
	SessionID string `json:"session_id"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	ViewerID  string `json:"viewer_id"`
}

func (r prepareFilePreviewRequest) validPaths() bool {
	return len(r.Path) > 0 && len(r.Path) <= 4096 && len(r.Repo) <= 4096
}

func validFilePreviewSource(source *FilePreviewSource, scope filePreviewScope, policy filePreviewPolicy) bool {
	return source.WorkspaceID == scope.WorkspaceID && source.Read != nil && source.Validate != nil && source.SizeBytes >= 0 && source.SizeBytes <= 64<<20 && (!source.External || policy.Mode != filePreviewDeny)
}

func (s *Service) filePreviewGrantStore() *filePreviewGrants {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.filePreviewGrants == nil {
		s.filePreviewGrants = newFilePreviewGrants()
	}
	return s.filePreviewGrants
}

func (c *Controller) prepareFilePreview(ctx *gin.Context) {
	pluginID := ctx.Param("id")
	scope, ok := c.filePreviewPolicyIdentity(ctx)
	if !ok {
		return
	}
	req, ok := c.bindPrepareFilePreview(ctx, pluginID)
	if !ok {
		return
	}
	policies := c.svc.filePreviewPolicyStore()
	if policies == nil || c.svc.filePreviewBackend == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_unavailable"})
		return
	}
	policy, err := policies.get(scope)
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_policy_unavailable"})
		return
	}
	release, err := c.svc.filePreviewGrantStore().reserve(scope.UserID)
	if err != nil {
		ctx.JSON(http.StatusTooManyRequests, gin.H{"code": "preview_busy"})
		return
	}
	defer release()
	source, err := c.svc.filePreviewBackend(ctx.Request.Context(), req.SessionID, req.Repo, req.Path)
	if errors.Is(err, ErrFilePreviewTooLarge) {
		ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": "preview_too_large"})
		return
	}
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "preview_unavailable"})
		return
	}
	if source == nil || source.Close == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_unavailable"})
		return
	}
	if !validFilePreviewSource(source, scope, policy) {
		source.Close()
		ctx.JSON(http.StatusForbidden, gin.H{"code": "preview_denied"})
		return
	}
	wrapped := c.validatedFilePreviewSource(pluginID, req, scope, policy, source, policies)
	id, err := c.svc.filePreviewGrantStore().add(scope, policy.Revision, source.External && policy.Mode == filePreviewAsk, wrapped)
	if err != nil {
		source.Close()
		ctx.JSON(http.StatusTooManyRequests, gin.H{"code": "preview_busy"})
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, gin.H{"id": id, "path": source.Path, "original_path": req.Path, "size_bytes": source.SizeBytes, "external": source.External, "execution_label": source.ExecutionLabel, "approval_required": source.External && policy.Mode == filePreviewAsk})
}

func (c *Controller) bindPrepareFilePreview(ctx *gin.Context, pluginID string) (prepareFilePreviewRequest, bool) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 16384)
	var req prepareFilePreviewRequest
	if ctx.ShouldBindJSON(&req) != nil || !isBoundedApprovalIdentifier(req.SessionID) || !req.validPaths() || !c.filePreviewEligible(pluginID, req.ViewerID, req.Path) {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "invalid_preview"})
		return req, false
	}
	return req, true
}

func (c *Controller) filePreviewEligible(plugin, viewer, path string) bool {
	record, err := c.svc.Get(plugin)
	if err != nil || record.Status != StatusActive {
		return false
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	for _, declared := range record.UI.FileViewers {
		if declared.ID != viewer {
			continue
		}
		for _, supported := range declared.Extensions {
			if supported == extension {
				return true
			}
		}
	}
	return false
}

type filePreviewWriter struct {
	writer    io.Writer
	remaining int64
	validate  func() error
}

func (w *filePreviewWriter) Write(p []byte) (int, error) {
	if err := w.validate(); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.remaining {
		return 0, io.ErrUnexpectedEOF
	}
	n, err := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, err
}

func (c *Controller) readFilePreview(ctx *gin.Context) {
	scope, ok := c.filePreviewPolicyIdentity(ctx)
	if !ok {
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 1024)
	var req struct {
		Approved bool `json:"approved"`
	}
	if ctx.ShouldBindJSON(&req) != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "invalid_preview"})
		return
	}
	if req.Approved && !interactiveFilePreviewRequest(ctx) {
		return
	}
	policies := c.svc.filePreviewPolicyStore()
	if policies == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": "preview_unavailable"})
		return
	}
	policy, err := policies.get(scope)
	if err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"code": "preview_denied"})
		return
	}
	_ = http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Now().Add(30 * time.Second))
	ctx.Header("Content-Type", "application/octet-stream")
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Header("Content-Disposition", "attachment")
	err = c.svc.filePreviewGrantStore().read(ctx.Request.Context(), ctx.Param("previewID"), scope, policy.Revision, req.Approved, ctx.Writer)
	if err != nil {
		if !ctx.Writer.Written() {
			ctx.JSON(http.StatusForbidden, gin.H{"code": "preview_denied"})
		}
		return
	}
	// The browser requires this final byte and the exact admitted length before parsing.
	_, _ = ctx.Writer.Write([]byte{0})
}

func (c *Controller) cancelFilePreview(ctx *gin.Context) {
	scope, ok := c.filePreviewPolicyIdentity(ctx)
	if !ok {
		return
	}
	c.svc.filePreviewGrantStore().cancelOwned(ctx.Param("previewID"), scope)
	ctx.Status(http.StatusNoContent)
}

func (c *Controller) validatedFilePreviewSource(pluginID string, req prepareFilePreviewRequest, scope filePreviewScope, policy filePreviewPolicy, source *FilePreviewSource, policies *filePreviewPolicies) *filePreviewSource {
	return &filePreviewSource{Close: source.Close, Read: func(readCtx context.Context, w io.Writer) error {
		validate := func() error {
			if err := readCtx.Err(); err != nil {
				return err
			}
			if !c.filePreviewEligible(pluginID, req.ViewerID, req.Path) {
				return errFilePreviewPolicy
			}
			record, err := c.svc.Get(pluginID)
			if err != nil || record.InstallationID != scope.InstallationID || filePreviewDigest(record) != scope.Digest {
				return errFilePreviewPolicy
			}
			current, err := policies.get(scope)
			if err != nil || current.Revision != policy.Revision {
				return errFilePreviewPolicy
			}
			return source.Validate(readCtx)
		}
		bounded := &filePreviewWriter{writer: w, remaining: source.SizeBytes, validate: validate}
		if err := validate(); err != nil {
			return err
		}
		if err := source.Read(readCtx, bounded); err != nil {
			return err
		}
		if bounded.remaining != 0 {
			return io.ErrUnexpectedEOF
		}
		return validate()
	}}
}
