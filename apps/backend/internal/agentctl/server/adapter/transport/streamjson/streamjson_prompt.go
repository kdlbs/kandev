package streamjson

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.opentelemetry.io/otel/attribute"
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
	// Create channel to wait for result
	a.resultCh = make(chan resultComplete, 1)
	a.mu.Unlock()

	if client == nil {
		return fmt.Errorf("adapter not initialized")
	}

	// Handle attachments by saving to workspace and referencing in prompt
	finalMessage := message
	if len(attachments) > 0 {
		saved, err := a.attachMgr.SaveAttachments(attachments)
		if err != nil {
			a.logger.Warn("failed to save attachments", zap.Error(err))
		} else if len(saved) > 0 {
			finalMessage = shared.BuildAttachmentPrompt(saved) + message
		}
	}

	// Start prompt span — notification spans become children via getPromptTraceCtx()
	promptCtx, promptSpan := shared.TraceProtocolRequest(ctx, shared.ProtocolStreamJSON, a.agentID, "prompt")
	promptSpan.SetAttributes(
		attribute.String("session_id", sessionID),
		attribute.Int("prompt_length", len(finalMessage)),
	)
	a.setPromptTraceCtx(promptCtx)
	defer func() {
		a.clearPromptTraceCtx()
		promptSpan.End()
	}()

	// Trace the outgoing user message payload
	a.traceOutgoingSend("prompt", &claudecode.UserMessage{
		Type:    claudecode.MessageTypeUser,
		Message: claudecode.UserMessageBody{Role: "user", Content: finalMessage},
	})

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
		// Clean up saved attachments — agent has finished reading them
		a.attachMgr.Cleanup()
		return nil
	}
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
	req := &claudecode.SDKControlRequest{
		Type:      claudecode.MessageTypeControlRequest,
		RequestID: uuid.New().String(),
		Request: claudecode.SDKControlRequestBody{
			Subtype: claudecode.SubtypeInterrupt,
		},
	}
	a.traceOutgoingControl("interrupt", req)
	return client.SendControlRequest(req)
}
