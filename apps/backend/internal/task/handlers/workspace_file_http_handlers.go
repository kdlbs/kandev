package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

// workspaceUploadClient is the slice of the agentctl client this handler needs.
//
// Declaring it here, structurally, keeps the handler off the runtime-tier
// package (ARCH-RUNTIME-IMPORT): the concrete client satisfies it without this
// file naming that package.
type workspaceUploadClient interface {
	PreflightWorkspaceUpload(
		ctx context.Context, dir, repo string, paths []string,
	) ([]agentctltypes.WorkspaceUploadConflict, error)
	UploadWorkspaceFile(
		ctx context.Context, params agentctltypes.WorkspaceUploadParams, content io.Reader,
	) (*agentctltypes.UploadedWorkspaceFile, error)
}

// workspaceUploadMultipartMemory bounds the in-memory portion of the parsed
// form; the file part streams from disk beyond it.
const workspaceUploadMultipartMemory int64 = 16 << 20

// workspaceUploadRequestSlack allows for multipart framing on top of the file.
const workspaceUploadRequestSlack int64 = 4 * 1024 * 1024

// RegisterWorkspaceFileRoutes exposes the session-scoped workspace upload
// surface. Both routes reuse the same session authorization as the other
// task-session routes, so an unauthenticated or unknown session is refused
// before any byte is streamed.
func RegisterWorkspaceFileRoutes(router *gin.Engine, handlers *ProcessHandlers) {
	files := router.Group("/api/v1/task-sessions/:id/workspace/files")
	files.POST("", handlers.httpUploadWorkspaceFile)
	files.POST("/preflight", handlers.httpPreflightWorkspaceUpload)
}

type workspaceUploadPreflightRequest struct {
	Dir   string   `json:"dir"`
	Repo  string   `json:"repo"`
	Paths []string `json:"paths"`
}

// httpPreflightWorkspaceUpload reports which destinations already exist so the
// caller can resolve every conflict before uploading anything.
func (h *ProcessHandlers) httpPreflightWorkspaceUpload(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	if h.denySessionAccess(c, sessionID) {
		return
	}

	var req workspaceUploadPreflightRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if len(req.Paths) == 0 {
		c.JSON(http.StatusOK, gin.H{"conflicts": []agentctltypes.WorkspaceUploadConflict{}})
		return
	}

	client, ok := h.workspaceUploadClient(c, sessionID)
	if !ok {
		return
	}

	conflicts, err := client.PreflightWorkspaceUpload(c.Request.Context(), req.Dir, req.Repo, req.Paths)
	if err != nil {
		h.respondWorkspaceUploadError(c, sessionID, err)
		return
	}
	if conflicts == nil {
		conflicts = []agentctltypes.WorkspaceUploadConflict{}
	}
	c.JSON(http.StatusOK, gin.H{"conflicts": conflicts})
}

// httpUploadWorkspaceFile streams one multipart file part through to agentctl.
// One request carries one file so that a rejection of one file in a selection
// does not fail the rest.
func (h *ProcessHandlers) httpUploadWorkspaceFile(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}
	if h.denySessionAccess(c, sessionID) {
		return
	}

	c.Request.Body = http.MaxBytesReader(
		c.Writer, c.Request.Body, models.MaxMessageAttachmentBytes+workspaceUploadRequestSlack,
	)
	if err := c.Request.ParseMultipartForm(workspaceUploadMultipartMemory); err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload request is too large"})
		return
	}

	relativePath := strings.TrimSpace(c.Request.FormValue("relative_path"))
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "relative_path is required"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file part is required"})
		return
	}
	if fileHeader.Size > models.MaxMessageAttachmentBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the maximum upload size"})
		return
	}
	if declared := strings.TrimSpace(c.Request.FormValue("size_bytes")); declared != "" {
		parsed, parseErr := strconv.ParseInt(declared, 10, 64)
		if parseErr != nil || parsed != fileHeader.Size {
			c.JSON(http.StatusBadRequest, gin.H{"error": "size_bytes does not match the uploaded file"})
			return
		}
	}

	client, ok := h.workspaceUploadClient(c, sessionID)
	if !ok {
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file part could not be read"})
		return
	}
	defer func() { _ = src.Close() }()

	uploaded, err := client.UploadWorkspaceFile(c.Request.Context(), agentctltypes.WorkspaceUploadParams{
		Dir:          c.Request.FormValue("dir"),
		Repo:         c.Request.FormValue("repo"),
		RelativePath: relativePath,
		Resolution:   c.Request.FormValue("resolution"),
		SizeBytes:    fileHeader.Size,
	}, src)
	if err != nil {
		h.respondWorkspaceUploadError(c, sessionID, err)
		return
	}
	c.JSON(http.StatusCreated, uploaded)
}

// workspaceUploadClient resolves the session's agentctl client, responding with
// 503 when the session has no live execution to write into.
func (h *ProcessHandlers) workspaceUploadClient(c *gin.Context, sessionID string) (workspaceUploadClient, bool) {
	execution, found := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !found || execution == nil || execution.GetAgentCtlClient() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace not ready"})
		return nil, false
	}
	return execution.GetAgentCtlClient(), true
}

// respondWorkspaceUploadError preserves the conflict case so the caller can
// prompt for a resolution rather than treating it as a failure.
func (h *ProcessHandlers) respondWorkspaceUploadError(c *gin.Context, sessionID string, err error) {
	if errors.Is(err, agentctltypes.ErrWorkspaceUploadConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	h.logger.Warn("workspace file upload failed",
		zap.String("session_id", sessionID),
		zap.Error(err),
	)
	c.JSON(http.StatusBadGateway, gin.H{"error": "workspace upload failed"})
}
