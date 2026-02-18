package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/codex"
	"go.uber.org/zap"
)

// handleItemStarted handles item/started notifications.
func (a *Adapter) handleItemStarted(params json.RawMessage, threadID, turnID string) {
	var p codex.ItemStartedParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse item started", zap.Error(err))
		return
	}
	if p.Item == nil {
		return
	}
	// Map Codex item types to tool call updates
	// Item types: "userMessage", "agentMessage", "commandExecution", "fileChange", "reasoning", "mcpToolCall"
	switch p.Item.Type {
	case CodexItemCommandExecution:
		args := map[string]any{"command": p.Item.Command, "cwd": p.Item.Cwd}
		normalizedPayload := a.normalizer.NormalizeToolCall(CodexItemCommandExecution, args)
		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeToolCall,
			SessionID:         threadID,
			OperationID:       turnID,
			ToolCallID:        p.Item.ID,
			ToolName:          CodexItemCommandExecution,
			ToolTitle:         p.Item.Command,
			ToolStatus:        "running",
			NormalizedPayload: normalizedPayload,
		})
	case CodexItemFileChange:
		a.sendFileChangeStarted(p.Item, threadID, turnID)
	case CodexItemMcpToolCall:
		a.sendMcpToolCallStarted(p.Item, threadID, turnID)
	}
}

// sendFileChangeStarted emits a tool_call event for a file change item.
func (a *Adapter) sendFileChangeStarted(item *codex.Item, threadID, turnID string) {
	var title string
	if len(item.Changes) > 0 {
		title = item.Changes[0].Path
		if len(item.Changes) > 1 {
			title += fmt.Sprintf(" (+%d more)", len(item.Changes)-1)
		}
	}
	changesArgs := make([]any, 0, len(item.Changes))
	for _, c := range item.Changes {
		changesArgs = append(changesArgs, map[string]any{"path": c.Path, "diff": c.Diff})
	}
	args := map[string]any{"changes": changesArgs}
	normalizedPayload := a.normalizer.NormalizeToolCall(CodexItemFileChange, args)
	a.sendUpdate(AgentEvent{
		Type:              streams.EventTypeToolCall,
		SessionID:         threadID,
		OperationID:       turnID,
		ToolCallID:        item.ID,
		ToolName:          CodexItemFileChange,
		ToolTitle:         title,
		ToolStatus:        "running",
		NormalizedPayload: normalizedPayload,
	})
}

// sendMcpToolCallStarted emits a tool_call event for an MCP tool call item.
func (a *Adapter) sendMcpToolCallStarted(item *codex.Item, threadID, turnID string) {
	title := item.Tool
	if item.Server != "" {
		title = item.Server + "/" + item.Tool
	}
	var argsMap map[string]any
	if len(item.Arguments) > 0 {
		_ = json.Unmarshal(item.Arguments, &argsMap)
	}
	args := map[string]any{"server": item.Server, "tool": item.Tool, "arguments": argsMap}
	normalizedPayload := a.normalizer.NormalizeToolCall(CodexItemMcpToolCall, args)
	a.sendUpdate(AgentEvent{
		Type:              streams.EventTypeToolCall,
		SessionID:         threadID,
		OperationID:       turnID,
		ToolCallID:        item.ID,
		ToolName:          item.Tool, // Use the actual MCP tool name for frontend display
		ToolTitle:         title,
		ToolStatus:        "running",
		NormalizedPayload: normalizedPayload,
	})
}

// handleItemCompleted handles item/completed notifications.
func (a *Adapter) handleItemCompleted(params json.RawMessage, threadID, turnID string) {
	var p codex.ItemCompletedParams
	if err := json.Unmarshal(params, &p); err != nil {
		a.logger.Warn("failed to parse item completed", zap.Error(err))
		return
	}
	if p.Item == nil {
		return
	}
	// Only send updates for tool-like items
	if p.Item.Type != CodexItemCommandExecution && p.Item.Type != CodexItemFileChange && p.Item.Type != CodexItemMcpToolCall {
		return
	}

	status := "complete"
	if p.Item.Status == "failed" {
		status = "error"
	}
	update := AgentEvent{
		Type:        streams.EventTypeToolUpdate,
		SessionID:   threadID,
		OperationID: turnID,
		ToolCallID:  p.Item.ID,
		ToolStatus:  status,
	}

	// Include normalized payload for fallback message creation
	// This ensures the correct message type is used if the message doesn't exist yet
	a.attachCompletedItemPayload(&update, p.Item)
	a.sendUpdate(update)
}

// attachCompletedItemPayload sets the NormalizedPayload and Diff fields on a completed item event.
func (a *Adapter) attachCompletedItemPayload(update *AgentEvent, item *codex.Item) {
	switch item.Type {
	case CodexItemCommandExecution:
		args := map[string]any{"command": item.Command, "cwd": item.Cwd}
		update.NormalizedPayload = a.normalizer.NormalizeToolCall(CodexItemCommandExecution, args)
	case CodexItemFileChange:
		changesArgs := make([]any, 0, len(item.Changes))
		for _, c := range item.Changes {
			changesArgs = append(changesArgs, map[string]any{"path": c.Path, "diff": c.Diff})
		}
		args := map[string]any{"changes": changesArgs}
		update.NormalizedPayload = a.normalizer.NormalizeToolCall(CodexItemFileChange, args)
		// Include diff for file changes
		if len(item.Changes) > 0 {
			var diffs []string
			for _, c := range item.Changes {
				if c.Diff != "" {
					diffs = append(diffs, c.Diff)
				}
			}
			if len(diffs) > 0 {
				update.Diff = strings.Join(diffs, "\n")
			}
		}
	case CodexItemMcpToolCall:
		var argsMap map[string]any
		if len(item.Arguments) > 0 {
			_ = json.Unmarshal(item.Arguments, &argsMap)
		}
		var resultMap any
		if len(item.Result) > 0 {
			_ = json.Unmarshal(item.Result, &resultMap)
		}
		args := map[string]any{
			"server":    item.Server,
			"tool":      item.Tool,
			"arguments": argsMap,
			"result":    resultMap,
		}
		if item.ToolError != "" {
			args["error"] = item.ToolError
		}
		update.NormalizedPayload = a.normalizer.NormalizeToolCall(CodexItemMcpToolCall, args)
		update.ToolName = item.Tool // Use the actual MCP tool name
	}
}
