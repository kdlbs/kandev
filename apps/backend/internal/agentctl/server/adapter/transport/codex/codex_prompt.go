package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/codex"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// Prompt sends a prompt to the agent, starting a new turn.
// This method blocks until the turn completes (turn/completed notification received).
func (a *Adapter) Prompt(ctx context.Context, message string, attachments []v1.MessageAttachment) error {
	a.mu.Lock()
	client := a.client
	threadID := a.threadID
	// Reset accumulators for new turn
	a.messageBuffer = ""
	a.reasoningBuffer = ""
	a.currentReasoningItemID = ""
	// Create channel to wait for turn completion
	a.turnCompleteCh = make(chan turnCompleteResult, 1)
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	inputs, err := a.buildPromptInputs(message, attachments)
	if err != nil {
		return err
	}

	// Start prompt span — notification spans become children via getPromptTraceCtx()
	promptCtx, promptSpan := shared.TraceProtocolRequest(ctx, shared.ProtocolCodex, a.agentID, "prompt")
	promptSpan.SetAttributes(
		attribute.String("session_id", threadID),
		attribute.Int("prompt_inputs", len(inputs)),
	)
	a.setPromptTraceCtx(promptCtx)
	defer func() {
		a.clearPromptTraceCtx()
		promptSpan.End()
	}()

	a.logger.Info("sending prompt",
		zap.String("thread_id", threadID),
		zap.Int("inputs", len(inputs)),
		zap.Int("image_attachments", len(attachments)))

	turnID, completeCh, err := a.startTurn(ctx, client, threadID, inputs)
	if err != nil {
		return err
	}

	return a.waitForTurnCompletion(ctx, threadID, turnID, completeCh)
}

// buildPromptInputs constructs the list of UserInput items from message text and attachments.
func (a *Adapter) buildPromptInputs(message string, attachments []v1.MessageAttachment) ([]codex.UserInput, error) {
	inputs := make([]codex.UserInput, 0, 1+len(attachments))
	if strings.TrimSpace(message) != "" {
		inputs = append(inputs, codex.UserInput{Type: "text", Text: message})
	}

	if len(attachments) > 0 {
		imagePaths, err := a.saveImageAttachments(a.cfg.WorkDir, attachments)
		if err != nil {
			a.logger.Warn("failed to save image attachments", zap.Error(err))
		} else {
			for _, imagePath := range imagePaths {
				inputs = append(inputs, codex.UserInput{Type: "localImage", Path: imagePath})
			}
		}
	}

	if len(inputs) == 0 {
		return nil, fmt.Errorf("prompt requires message text or attachments")
	}
	return inputs, nil
}

// startTurn calls turn/start and returns the turn ID and completion channel.
func (a *Adapter) startTurn(ctx context.Context, client *codex.Client, threadID string, inputs []codex.UserInput) (string, chan turnCompleteResult, error) {
	params := &codex.TurnStartParams{
		ThreadID: threadID,
		Input:    inputs,
	}

	resp, err := client.Call(ctx, codex.MethodTurnStart, params)
	if err != nil {
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		return "", nil, fmt.Errorf("failed to start turn: %w", err)
	}

	if resp.Error != nil {
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		return "", nil, fmt.Errorf("turn start error: %s", resp.Error.Message)
	}

	var result codex.TurnStartResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		a.logger.Warn("failed to parse turn start result", zap.Error(err), zap.String("raw", string(resp.Result)))
	}

	turnID := ""
	if result.Turn != nil {
		turnID = result.Turn.ID
	}

	a.mu.Lock()
	a.turnID = turnID
	completeCh := a.turnCompleteCh
	a.mu.Unlock()

	if result.Turn != nil {
		a.logger.Info("started turn, waiting for completion", zap.String("turn_id", turnID), zap.String("status", result.Turn.Status))
	} else {
		a.logger.Info("started turn, waiting for completion", zap.String("turn_id", turnID))
	}

	return turnID, completeCh, nil
}

// waitForTurnCompletion blocks until the turn completes or context is cancelled.
func (a *Adapter) waitForTurnCompletion(ctx context.Context, threadID, turnID string, completeCh chan turnCompleteResult) error {
	select {
	case <-ctx.Done():
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		return ctx.Err()
	case completeResult := <-completeCh:
		a.mu.Lock()
		a.turnCompleteCh = nil
		a.mu.Unlock()
		if !completeResult.success && completeResult.err != "" {
			return fmt.Errorf("turn failed: %s", completeResult.err)
		}
		a.logger.Info("turn completed", zap.String("turn_id", turnID), zap.Bool("success", completeResult.success))

		// Emit complete event via the stream.
		// This normalizes Codex behavior to match other adapters.
		// All adapters now emit complete events, eliminating the need for protocol-specific flags.
		a.sendUpdate(AgentEvent{
			Type:        streams.EventTypeComplete,
			SessionID:   threadID,
			OperationID: turnID,
		})

		return nil
	}
}

// saveImageAttachments saves image attachments to temp files in the workspace.
func (a *Adapter) saveImageAttachments(workDir string, attachments []v1.MessageAttachment) ([]string, error) {
	var imagePaths []string

	if workDir == "" {
		return nil, fmt.Errorf("workDir is required to save attachments")
	}

	tempDir := filepath.Join(workDir, ".kandev", "temp", "images")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	for _, att := range attachments {
		if att.Type != "image" {
			continue
		}

		imageData, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			a.logger.Warn("failed to decode image attachment", zap.Error(err))
			continue
		}

		ext := ".png"
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

		filename := fmt.Sprintf("image-%s%s", uuid.New().String()[:8], ext)
		filePath := filepath.Join(tempDir, filename)
		if err := os.WriteFile(filePath, imageData, 0644); err != nil {
			a.logger.Warn("failed to write image file", zap.Error(err), zap.String("path", filePath))
			continue
		}

		imagePaths = append(imagePaths, filePath)
		a.logger.Info("saved image attachment",
			zap.String("path", filePath),
			zap.Int("size", len(imageData)))
	}

	return imagePaths, nil
}

// Cancel interrupts the current turn.
func (a *Adapter) Cancel(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	threadID := a.threadID
	turnID := a.turnID
	a.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	a.logger.Info("cancelling turn", zap.String("thread_id", threadID), zap.String("turn_id", turnID))

	// Codex uses turn/interrupt to cancel
	_, err := client.Call(ctx, codex.MethodTurnInterrupt, map[string]string{
		"threadId": threadID,
		"turnId":   turnID,
	})
	return err
}
