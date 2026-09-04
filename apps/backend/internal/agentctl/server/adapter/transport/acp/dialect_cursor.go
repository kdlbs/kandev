package acp

import (
	"encoding/json"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	cursorRetriableStreamResetPrefix  = "Error: RetriableError:"
	cursorRetriableStreamResetMessage = "Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)"
	cursorRetriableStreamResetMaxTail = 256
)

// isCursorRetriableStreamReset recognizes Cursor's complete transport control
// chunk. The prefix check keeps the common per-token path allocation-free;
// requiring the complete diagnostic prevents ordinary provider prose from
// combining the control prefix with an unrelated transport fragment.
func isCursorRetriableStreamReset(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < len(cursorRetriableStreamResetPrefix) ||
		!strings.EqualFold(trimmed[:len(cursorRetriableStreamResetPrefix)], cursorRetriableStreamResetPrefix) {
		return false
	}
	if len(trimmed)-len(cursorRetriableStreamResetPrefix) > cursorRetriableStreamResetMaxTail {
		return false
	}
	return strings.EqualFold(trimmed, cursorRetriableStreamResetMessage) ||
		strings.EqualFold(trimmed, cursorRetriableStreamResetMessage+" [canceled]")
}

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
