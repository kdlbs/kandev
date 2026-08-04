package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const maxMaterializedAttachmentRequestBytes int64 = models.MaxMessageAttachmentBytes + 4*1024*1024

type materializedAttachmentResponse struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

type materializedAttachmentUpload struct {
	sessionID    string
	attachmentID string
	name         string
	mimeType     string
	declaredSize int64
	fileHeader   *multipart.FileHeader
}

func (s *Server) handleMaterializeAttachment(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMaterializedAttachmentRequestBytes)
	if err := c.Request.ParseMultipartForm(16 << 20); err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "attachment request is too large"})
		return
	}
	upload, status, message := parseMaterializedAttachmentUpload(c)
	if status != 0 {
		c.JSON(status, gin.H{"error": message})
		return
	}
	response, err := s.persistMaterializedAttachment(upload)
	if err != nil {
		if status, message, ok := materializedAttachmentError(err); ok {
			c.JSON(status, gin.H{"error": message})
			return
		}
		s.logger.Error("materialize attachment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "attachment storage unavailable"})
		return
	}
	c.JSON(http.StatusCreated, response)
}

func parseMaterializedAttachmentUpload(c *gin.Context) (materializedAttachmentUpload, int, string) {
	upload := materializedAttachmentUpload{
		sessionID:    strings.TrimSpace(c.PostForm("session_id")),
		attachmentID: strings.TrimSpace(c.PostForm("attachment_id")),
		name:         strings.TrimSpace(c.PostForm("name")),
		mimeType:     strings.TrimSpace(c.PostForm("mime_type")),
	}
	declaredSize, err := strconv.ParseInt(strings.TrimSpace(c.PostForm("size_bytes")), 10, 64)
	if upload.sessionID == "" || upload.attachmentID == "" || upload.name == "" || upload.mimeType == "" || err != nil || declaredSize < 0 || declaredSize > models.MaxMessageAttachmentBytes {
		return upload, http.StatusBadRequest, "attachment metadata is invalid"
	}
	upload.declaredSize = declaredSize
	if !isSafeAttachmentComponent(upload.sessionID) || !isSafeAttachmentComponent(upload.attachmentID) {
		return upload, http.StatusBadRequest, "attachment identity is invalid"
	}
	upload.fileHeader, err = c.FormFile("file")
	if err != nil {
		return upload, http.StatusBadRequest, "file is required"
	}
	return upload, 0, ""
}

func (s *Server) persistMaterializedAttachment(upload materializedAttachmentUpload) (materializedAttachmentResponse, error) {
	file, err := upload.fileHeader.Open()
	if err != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusBadRequest, "file is unreadable")
	}
	defer func() { _ = file.Close() }()

	dir, err := safeAttachmentPath(s.cfg.WorkDir, ".kandev", "attachments", upload.sessionID)
	if err != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusBadRequest, "attachment identity is invalid")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.logger.Error("create agent attachment directory", zap.Error(err))
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusInternalServerError, "attachment storage unavailable")
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusInternalServerError, "attachment storage unavailable")
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	defer cleanup()
	n, copyErr := io.Copy(tmp, io.LimitReader(file, models.MaxMessageAttachmentBytes+1))
	if copyErr != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusBadRequest, "attachment upload failed")
	}
	if n != upload.declaredSize || n > models.MaxMessageAttachmentBytes {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusRequestEntityTooLarge, "attachment size does not match its descriptor")
	}
	if err := tmp.Close(); err != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusInternalServerError, "attachment storage unavailable")
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusInternalServerError, "attachment storage unavailable")
	}
	destinationName := safeAttachmentName(upload.name)
	if destinationName == "" {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusBadRequest, "attachment name is invalid")
	}
	destinationName = uniqueAttachmentName(dir, destinationName)
	destination, err := safeAttachmentPath(dir, destinationName)
	if err != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusBadRequest, "attachment name is invalid")
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return materializedAttachmentResponse{}, newMaterializedAttachmentError(http.StatusInternalServerError, "attachment storage unavailable")
	}
	return materializedAttachmentResponse{Name: destinationName, SizeBytes: n}, nil
}

type materializedAttachmentHTTPError struct {
	status  int
	message string
}

func (e *materializedAttachmentHTTPError) Error() string { return e.message }

func newMaterializedAttachmentError(status int, message string) error {
	return &materializedAttachmentHTTPError{status: status, message: message}
}

func materializedAttachmentError(err error) (int, string, bool) {
	var httpErr *materializedAttachmentHTTPError
	if !errors.As(err, &httpErr) {
		return 0, "", false
	}
	return httpErr.status, httpErr.message, true
}

func safeAttachmentName(name string) string {
	if !isSafeAttachmentComponent(name) {
		return ""
	}
	return name
}

func isSafeAttachmentComponent(value string) bool {
	return value != "" && filepath.IsLocal(value) && filepath.Base(value) == value && !strings.ContainsAny(value, "/\\\x00")
}

func safeAttachmentPath(root string, components ...string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("attachment root is required")
	}
	parts := make([]string, 0, len(components)+1)
	parts = append(parts, root)
	for _, component := range components {
		if !isSafeAttachmentComponent(component) {
			return "", fmt.Errorf("invalid attachment path component")
		}
		parts = append(parts, component)
	}
	path := filepath.Join(parts...)
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("attachment path escapes root")
	}
	return path, nil
}

func uniqueAttachmentName(dir, name string) string {
	namePath, pathErr := safeAttachmentPath(dir, name)
	if pathErr != nil {
		return ""
	}
	if _, err := os.Stat(namePath); err == nil || !errors.Is(err, os.ErrNotExist) {
		// An existing file (or an error other than not-exist) must not be
		// overwritten. Continue with a deterministic suffix; the final rename
		// remains atomic and therefore safe against concurrent materializers.
	} else {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; i <= 10000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		candidatePath, pathErr := safeAttachmentPath(dir, candidate)
		if pathErr != nil {
			return ""
		}
		if _, err := os.Stat(candidatePath); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
}
