package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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

const (
	workspaceUploadPreflightBodyLimit = 1 << 20
	workspaceUploadMaxPaths           = 4096
	workspaceUploadMaxPathBytes       = 4096
)

var errWorkspaceUploadTooLarge = errors.New("workspace upload exceeds maximum size")

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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, workspaceUploadPreflightBodyLimit)
	var req uploadPreflightRequest
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
	if len(req.Dir) > workspaceUploadMaxPathBytes || len(req.Repo) > workspaceUploadMaxPathBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload directory is too long"})
		return
	}
	for _, uploadPath := range req.Paths {
		if len(uploadPath) > workspaceUploadMaxPathBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "upload path is too long"})
			return
		}
	}
	if len(req.Paths) == 0 {
		c.JSON(http.StatusOK, uploadPreflightResponse{Conflicts: []process.UploadConflict{}})
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
	multipartReader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "multipart upload is required"})
		return
	}

	parsed, parseErr := parseWorkspaceUploadMultipart(multipartReader)
	if parsed != nil {
		defer parsed.cleanup()
	}
	if parseErr != nil {
		c.JSON(parseErr.status, gin.H{"error": parseErr.message})
		return
	}
	if parsed.staged != nil {
		if _, err := parsed.staged.file.Seek(0, io.SeekStart); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "workspace upload failed"})
			return
		}
		parsed.source = parsed.staged.file
	}
	if parsed.source == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file part is required"})
		return
	}
	response, err := s.writeWorkspaceUploadPart(c, parsed.fields, parsed.source)
	if err != nil {
		return
	}
	c.JSON(http.StatusCreated, response)
}

type workspaceUploadParseError struct {
	status  int
	message string
}

func (e workspaceUploadParseError) Error() string { return e.message }

type parsedWorkspaceUpload struct {
	fields       workspaceUploadFields
	source       io.Reader
	sourceCloser io.Closer
	staged       *stagedWorkspaceUpload
}

func (p *parsedWorkspaceUpload) cleanup() {
	if p == nil {
		return
	}
	if p.sourceCloser != nil {
		_ = p.sourceCloser.Close()
	}
	if p.staged != nil {
		p.staged.cleanup()
	}
}

func parseWorkspaceUploadMultipart(reader *multipart.Reader) (*parsedWorkspaceUpload, *workspaceUploadParseError) {
	parsed := &parsedWorkspaceUpload{}
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
			if parseErr := parseWorkspaceUploadField(part, &parsed.fields); parseErr != nil {
				return parsed, parseErr
			}
			continue
		}
		if part.FormName() != "file" || parsed.source != nil || parsed.staged != nil {
			_ = part.Close()
			return parsed, &workspaceUploadParseError{status: http.StatusBadRequest, message: "exactly one file part is required"}
		}
		if !parsed.fields.allFieldsSeen() {
			staged, stageErr := stageWorkspaceUploadPart(part)
			_ = part.Close()
			if stageErr != nil {
				if errors.Is(stageErr, errWorkspaceUploadTooLarge) || isMultipartSizeError(stageErr) {
					return parsed, &workspaceUploadParseError{status: http.StatusRequestEntityTooLarge, message: "file exceeds the maximum upload size"}
				}
				return parsed, &workspaceUploadParseError{status: http.StatusInternalServerError, message: "workspace upload failed"}
			}
			parsed.staged = staged
			continue
		}
		parsed.source = part
		parsed.sourceCloser = part
		return parsed, nil
	}
}

func parseWorkspaceUploadField(part *multipart.Part, fields *workspaceUploadFields) *workspaceUploadParseError {
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

type workspaceUploadFields struct {
	dir            string
	repo           string
	relativePath   string
	resolution     string
	declaredSize   int64
	hasSize        bool
	seenDir        bool
	seenRepo       bool
	seenPath       bool
	seenResolution bool
	seenSize       bool
}

func (f *workspaceUploadFields) set(name, value string) {
	switch name {
	case "dir":
		f.dir = value
		f.seenDir = true
	case "repo":
		f.repo = value
		f.seenRepo = true
	case "relative_path":
		f.relativePath = value
		f.seenPath = true
	case "resolution":
		f.resolution = value
		f.seenResolution = true
	case "size_bytes":
		f.seenSize = true
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			f.declaredSize = parsed
			f.hasSize = true
		}
	}
}

func (f workspaceUploadFields) allFieldsSeen() bool {
	return f.seenDir && f.seenRepo && f.seenPath && f.seenResolution && f.seenSize
}

func (f workspaceUploadFields) validate() (process.UploadResolution, error) {
	if strings.TrimSpace(f.relativePath) == "" {
		return process.UploadResolutionNone, errors.New("relative_path is required")
	}
	if len(f.relativePath) > workspaceUploadMaxPathBytes || len(f.dir) > workspaceUploadMaxPathBytes || len(f.repo) > workspaceUploadMaxPathBytes {
		return process.UploadResolutionNone, errors.New("upload path is too long")
	}
	if !f.hasSize || f.declaredSize < 0 {
		return process.UploadResolutionNone, errors.New("size_bytes is required")
	}
	if f.declaredSize > maxWorkspaceUploadBytes {
		return process.UploadResolutionNone, errors.New("file exceeds the maximum upload size")
	}
	resolution, err := process.ParseUploadResolution(f.resolution)
	if err != nil {
		return process.UploadResolutionNone, err
	}
	return resolution, nil
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

type boundedWorkspaceUploadReader struct {
	part     io.Reader
	expected int64
	read     int64
}

type stagedWorkspaceUpload struct {
	file *os.File
	path string
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
	written, err := io.Copy(tmp, io.LimitReader(part, maxWorkspaceUploadBytes+1))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("stage upload: %w", err)
	}
	if written > maxWorkspaceUploadBytes {
		cleanup()
		return nil, errWorkspaceUploadTooLarge
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, fmt.Errorf("rewind upload staging file: %w", err)
	}
	return &stagedWorkspaceUpload{file: tmp, path: tmp.Name()}, nil
}

func (s *Server) writeWorkspaceUploadPart(c *gin.Context, fields workspaceUploadFields, src io.Reader) (*workspaceUploadResponse, error) {
	resolution, validateErr := fields.validate()
	if validateErr != nil {
		if fields.declaredSize > maxWorkspaceUploadBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file exceeds the maximum upload size"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": validateErr.Error()})
		}
		return nil, validateErr
	}
	scopedPath, scopeErr := s.scopedUploadPath(fields.repo, fields.dir, fields.relativePath)
	if scopeErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": scopeErr.Error()})
		return nil, scopeErr
	}
	writtenPath, written, writeErr := s.procMgr.GetWorkspaceTracker().WriteFileStream(
		scopedPath,
		resolution,
		&boundedWorkspaceUploadReader{part: src, expected: fields.declaredSize},
	)
	if writeErr != nil {
		s.respondUploadError(c, scopedPath, writeErr)
		return nil, writeErr
	}
	return &workspaceUploadResponse{
		Path:              writtenPath,
		SizeBytes:         written,
		ResolutionApplied: string(resolution),
	}, nil
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
		return n, errors.New("size_bytes does not match uploaded content")
	}
	if errors.Is(err, io.EOF) && r.read != r.expected {
		return n, errors.New("size_bytes does not match uploaded content")
	}
	return n, err
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
		"size_bytes does not match",
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
