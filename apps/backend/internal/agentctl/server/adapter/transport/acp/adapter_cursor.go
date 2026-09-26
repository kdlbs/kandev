package acp

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func (a *Adapter) handleCursorTask(params json.RawMessage) {
	meta := parseCursorTaskParams(params)
	if meta.ToolCallID == "" {
		return
	}

	a.mu.Lock()
	sessionID := a.sessionID
	var emitted *streams.NormalizedPayload
	if active := a.activeToolCalls[meta.ToolCallID]; active != nil && active.Kind() == streams.ToolKindSubagentTask {
		applyCursorTaskMeta(active.SubagentTask(), meta)
		emitted = cloneSubagentPayload(active)
	} else if sessionID != "" {
		a.storeCursorTaskMetaLocked(sessionID, meta)
	}
	a.mu.Unlock()

	if emitted != nil && sessionID != "" {
		a.sendUpdate(AgentEvent{
			Type:              streams.EventTypeToolUpdate,
			SessionID:         sessionID,
			ToolCallID:        meta.ToolCallID,
			ToolStatus:        toolStatusInProgress,
			NormalizedPayload: emitted,
		})
	}
}

func (a *Adapter) applyPendingCursorTaskMetaLocked(sessionID, toolCallID string, payload *streams.NormalizedPayload) {
	if payload == nil || payload.Kind() != streams.ToolKindSubagentTask {
		return
	}
	sessionMeta := a.cursorTaskMetaBySession[sessionID]
	if sessionMeta == nil {
		return
	}
	meta, ok := sessionMeta[toolCallID]
	if !ok {
		return
	}
	applyCursorTaskMeta(payload.SubagentTask(), meta)
	delete(sessionMeta, toolCallID)
	if len(sessionMeta) == 0 {
		delete(a.cursorTaskMetaBySession, sessionID)
	}
}

func (a *Adapter) storeCursorTaskMetaLocked(sessionID string, meta cursorTaskMeta) {
	if sessionID == "" || meta.ToolCallID == "" {
		return
	}
	if a.cursorTaskMetaBySession == nil {
		a.cursorTaskMetaBySession = make(map[string]map[string]cursorTaskMeta)
	}
	if a.cursorTaskMetaBySession[sessionID] == nil {
		a.cursorTaskMetaBySession[sessionID] = make(map[string]cursorTaskMeta)
	}
	stored := a.cursorTaskMetaBySession[sessionID][meta.ToolCallID]
	a.cursorTaskMetaBySession[sessionID][meta.ToolCallID] = mergeCursorTaskMeta(stored, meta)
}

// sweepCursorTaskMetaOnPromptEnd drops any pending cursor/task metadata for the
// session that never matched a subagent tool_call during the turn. The entries
// are keyed by wire tool-call ID, which Cursor can reuse across turns, so an
// unmatched entry left behind would attach to an unrelated subagent later.
func (a *Adapter) sweepCursorTaskMetaOnPromptEnd(sessionID string) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	a.clearCursorTaskMetaLocked(sessionID)
	a.mu.Unlock()
}

func (a *Adapter) clearCursorTaskMetaLocked(sessionID string) {
	if sessionID == "" {
		clear(a.cursorTaskMetaBySession)
		return
	}
	delete(a.cursorTaskMetaBySession, sessionID)
}
