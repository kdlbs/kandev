package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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

// workspaceUploadRequestSlack allows for multipart framing on top of the file.
const workspaceUploadRequestSlack int64 = 4 * 1024 * 1024

// workspaceUploadPreflightBodyLimit bounds the JSON envelope before Gin
// decodes a caller-controlled path list.
const workspaceUploadPreflightBodyLimit int64 = 1 << 20

// These limits keep one preflight from consuming unbounded memory or work.
const (
	workspaceUploadMaxPaths     = 4096
	workspaceUploadMaxPathBytes = 4096
)

var errWorkspaceUploadTooLarge = errors.New("workspace upload exceeds maximum size")

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

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, workspaceUploadPreflightBodyLimit)
	var req workspaceUploadPreflightRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		status := http.StatusBadRequest
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if len(req.Paths) > workspaceUploadMaxPaths {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many upload paths"})
		return
	}
	for _, uploadPath := range req.Paths {
		if len(uploadPath) > workspaceUploadMaxPathBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "upload path is too long"})
			return
		}
	}
	if len(req.Dir) > workspaceUploadMaxPathBytes || len(req.Repo) > workspaceUploadMaxPathBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload directory is too long"})
		return
	}
	if len(req.Paths) == 0 {
		c.JSON(http.StatusOK, gin.H{"conflicts": []agentctltypes.WorkspaceUploadConflict{}})
		return
	}

	client, release, ok := h.workspaceUploadClient(c, sessionID)
	if !ok {
		return
	}
	defer release()

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
	// MultipartReader reads from Request.Body, so the body cap is applied before
	// obtaining and consuming the reader.
	multipartReader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart upload is required"})
		return
	}

	parsed, parseErr := parseTaskWorkspaceUploadMultipart(multipartReader)
	if parsed != nil {
		defer parsed.cleanup()
	}
	if parseErr != nil {
		c.JSON(parseErr.status, gin.H{"error": parseErr.message})
		return
	}
	if parsed.staged == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file part is required"})
		return
	}
	if err := parsed.fields.validate(); err != nil {
		if parsed.fields.declaredSize > models.MaxMessageAttachmentBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the maximum upload size"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}
	if parsed.staged.size != parsed.fields.declaredSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "size_bytes does not match uploaded content"})
		return
	}
	if _, err := parsed.staged.file.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "workspace upload failed"})
		return
	}
	uploaded, err := h.forwardWorkspaceUpload(c, sessionID, parsed.fields, parsed.staged.file)
	if err != nil {
		return
	}
	c.JSON(http.StatusCreated, uploaded)
}

type workspaceUploadParseError struct {
	status  int
	message string
}

type parsedTaskWorkspaceUpload struct {
	fields workspaceUploadMultipartFields
	staged *stagedWorkspaceUpload
}

func (p *parsedTaskWorkspaceUpload) cleanup() {
	if p == nil || p.staged == nil {
		return
	}
	p.staged.cleanup()
}

func parseTaskWorkspaceUploadMultipart(reader *multipart.Reader) (*parsedTaskWorkspaceUpload, *workspaceUploadParseError) {
	parsed := &parsedTaskWorkspaceUpload{}
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			return parsed, nil
		}
		if nextErr != nil {
			if isMultipartSizeError(nextErr) {
				return parsed, &workspaceUploadParseError{status: http.StatusRequestEntityTooLarge, message: "upload request is too large"}
			}
			return parsed, &workspaceUploadParseError{status: http.StatusBadRequest, message: "invalid multipart upload"}
		}
		if part.FileName() == "" {
			if parseErr := parseTaskWorkspaceUploadField(part, &parsed.fields); parseErr != nil {
				return parsed, parseErr
			}
			continue
		}
		if part.FormName() != "file" || parsed.staged != nil {
			_ = part.Close()
			return parsed, &workspaceUploadParseError{status: http.StatusBadRequest, message: "exactly one file part is required"}
		}
		staged, stageErr := stageWorkspaceUploadPart(part)
		_ = part.Close()
		if stageErr != nil {
			if errors.Is(stageErr, errWorkspaceUploadTooLarge) || isMultipartSizeError(stageErr) {
				return parsed, &workspaceUploadParseError{status: http.StatusRequestEntityTooLarge, message: "file exceeds the maximum upload size"}
			}
			return parsed, &workspaceUploadParseError{status: http.StatusInternalServerError, message: "workspace upload failed"}
		}
		parsed.staged = staged
	}
}

func parseTaskWorkspaceUploadField(part *multipart.Part, fields *workspaceUploadMultipartFields) *workspaceUploadParseError {
	value, readErr := readWorkspaceUploadField(part)
	_ = part.Close()
	if readErr != nil {
		return &workspaceUploadParseError{status: http.StatusBadRequest, message: "invalid upload metadata"}
	}
	fields.set(part.FormName(), value)
	return nil
}

func isMultipartSizeError(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

type workspaceUploadMultipartFields struct {
	dir          string
	repo         string
	relativePath string
	resolution   string
	declaredSize int64
	hasSize      bool
}

func (f *workspaceUploadMultipartFields) set(name, value string) {
	switch name {
	case "dir":
		f.dir = value
	case "repo":
		f.repo = value
	case "relative_path":
		f.relativePath = value
	case "resolution":
		f.resolution = value
	case "size_bytes":
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && parsed >= 0 {
			f.declaredSize = parsed
			f.hasSize = true
		}
	}
}

func (f workspaceUploadMultipartFields) validate() error {
	if strings.TrimSpace(f.relativePath) == "" {
		return errors.New("relative_path is required")
	}
	if len(f.relativePath) > workspaceUploadMaxPathBytes || len(f.dir) > workspaceUploadMaxPathBytes || len(f.repo) > workspaceUploadMaxPathBytes {
		return errors.New("upload path is too long")
	}
	if !f.hasSize {
		return errors.New("size_bytes is required")
	}
	if f.declaredSize > models.MaxMessageAttachmentBytes {
		return errors.New("file exceeds the maximum upload size")
	}
	return nil
}

const workspaceUploadFieldLimit int64 = workspaceUploadMaxPathBytes + 128

func readWorkspaceUploadField(part *multipart.Part) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, workspaceUploadFieldLimit+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > workspaceUploadFieldLimit {
		return "", errors.New("upload metadata is too long")
	}
	return string(data), nil
}

// boundedWorkspaceUploadReader makes a size mismatch fail before the rooted
// writer renames its temporary file into the destination.
type boundedWorkspaceUploadReader struct {
	part     io.Reader
	expected int64
	read     int64
}

type stagedWorkspaceUpload struct {
	file *os.File
	path string
	size int64
}

func stageWorkspaceUploadPart(part io.Reader) (*stagedWorkspaceUpload, error) {
	tmp, err := os.CreateTemp("", "kandev-workspace-upload-")
	if err != nil {
		return nil, fmt.Errorf("create upload staging file: %w", err)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}
	written, err := io.Copy(tmp, io.LimitReader(part, models.MaxMessageAttachmentBytes+1))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("stage upload: %w", err)
	}
	if written > models.MaxMessageAttachmentBytes {
		cleanup()
		return nil, errWorkspaceUploadTooLarge
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, fmt.Errorf("rewind upload staging file: %w", err)
	}
	return &stagedWorkspaceUpload{file: tmp, path: tmp.Name(), size: written}, nil
}

func (s *stagedWorkspaceUpload) cleanup() {
	if s == nil || s.file == nil {
		return
	}
	_ = s.file.Close()
	_ = os.Remove(s.path)
}

func (r *boundedWorkspaceUploadReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	remaining := r.expected - r.read
	if remaining < int64(len(p)) {
		p = p[:remaining+1]
	}
	n, err := r.part.Read(p)
	r.read += int64(n)
	if r.read > r.expected {
		return n, fmt.Errorf("size_bytes does not match uploaded content")
	}
	if errors.Is(err, io.EOF) && r.read != r.expected {
		return n, fmt.Errorf("size_bytes does not match uploaded content")
	}
	return n, err
}

func (h *ProcessHandlers) forwardWorkspaceUpload(c *gin.Context, sessionID string, fields workspaceUploadMultipartFields, src io.Reader) (*agentctltypes.UploadedWorkspaceFile, error) {
	if err := fields.validate(); err != nil {
		if fields.declaredSize > models.MaxMessageAttachmentBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the maximum upload size"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return nil, err
	}
	client, release, ok := h.workspaceUploadClient(c, sessionID)
	if !ok {
		return nil, errors.New("workspace not ready")
	}
	defer release()
	uploaded, err := client.UploadWorkspaceFile(c.Request.Context(), agentctltypes.WorkspaceUploadParams{
		Dir:          fields.dir,
		Repo:         fields.repo,
		RelativePath: fields.relativePath,
		Resolution:   fields.resolution,
		SizeBytes:    fields.declaredSize,
	}, &boundedWorkspaceUploadReader{part: src, expected: fields.declaredSize})
	if err != nil {
		h.respondWorkspaceUploadError(c, sessionID, err)
		return nil, err
	}
	return uploaded, nil
}

// workspaceUploadClient resolves the session's agentctl client, responding with
// 503 when the session has no live execution to write into.
func (h *ProcessHandlers) workspaceUploadClient(c *gin.Context, sessionID string) (workspaceUploadClient, func(), bool) {
	execution, err := h.lifecycleMgr.GetOrEnsureExecution(c.Request.Context(), sessionID)
	if err != nil || execution == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace not ready"})
		return nil, func() {}, false
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace not ready"})
		return nil, func() {}, false
	}
	return client, release, true
}

// respondWorkspaceUploadError preserves the conflict case so the caller can
// prompt for a resolution rather than treating it as a failure.
func (h *ProcessHandlers) respondWorkspaceUploadError(c *gin.Context, sessionID string, err error) {
	if errors.Is(err, agentctltypes.ErrWorkspaceUploadConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if strings.Contains(err.Error(), "status 413") {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the maximum upload size"})
		return
	}
	if strings.Contains(err.Error(), "status 400") || strings.Contains(err.Error(), "size_bytes does not match") {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.logger.Warn("workspace file upload failed",
		zap.String("session_id", sessionID),
		zap.Error(err),
	)
	c.JSON(http.StatusBadGateway, gin.H{"error": "workspace upload failed"})
}
