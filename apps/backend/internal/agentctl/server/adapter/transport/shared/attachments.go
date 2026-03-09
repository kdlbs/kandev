package shared

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// SavedAttachment represents an attachment saved to the workspace.
type SavedAttachment struct {
	// RelPath is the path relative to workDir (e.g. ".kandev/attachments/{sid}/file.pdf").
	RelPath string
	// AbsPath is the absolute path to the saved file.
	AbsPath string
	// Name is the original filename.
	Name string
	// MimeType is the MIME type of the attachment.
	MimeType string
	// Type is the attachment type ("image" or "resource").
	Type string
}

// AttachmentManager saves attachments to session-scoped directories and cleans up.
// Each session gets its own subdirectory under .kandev/attachments/{sessionID}/
// so concurrent agents sharing a workspace don't interfere with each other.
type AttachmentManager struct {
	workDir   string
	sessionID string
	logger    *zap.Logger
}

// NewAttachmentManager creates a new AttachmentManager.
// sessionID can be empty initially and set later via SetSessionID.
func NewAttachmentManager(workDir string, logger *zap.Logger) *AttachmentManager {
	return &AttachmentManager{
		workDir: workDir,
		logger:  logger,
	}
}

// SetSessionID updates the session ID used for the scoped subdirectory.
func (m *AttachmentManager) SetSessionID(id string) {
	m.sessionID = id
}

// SaveAttachments saves all attachments to .kandev/attachments/{sessionID}/.
// Returns saved attachment metadata so the caller can decide how to reference them.
func (m *AttachmentManager) SaveAttachments(attachments []v1.MessageAttachment) ([]SavedAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	if m.workDir == "" || m.sessionID == "" {
		return nil, fmt.Errorf("workDir or sessionID not set")
	}

	dir := filepath.Join(m.workDir, ".kandev", "attachments", m.sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create attachments dir: %w", err)
	}

	var saved []SavedAttachment
	for _, att := range attachments {
		name := att.Name
		if name == "" {
			name = m.generateName(att)
		}
		// Sanitize: strip directory components to prevent path traversal attacks.
		name = filepath.Base(name)
		if name == "." || name == ".." || name == string(filepath.Separator) {
			m.logger.Warn("skipping attachment with invalid name", zap.String("original_name", att.Name))
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			m.logger.Warn("failed to decode attachment", zap.String("name", name), zap.Error(err))
			continue
		}

		absPath := filepath.Join(dir, name)
		if err := os.WriteFile(absPath, decoded, 0o644); err != nil {
			m.logger.Warn("failed to write attachment", zap.String("path", absPath), zap.Error(err))
			continue
		}

		relPath := filepath.Join(".kandev", "attachments", m.sessionID, name)
		saved = append(saved, SavedAttachment{
			RelPath:  relPath,
			AbsPath:  absPath,
			Name:     name,
			MimeType: att.MimeType,
			Type:     att.Type,
		})

		m.logger.Debug("saved attachment", zap.String("path", relPath), zap.Int("size", len(decoded)))
	}

	return saved, nil
}

// Cleanup removes the session's attachment directory.
// Safe to call multiple times or with empty sessionID (no-op).
func (m *AttachmentManager) Cleanup() {
	if m.sessionID == "" || m.workDir == "" {
		return
	}
	dir := filepath.Join(m.workDir, ".kandev", "attachments", m.sessionID)
	if err := os.RemoveAll(dir); err != nil {
		m.logger.Debug("failed to clean attachments dir", zap.String("dir", dir), zap.Error(err))
	}
}

// BuildAttachmentPrompt generates prompt text referencing saved attachment files.
// Used by adapters that don't support native multimodal content.
func BuildAttachmentPrompt(saved []SavedAttachment) string {
	if len(saved) == 0 {
		return ""
	}

	var sb strings.Builder
	if len(saved) == 1 {
		s := saved[0]
		fmt.Fprintf(&sb, "The user attached a file: %s (saved to %s in the workspace). Use your file reading tools to access it.\n\n", s.Name, s.RelPath)
	} else {
		sb.WriteString("The user attached files that you should read and analyze:\n")
		for _, s := range saved {
			fmt.Fprintf(&sb, "- %s (saved to %s)\n", s.Name, s.RelPath)
		}
		sb.WriteString("\nUse your file reading tools to access them.\n\n")
	}
	return sb.String()
}

// generateName creates a filename for attachments that don't have a Name field.
func (m *AttachmentManager) generateName(att v1.MessageAttachment) string {
	ext := extensionFromMimeType(att.MimeType)
	return "attachment" + ext
}

// extensionFromMimeType returns a file extension for common MIME types.
func extensionFromMimeType(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "application/json":
		return ".json"
	case "text/csv":
		return ".csv"
	case "text/html":
		return ".html"
	case "text/markdown":
		return ".md"
	default:
		return ""
	}
}
