package streamjson

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// handleSystemMessage processes system init messages.
// Note: System messages are session initialization, NOT turn completion.
// Turn completion is signaled by result messages.
func (a *Adapter) handleSystemMessage(msg *claudecode.CLIMessage) {
	a.logger.Info("received system message",
		zap.String("session_id", msg.SessionID),
		zap.String("status", msg.SessionStatus),
		zap.Int("slash_commands_count", len(msg.SlashCommands)))

	// Update session ID if provided
	a.mu.Lock()
	if msg.SessionID != "" {
		a.sessionID = msg.SessionID
	}
	alreadySent := a.sessionStatusSent
	a.sessionStatusSent = true
	a.mu.Unlock()

	// Emit available commands if present (do this on every system message,
	// not just the first, in case commands change)
	// Note: System message slash_commands is just an array of names (strings),
	// so we only have the name, not description. The initialize response has full details.
	if len(msg.SlashCommands) > 0 {
		commands := make([]streams.AvailableCommand, len(msg.SlashCommands))
		for i, name := range msg.SlashCommands {
			commands[i] = streams.AvailableCommand{
				Name:        name,
				Description: "", // System message only has names, not descriptions
			}
		}
		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeAvailableCommands,
			SessionID:         msg.SessionID,
			AvailableCommands: commands,
		})
	}

	// Only send session status event once per session (on first prompt)
	// The agent sends system messages on every prompt, but we only want to
	// show "New session started" or "Session resumed" once
	if alreadySent {
		return
	}

	// Send session status event (NOT complete - that's only for result messages)
	a.sendUpdate(AgentEvent{
		Type:      streams.EventTypeSessionStatus,
		SessionID: msg.SessionID,
		Data: map[string]any{
			"session_status": msg.SessionStatus,
			"init":           true,
		},
	})
}

// updateMainModel tracks the first model seen as the main model name.
func (a *Adapter) updateMainModel(model string) {
	if model == "" || a.agentInfo == nil {
		return
	}
	a.agentInfo.Version = model
	a.mu.Lock()
	if a.mainModelName == "" {
		a.mainModelName = model
		a.logger.Debug("tracking main model", zap.String("model", model))
	}
	a.mu.Unlock()
}

// emitContextWindow emits a context window event.
func (a *Adapter) emitContextWindow(sessionID, operationID string, contextUsed, contextSize int64) {
	remaining := contextSize - contextUsed
	if remaining < 0 {
		remaining = 0
	}
	a.sendUpdate(AgentEvent{
		Type:                   streams.EventTypeContextWindow,
		SessionID:              sessionID,
		OperationID:            operationID,
		ContextWindowSize:      contextSize,
		ContextWindowUsed:      contextUsed,
		ContextWindowRemaining: remaining,
		ContextEfficiency:      float64(contextUsed) / float64(contextSize) * 100,
	})
}

// processTextBlock handles a text content block from an assistant message.
func (a *Adapter) processTextBlock(block claudecode.ContentBlock, sessionID, operationID string) {
	if block.Text == "" {
		return
	}
	// Mark that we've sent streaming text this turn
	// This prevents duplicate content from result.text
	a.mu.Lock()
	a.streamingTextSentThisTurn = true
	a.mu.Unlock()

	a.sendUpdate(AgentEvent{
		Type:        streams.EventTypeMessageChunk,
		SessionID:   sessionID,
		OperationID: operationID,
		Text:        block.Text,
	})
}

// processToolUseBlock handles a tool_use content block from an assistant message.
func (a *Adapter) processToolUseBlock(block claudecode.ContentBlock, sessionID, operationID, parentToolUseID string) {
	// Generate normalized payload using the normalizer
	normalizedPayload := a.normalizer.NormalizeToolCall(block.Name, block.Input)

	// Detect specific tool operation type for logging
	toolType := DetectStreamJSONToolType(block.Name)

	// Build a human-readable title for the tool call
	toolTitle := block.Name
	if cmd, ok := block.Input["command"].(string); ok && block.Name == claudecode.ToolBash {
		toolTitle = cmd
	} else if path, ok := block.Input["file_path"].(string); ok {
		toolTitle = fmt.Sprintf("%s: %s", block.Name, path)
	}
	a.logger.Debug("tool_use block received",
		zap.String("tool_call_id", block.ID),
		zap.String("tool_name", block.Name),
		zap.String("tool_type", toolType),
		zap.String("title", toolTitle))

	// Track this tool call as pending with its payload for result enrichment
	a.mu.Lock()
	a.pendingToolCalls[block.ID] = normalizedPayload
	a.mu.Unlock()

	a.sendUpdate(AgentEvent{
		Type:              streams.EventTypeToolCall,
		SessionID:         sessionID,
		OperationID:       operationID,
		ToolCallID:        block.ID,
		ParentToolCallID:  parentToolUseID,
		ToolName:          block.Name,
		ToolTitle:         toolTitle,
		ToolStatus:        "running",
		NormalizedPayload: normalizedPayload,
	})
}

// handleAssistantMessage processes assistant messages (text, thinking, tool calls).
func (a *Adapter) handleAssistantMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Extract parent tool use ID for subagent nesting
	parentToolUseID := msg.ParentToolUseID

	// Get content blocks (may be nil if content is a string)
	contentBlocks := msg.Message.GetContentBlocks()

	// Log content block types for debugging
	blockTypes := make([]string, 0, len(contentBlocks))
	for _, block := range contentBlocks {
		blockTypes = append(blockTypes, block.Type)
	}
	a.logger.Debug("processing assistant message",
		zap.Int("num_blocks", len(contentBlocks)),
		zap.Strings("block_types", blockTypes),
		zap.String("parent_tool_use_id", parentToolUseID))

	// Update agent version and track main model name from model info
	a.updateMainModel(msg.Message.Model)

	// Process content blocks
	for _, block := range contentBlocks {
		switch block.Type {
		case "text":
			a.processTextBlock(block, sessionID, operationID)
		case "thinking":
			if block.Thinking != "" {
				a.sendUpdate(AgentEvent{
					Type:          streams.EventTypeReasoning,
					SessionID:     sessionID,
					OperationID:   operationID,
					ReasoningText: block.Thinking,
				})
			}
		case "tool_use":
			a.processToolUseBlock(block, sessionID, operationID, parentToolUseID)
		}
	}

	// Calculate and emit token usage as context window event
	if msg.Message.Usage != nil {
		usage := msg.Message.Usage

		// Calculate total tokens used (including cache tokens)
		contextUsed := usage.InputTokens + usage.OutputTokens +
			usage.CacheCreationInputTokens + usage.CacheReadInputTokens

		// Update tracked token usage
		a.mu.Lock()
		a.contextTokensUsed = contextUsed
		contextSize := a.mainModelContextWindow
		a.mu.Unlock()

		a.emitContextWindow(sessionID, operationID, contextUsed, contextSize)
	}
}

// handleUserMessage processes user messages containing tool results or slash command output.
// Claude Code sends tool results back as user messages with tool_result content blocks.
// For slash commands, content may be a plain string wrapped in <local-command-stdout> tags.
func (a *Adapter) handleUserMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Check if content is a string (slash command output)
	if contentStr := msg.Message.GetContentString(); contentStr != "" {
		// Extract text from <local-command-stdout> tags if present
		text := contentStr
		if strings.HasPrefix(text, "<local-command-stdout>") && strings.HasSuffix(text, "</local-command-stdout>") {
			text = strings.TrimPrefix(text, "<local-command-stdout>")
			text = strings.TrimSuffix(text, "</local-command-stdout>")
		}

		if text != "" {
			a.logger.Info("received user message with string content (slash command output)",
				zap.String("session_id", sessionID),
				zap.Int("content_length", len(text)))

			a.sendUpdate(AgentEvent{
				Type:        streams.EventTypeMessageChunk,
				SessionID:   sessionID,
				OperationID: operationID,
				Text:        text,
			})
		}
		return
	}

	// Process content blocks looking for tool_result
	contentBlocks := msg.Message.GetContentBlocks()
	for _, block := range contentBlocks {
		if block.Type != "tool_result" {
			continue
		}

		// Get and enrich the pending payload with result content
		a.mu.Lock()
		payload := a.pendingToolCalls[block.ToolUseID]
		delete(a.pendingToolCalls, block.ToolUseID)
		a.mu.Unlock()

		// Enrich payload with result content
		if payload != nil && block.Content != "" {
			a.normalizer.NormalizeToolResult(payload, block.Content)
		}

		// If there's an error, set the error flag on the payload
		// This ensures the frontend can display error messages properly
		if payload != nil && block.IsError {
			if payload.HttpRequest() != nil {
				payload.HttpRequest().IsError = true
			}
		}

		// Determine status
		status := "complete"
		if block.IsError {
			status = "error"
		}

		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeToolUpdate,
			SessionID:         sessionID,
			OperationID:       operationID,
			ToolCallID:        block.ToolUseID,
			ToolStatus:        status,
			NormalizedPayload: payload,
		})
	}
}
