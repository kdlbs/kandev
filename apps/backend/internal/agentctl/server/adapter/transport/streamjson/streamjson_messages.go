package streamjson

import (
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/claudecode"
	"go.uber.org/zap"
)

// handleSystemMessage processes system messages.
// Subtypes:
//   - "init" (or empty): Session initialization with slash commands and status
//   - "task_started": Sub-agent task lifecycle event — logged and skipped
//
// Note: Turn completion is signaled by result messages, not system messages.
func (a *Adapter) handleSystemMessage(msg *claudecode.CLIMessage) {
	// Skip non-init system messages (e.g. task_started for sub-agent lifecycle)
	if msg.Subtype != "" && msg.Subtype != "init" {
		a.logger.Debug("skipping system message with subtype",
			zap.String("subtype", msg.Subtype),
			zap.String("session_id", msg.SessionID))
		return
	}

	a.logger.Info("received system message",
		zap.String("session_id", msg.SessionID),
		zap.String("status", msg.SessionStatus),
		zap.Int("slash_commands_count", len(msg.SlashCommands)))

	// Note: session ID is updated centrally in handleMessage() for all message types.
	a.mu.Lock()
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

	// Track assistant message UUID (committed on result)
	if msg.UUID != "" {
		a.mu.Lock()
		a.pendingAssistantUUID = msg.UUID
		a.mu.Unlock()
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

// handleRateLimitMessage processes rate limit notifications from the Anthropic API.
func (a *Adapter) handleRateLimitMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	message := "Rate limited by API"
	if len(msg.RateLimitInfo) > 0 {
		message = string(msg.RateLimitInfo)
	}

	a.logger.Warn("rate limit event received",
		zap.String("session_id", sessionID),
		zap.String("message", message))

	a.sendUpdate(AgentEvent{
		Type:             streams.EventTypeRateLimit,
		SessionID:        sessionID,
		OperationID:      operationID,
		RateLimitMessage: message,
	})
}

// handleUserMessage processes user messages containing tool results.
// Claude Code sends tool results back as user messages with tool_result content blocks.
//
// User message variants (from Claude Code CLI):
//   - isReplay=true: Historical context echoed on resume — drop entirely
//   - String content: Echoed user prompt or slash command output — skip
//     (slash command output is delivered via the result message instead)
//   - Content blocks with tool_result: Tool results — process normally
func (a *Adapter) handleUserMessage(msg *claudecode.CLIMessage, sessionID, operationID string) {
	if msg.Message == nil {
		return
	}

	// Clear pending assistant UUID — a new user message means any previous
	// assistant message's UUID should not be committed on the next result.
	// User messages are immediately safe to reference for resume.
	a.mu.Lock()
	a.pendingAssistantUUID = ""
	if msg.UUID != "" {
		a.lastMessageUUID = msg.UUID
	}
	a.mu.Unlock()

	// Skip replay messages — historical context echoed on resume.
	// UUID tracking above still fires for --resume-session-at support.
	if msg.IsReplay {
		a.logger.Debug("skipping replay user message",
			zap.String("session_id", sessionID),
			zap.String("uuid", msg.UUID))
		return
	}

	// Skip string-content user messages (echoed prompts).
	// Slash command output also arrives here but is always isReplay=true (handled above).
	// The output is delivered via the result message instead.
	if contentStr := msg.Message.GetContentString(); contentStr != "" {
		a.logger.Debug("skipping echoed user message",
			zap.String("session_id", sessionID),
			zap.Int("content_length", len(contentStr)))
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
		contentStr := block.GetContentString()
		if payload != nil && contentStr != "" {
			a.normalizer.NormalizeToolResult(payload, contentStr)
		}

		// Enrich payload with structured tool_use_result metadata
		if payload != nil && len(msg.ToolUseResult) > 0 {
			a.enrichFromToolUseResult(payload, msg.ToolUseResult)
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

// enrichFromToolUseResult populates payload-specific result fields from the
// top-level tool_use_result JSON on user messages.
//
// Supported formats:
//   - TodoWrite: {oldTodos, newTodos} → populates ManageTodos items from newTodos
//   - Task (sub-agent): {status, agentId, totalDurationMs, totalTokens, totalToolUseCount}
func (a *Adapter) enrichFromToolUseResult(payload *streams.NormalizedPayload, raw json.RawMessage) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}

	switch payload.Kind() {
	case streams.ToolKindManageTodos:
		a.enrichTodoResult(payload, data)
	case streams.ToolKindSubagentTask:
		a.enrichSubagentResult(payload, data)
	}
}

// enrichTodoResult populates ManageTodos items from the newTodos array in tool_use_result.
func (a *Adapter) enrichTodoResult(payload *streams.NormalizedPayload, data map[string]any) {
	if payload.ManageTodos() == nil {
		return
	}
	newTodos, ok := data["newTodos"].([]any)
	if !ok || len(newTodos) == 0 {
		return
	}
	items := make([]streams.TodoItem, 0, len(newTodos))
	for _, item := range newTodos {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		desc := shared.GetString(itemMap, "content")
		if desc == "" {
			desc = shared.GetString(itemMap, "description")
		}
		items = append(items, streams.TodoItem{
			ID:          shared.GetString(itemMap, "id"),
			Description: desc,
			Status:      shared.GetString(itemMap, "status"),
			ActiveForm:  shared.GetString(itemMap, "activeForm"),
		})
	}
	if len(items) > 0 {
		payload.ManageTodos().Items = items
	}
}

// enrichSubagentResult populates SubagentTask result fields from tool_use_result.
func (a *Adapter) enrichSubagentResult(payload *streams.NormalizedPayload, data map[string]any) {
	if payload.SubagentTask() == nil {
		return
	}
	st := payload.SubagentTask()
	if v := shared.GetString(data, "status"); v != "" {
		st.Status = v
	}
	if v := shared.GetString(data, "agentId"); v != "" {
		st.AgentID = v
	}
	if v, ok := data["totalDurationMs"].(float64); ok {
		st.DurationMs = int64(v)
	}
	if v, ok := data["totalTokens"].(float64); ok {
		st.TotalTokens = int64(v)
	}
	if v, ok := data["totalToolUseCount"].(float64); ok {
		st.ToolUseCount = int(v)
	}
}
