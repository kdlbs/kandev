package acp

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type cursorTaskMeta struct {
	ToolCallID  string
	AgentID     string
	Description string
	Model       string
	Prompt      string
}

func parseCursorTaskParams(params json.RawMessage) cursorTaskMeta {
	if len(params) == 0 {
		return cursorTaskMeta{}
	}
	var raw map[string]any
	if err := json.Unmarshal(params, &raw); err != nil {
		return cursorTaskMeta{}
	}
	return cursorTaskMeta{
		ToolCallID:  stringFromAnyMap(raw, "toolCallId"),
		AgentID:     stringFromAnyMap(raw, "agentId"),
		Description: stringFromAnyMap(raw, "description"),
		Model:       stringFromAnyMap(raw, "model"),
		Prompt:      stringFromAnyMap(raw, "prompt"),
	}
}

func mergeCursorTaskMeta(base, update cursorTaskMeta) cursorTaskMeta {
	if base.ToolCallID == "" {
		base.ToolCallID = update.ToolCallID
	}
	if base.AgentID == "" {
		base.AgentID = update.AgentID
	}
	if base.Description == "" {
		base.Description = update.Description
	}
	if base.Model == "" {
		base.Model = update.Model
	}
	if base.Prompt == "" {
		base.Prompt = update.Prompt
	}
	return base
}

func applyCursorTaskMeta(payload *streams.SubagentTaskPayload, meta cursorTaskMeta) {
	if payload == nil {
		return
	}
	fillIfEmpty(&payload.Description, meta.Description)
	fillIfEmpty(&payload.Prompt, meta.Prompt)
	fillIfEmpty(&payload.Model, meta.Model)
	fillIfEmpty(&payload.AgentID, meta.AgentID)
}

func stringFromAnyMap(raw map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	value, _ := raw[key].(string)
	return value
}
