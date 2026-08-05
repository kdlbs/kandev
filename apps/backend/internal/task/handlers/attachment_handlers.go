package handlers

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

const maxAttachmentRequestBytes int64 = service.MaxAttachmentBytes + 4*1024*1024
const attachmentMultipartMaxMemory = 1 << 20

// AttachmentHandlers exposes the authenticated staging/download/delete API.
type AttachmentHandlers struct {
	service *service.AttachmentService
	log     *logger.Logger
}

func NewAttachmentHandlers(svc *service.AttachmentService, log *logger.Logger) *AttachmentHandlers {
	return &AttachmentHandlers{service: svc, log: log.WithFields(zap.String("component", "attachment-http-handlers"))}
}

func RegisterAttachmentRoutes(router *gin.Engine, svc *service.AttachmentService, log *logger.Logger) *AttachmentHandlers {
	h := NewAttachmentHandlers(svc, log)
	api := router.Group("/api/v1")
	api.POST("/attachments", h.upload)
	api.GET("/attachments/:id/content", h.download)
	api.DELETE("/attachments/:id", h.remove)
	return h
}

func (h *AttachmentHandlers) owner(c *gin.Context) (string, bool) {
	identity, ok := authn.FromGin(c)
	if !ok || strings.TrimSpace(identity.UserID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return "", false
	}
	return identity.UserID, true
}

func (h *AttachmentHandlers) upload(c *gin.Context) {
	ownerID, ok := h.owner(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAttachmentRequestBytes)
	if err := c.Request.ParseMultipartForm(attachmentMultipartMaxMemory); err != nil {
		if isAttachmentRequestTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "attachment exceeds maximum size"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart upload is required"})
		return
	}
	defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
	workspaceID := strings.TrimSpace(c.PostForm("workspace_id"))
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		h.log.Error("open multipart attachment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read attachment"})
		return
	}
	defer func() { _ = file.Close() }()
	mimeType := strings.TrimSpace(c.PostForm("mime_type"))
	if mimeType == "" {
		mimeType = fileHeader.Header.Get("Content-Type")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	kind := strings.TrimSpace(c.PostForm("kind"))
	if kind == "" {
		kind = attachmentKind(mimeType)
	}
	deliveryMode := strings.TrimSpace(c.PostForm("delivery_mode"))
	if deliveryMode == "" {
		if kind == "image" {
			deliveryMode = "prompt"
		} else {
			deliveryMode = "path"
		}
	}
	attachment, err := h.service.Stage(c.Request.Context(), ownerID, workspaceID, filepath.Base(fileHeader.Filename), mimeType, kind, deliveryMode, file)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, h.service.Descriptor(attachment))
}

func isAttachmentRequestTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr) || errors.Is(err, multipart.ErrMessageTooLarge)
}

func attachmentKind(mimeType string) string {
	if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "image"
	}
	if strings.HasPrefix(strings.ToLower(mimeType), "audio/") {
		return "audio"
	}
	return "resource"
}

func (h *AttachmentHandlers) download(c *gin.Context) {
	ownerID, ok := h.owner(c)
	if !ok {
		return
	}
	attachment, file, err := h.service.Open(c.Request.Context(), ownerID, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	c.Header("Content-Type", attachment.MimeType)
	c.Header("Content-Length", strconv.FormatInt(attachment.SizeBytes, 10))
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", strings.ReplaceAll(attachment.Name, `"`, "")))
	http.ServeContent(c.Writer, c.Request, attachment.Name, attachment.CreatedAt, file)
}

func (h *AttachmentHandlers) remove(c *gin.Context) {
	ownerID, ok := h.owner(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), ownerID, c.Param("id")); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AttachmentHandlers) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAttachmentTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "attachment exceeds maximum size"})
	case errors.Is(err, service.ErrAttachmentForbidden), errors.Is(err, service.ErrAttachmentNotFound), errors.Is(err, models.ErrAttachmentClaimConflict):
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
	case errors.Is(err, service.ErrAttachmentInvalid), errors.Is(err, service.ErrAttachmentTotalTooLarge), errors.Is(err, service.ErrTooManyAttachments):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		h.log.Error("attachment request failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "attachment request failed"})
	}
}
