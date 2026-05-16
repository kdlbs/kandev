package dashboard

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"

	"go.uber.org/zap"
)

const (
	defaultRevisionLimit = 50
	isoTimeFormat        = "2006-01-02T15:04:05Z"
)

// DocumentHandler provides HTTP handlers for task document CRUD,
// revision history, and attachment upload/download.
type DocumentHandler struct {
	svc      *taskservice.DocumentService
	basePath string
	logger   *logger.Logger
}

// NewDocumentHandler creates a new DocumentHandler.
// basePath is the root storage directory (e.g. KANDEV_HOME); attachments are
// stored under <basePath>/data/attachments/<taskID>/<key>.<ext>.
func NewDocumentHandler(svc *taskservice.DocumentService, basePath string, log *logger.Logger) *DocumentHandler {
	return &DocumentHandler{
		svc:      svc,
		basePath: filepath.Join(basePath, "data"),
		logger:   log.WithFields(zap.String("component", "document-handler")),
	}
}

// RegisterDocumentRoutes mounts document routes on the given router group.
func RegisterDocumentRoutes(api *gin.RouterGroup, h *DocumentHandler) {
	api.GET("/tasks/:id/documents", h.listDocuments)
	api.GET("/tasks/:id/documents/:key", h.getDocument)
	api.PUT("/tasks/:id/documents/:key", h.createOrUpdateDocument)
	api.DELETE("/tasks/:id/documents/:key", h.deleteDocument)
	api.GET("/tasks/:id/documents/:key/revisions", h.listRevisions)
	api.POST("/tasks/:id/documents/:key/revisions/:revId/restore", h.revertDocument)
	api.POST("/tasks/:id/documents/:key/upload", h.uploadAttachment)
	api.GET("/tasks/:id/documents/:key/download", h.downloadAttachment)
}

func (h *DocumentHandler) listDocuments(c *gin.Context) {
	taskID := c.Param("id")
	docs, err := h.svc.ListDocuments(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dtos := make([]*DocumentDTO, len(docs))
	for i, d := range docs {
		dtos[i] = documentToDTO(d)
	}
	c.JSON(http.StatusOK, DocumentListResponse{Documents: dtos})
}

func (h *DocumentHandler) getDocument(c *gin.Context) {
	taskID := c.Param("id")
	key := c.Param("key")
	doc, err := h.svc.GetDocument(c.Request.Context(), taskID, key)
	if err != nil {
		if errors.Is(err, taskservice.ErrDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, DocumentResponse{Document: documentToDTO(doc)})
}

func (h *DocumentHandler) createOrUpdateDocument(c *gin.Context) {
	taskID := c.Param("id")
	key := c.Param("key")

	var req CreateOrUpdateDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	doc, err := h.svc.CreateOrUpdateDocument(
		c.Request.Context(),
		taskID, key,
		req.Type, req.Title, req.Content,
		req.AuthorKind, req.AuthorName,
	)
	if err != nil {
		if errors.Is(err, taskservice.ErrDocumentKeyRequired) || errors.Is(err, taskservice.ErrDocumentTaskRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, DocumentResponse{Document: documentToDTO(doc)})
}

func (h *DocumentHandler) deleteDocument(c *gin.Context) {
	taskID := c.Param("id")
	key := c.Param("key")

	if err := h.svc.DeleteDocument(c.Request.Context(), taskID, key); err != nil {
		if errors.Is(err, taskservice.ErrDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *DocumentHandler) listRevisions(c *gin.Context) {
	taskID := c.Param("id")
	key := c.Param("key")

	limit := defaultRevisionLimit
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	revs, err := h.svc.ListRevisions(c.Request.Context(), taskID, key, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	dtos := make([]*DocumentRevisionDTO, len(revs))
	for i, r := range revs {
		dtos[i] = revisionToDTO(r)
	}
	c.JSON(http.StatusOK, DocumentRevisionListResponse{Revisions: dtos})
}

func (h *DocumentHandler) revertDocument(c *gin.Context) {
	taskID := c.Param("id")
	key := c.Param("key")
	revID := c.Param("revId")

	rev, err := h.svc.RevertDocument(c.Request.Context(), taskID, key, revID)
	if err != nil {
		if errors.Is(err, taskservice.ErrDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "revision not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, DocumentRevisionResponse{Revision: revisionToDTO(rev)})
}

func (h *DocumentHandler) uploadAttachment(c *gin.Context) {
	taskID := c.Param("id")
	key := c.Param("key")

	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open uploaded file"})
		return
	}
	defer f.Close() //nolint:errcheck

	data, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read uploaded file"})
		return
	}

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	doc, err := h.svc.UploadAttachment(
		c.Request.Context(),
		taskID, key,
		fh.Filename, mimeType,
		data,
		h.basePath,
	)
	if err != nil {
		if errors.Is(err, taskservice.ErrAttachmentTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, DocumentResponse{Document: documentToDTO(doc)})
}

func (h *DocumentHandler) downloadAttachment(c *gin.Context) {
	taskID := c.Param("id")
	key := c.Param("key")

	diskPath, doc, err := h.svc.DownloadAttachment(c.Request.Context(), taskID, key)
	if err != nil {
		if errors.Is(err, taskservice.ErrDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if doc.MimeType != "" {
		c.Header("Content-Type", doc.MimeType)
	}
	if doc.Filename != "" {
		c.Header("Content-Disposition", "attachment; filename=\""+doc.Filename+"\"")
	}
	c.File(diskPath)
}

// -- Conversion helpers --

func documentToDTO(d *models.TaskDocument) *DocumentDTO {
	return &DocumentDTO{
		ID:         d.ID,
		TaskID:     d.TaskID,
		Key:        d.Key,
		Type:       d.Type,
		Title:      d.Title,
		Content:    d.Content,
		AuthorKind: d.AuthorKind,
		AuthorName: d.AuthorName,
		Filename:   d.Filename,
		MimeType:   d.MimeType,
		SizeBytes:  d.SizeBytes,
		CreatedAt:  d.CreatedAt.UTC().Format(isoTimeFormat),
		UpdatedAt:  d.UpdatedAt.UTC().Format(isoTimeFormat),
	}
}

func revisionToDTO(r *models.TaskDocumentRevision) *DocumentRevisionDTO {
	return &DocumentRevisionDTO{
		ID:                 r.ID,
		TaskID:             r.TaskID,
		DocumentKey:        r.DocumentKey,
		RevisionNumber:     r.RevisionNumber,
		Title:              r.Title,
		Content:            r.Content,
		AuthorKind:         r.AuthorKind,
		AuthorName:         r.AuthorName,
		RevertOfRevisionID: r.RevertOfRevisionID,
		CreatedAt:          r.CreatedAt.UTC().Format(isoTimeFormat),
		UpdatedAt:          r.UpdatedAt.UTC().Format(isoTimeFormat),
	}
}
