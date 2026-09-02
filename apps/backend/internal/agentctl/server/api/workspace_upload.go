package api

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// maxWorkspaceUploadBytes caps a single uploaded workspace file. It reuses the
// message-attachment limit so an upload and a chat attachment behave alike.
const maxWorkspaceUploadBytes int64 = taskmodels.MaxMessageAttachmentBytes

// workspaceUploadRequestSlack allows for multipart framing on top of the file
// itself, matching the allowance the attachment endpoint already makes.
const workspaceUploadRequestSlack int64 = 4 * 1024 * 1024

// workspaceUploadMultipartMemory bounds the in-memory portion of the parsed
// form. The file part streams from disk beyond this.
const workspaceUploadMultipartMemory int64 = 16 << 20

type uploadPreflightRequest struct {
	Dir   string   `json:"dir"`
	Repo  string   `json:"repo"`
	Paths []string `json:"paths"`
}

type uploadPreflightResponse struct {
	Conflicts []process.UploadConflict `json:"conflicts"`
}

type workspaceUploadResponse struct {
	Path              string `json:"path"`
	SizeBytes         int64  `json:"size_bytes"`
	ResolutionApplied string `json:"resolution_applied,omitempty"`
}

// handleUploadPreflight reports which of the requested destinations already
// exist, so the caller can resolve every conflict before sending any bytes.
func (s *Server) handleUploadPreflight(c *gin.Context) {
	var req uploadPreflightRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	scoped := make([]string, 0, len(req.Paths))
	for _, rel := range req.Paths {
		joined, err := s.scopedUploadPath(req.Repo, req.Dir, rel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		scoped = append(scoped, joined)
	}

	conflicts, err := s.procMgr.GetWorkspaceTracker().CheckUploadConflicts(scoped)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, uploadPreflightResponse{Conflicts: conflicts})
}

// handleFileUpload streams one multipart file part into the workspace.
func (s *Server) handleFileUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWorkspaceUploadBytes+workspaceUploadRequestSlack)
	if err := c.Request.ParseMultipartForm(workspaceUploadMultipartMemory); err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "upload request is too large"})
		return
	}

	declaredSize, err := strconv.ParseInt(strings.TrimSpace(c.Request.FormValue("size_bytes")), 10, 64)
	if err != nil || declaredSize < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "size_bytes is required"})
		return
	}
	if declaredSize > maxWorkspaceUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the maximum upload size"})
		return
	}

	resolution, err := process.ParseUploadResolution(c.Request.FormValue("resolution"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	relativePath := strings.TrimSpace(c.Request.FormValue("relative_path"))
	if relativePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "relative_path is required"})
		return
	}

	scopedPath, err := s.scopedUploadPath(c.Request.FormValue("repo"), c.Request.FormValue("dir"), relativePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file part is required"})
		return
	}
	if fileHeader.Size != declaredSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "size_bytes does not match the uploaded file"})
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file part could not be read"})
		return
	}
	defer func() { _ = src.Close() }()

	writtenPath, written, err := s.procMgr.GetWorkspaceTracker().WriteFileStream(scopedPath, resolution, src)
	if err != nil {
		s.respondUploadError(c, scopedPath, err)
		return
	}

	c.JSON(http.StatusCreated, workspaceUploadResponse{
		Path:              writtenPath,
		SizeBytes:         written,
		ResolutionApplied: string(resolution),
	})
}

// respondUploadError maps a write failure onto a status that tells the caller
// whether to resolve a conflict, fix the request, or retry.
func (s *Server) respondUploadError(c *gin.Context, scopedPath string, err error) {
	if errors.Is(err, process.ErrUploadConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if isUploadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s.logger.Error("workspace file upload failed", zap.String("path", scopedPath), zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "workspace upload failed"})
}

// isUploadRequestError distinguishes a caller mistake, such as an escaping
// path, from a genuine server-side failure.
func isUploadRequestError(err error) bool {
	msg := err.Error()
	for _, marker := range []string{
		"path traversal",
		"path outside workspace",
		"outside the workspace",
		"invalid path",
		"is a directory",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// scopedUploadPath joins the destination directory and the file's path beneath
// it, then applies repository scoping.
//
// An escaping segment is rejected rather than normalized away. Silently
// rewriting "../x" to "x" would land the file somewhere the caller did not ask
// for, which is a worse failure than refusing the request.
func (s *Server) scopedUploadPath(repo, dir, relativePath string) (string, error) {
	cleanRel, err := sanitizeUploadSegment(relativePath, "relative_path")
	if err != nil {
		return "", err
	}

	joined := cleanRel
	if strings.TrimSpace(dir) != "" {
		cleanDir, err := sanitizeUploadSegment(dir, "dir")
		if err != nil {
			return "", err
		}
		joined = cleanDir + "/" + cleanRel
	}

	scoped, err := s.procMgr.JoinRepoPath(repo, joined)
	if err != nil {
		return "", err
	}
	return scoped, nil
}

// sanitizeUploadSegment validates one caller-supplied path fragment, rejecting
// absolute paths and any traversal component.
func sanitizeUploadSegment(raw, field string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if normalized == "" {
		return "", fmt.Errorf("invalid path: %s is empty", field)
	}
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("invalid path: %s must be relative", field)
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", fmt.Errorf("path traversal detected in %s", field)
		}
	}
	cleaned := strings.Trim(path.Clean(normalized), "/")
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("invalid path: %s", raw)
	}
	return cleaned, nil
}
