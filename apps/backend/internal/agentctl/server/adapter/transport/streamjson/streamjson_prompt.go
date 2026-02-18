package streamjson

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// Prompt sends a prompt and waits for completion.
// Since stream-json protocol doesn't support multimodal content blocks, image attachments
// are saved to temp files in the workspace and the prompt is modified to reference them.
// Claude Code can then read these files with its Read tool.
func (a *Adapter) Prompt(ctx context.Context, message string, attachments []v1.MessageAttachment) error {
	a.mu.Lock()
	client := a.client
	sessionID := a.sessionID
	operationID := uuid.New().String()
	a.operationID = operationID
	workDir := a.cfg.WorkDir
	// Create channel to wait for result
	a.resultCh = make(chan resultComplete, 1)
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Handle image attachments by saving to temp files
	finalMessage := message
	if len(attachments) > 0 {
		imagePaths, err := a.saveImageAttachments(workDir, attachments)
		if err != nil {
			a.logger.Warn("failed to save image attachments", zap.Error(err))
		} else if len(imagePaths) > 0 {
			finalMessage = a.buildMessageWithImages(message, imagePaths)
		}
	}

	a.logger.Info("sending prompt",
		zap.String("session_id", sessionID),
		zap.String("operation_id", operationID),
		zap.Int("attachments", len(attachments)))

	// Send user message
	if err := client.SendUserMessage(finalMessage); err != nil {
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		return fmt.Errorf("failed to send user message: %w", err)
	}

	// Wait for result or context cancellation
	a.mu.RLock()
	resultCh := a.resultCh
	a.mu.RUnlock()

	select {
	case <-ctx.Done():
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		return ctx.Err()
	case result := <-resultCh:
		a.mu.Lock()
		a.resultCh = nil
		a.mu.Unlock()
		if !result.success && result.err != "" {
			return fmt.Errorf("prompt failed: %s", result.err)
		}
		a.logger.Info("prompt completed",
			zap.String("operation_id", operationID),
			zap.Bool("success", result.success))
		return nil
	}
}

// saveImageAttachments saves image attachments to temp files in the workspace.
// Returns the relative paths to the saved files.
func (a *Adapter) saveImageAttachments(workDir string, attachments []v1.MessageAttachment) ([]string, error) {
	var imagePaths []string

	// Create temp directory for images
	tempDir := filepath.Join(workDir, ".kandev", "temp", "images")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	for _, att := range attachments {
		if att.Type != "image" {
			continue
		}

		// Decode base64 data
		imageData, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			a.logger.Warn("failed to decode image attachment", zap.Error(err))
			continue
		}

		// Determine file extension from mime type
		ext := ".png" // default
		switch att.MimeType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		case "image/png":
			ext = ".png"
		}

		// Generate unique filename
		filename := fmt.Sprintf("image-%s%s", uuid.New().String()[:8], ext)
		filePath := filepath.Join(tempDir, filename)

		// Write image to file
		if err := os.WriteFile(filePath, imageData, 0644); err != nil {
			a.logger.Warn("failed to write image file", zap.Error(err), zap.String("path", filePath))
			continue
		}

		// Store relative path from workspace root
		relPath, err := filepath.Rel(workDir, filePath)
		if err != nil {
			relPath = filePath // Fall back to absolute path
		}

		imagePaths = append(imagePaths, relPath)
		a.logger.Info("saved image attachment",
			zap.String("path", relPath),
			zap.Int("size", len(imageData)))
	}

	return imagePaths, nil
}

// buildMessageWithImages prepends image file references to the user's message.
func (a *Adapter) buildMessageWithImages(message string, imagePaths []string) string {
	var sb strings.Builder

	// Add image references at the beginning
	if len(imagePaths) == 1 {
		sb.WriteString(fmt.Sprintf("I've attached an image file that you should read and analyze: %s\n\n", imagePaths[0]))
	} else {
		sb.WriteString("I've attached image files that you should read and analyze:\n")
		for _, path := range imagePaths {
			sb.WriteString(fmt.Sprintf("- %s\n", path))
		}
		sb.WriteString("\n")
	}

	// Add the original message
	sb.WriteString(message)

	return sb.String()
}

// Cancel interrupts the current operation.
func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling operation")

	// Send interrupt control request
	return client.SendControlRequest(&claudecode.SDKControlRequest{
		Type:      claudecode.MessageTypeControlRequest,
		RequestID: uuid.New().String(),
		Request: claudecode.SDKControlRequestBody{
			Subtype: claudecode.SubtypeInterrupt,
		},
	})
}
