package acp

import (
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestParseCursorTaskParams(t *testing.T) {
	tests := []struct {
		name   string
		params string
		want   cursorTaskMeta
	}{
		{
			name: "full payload ignores object subagentType",
			params: `{"agentId":"agent-1","description":"Summarize JS","model":"composer-2.5-fast",` +
				`"prompt":"Summarize src JS files","subagentType":{"custom":{"unspecified":{}}},"toolCallId":"tool-1"}`,
			want: cursorTaskMeta{
				ToolCallID:  "tool-1",
				AgentID:     "agent-1",
				Description: "Summarize JS",
				Model:       "composer-2.5-fast",
				Prompt:      "Summarize src JS files",
			},
		},
		{
			name:   "missing fields stay empty",
			params: `{"toolCallId":"tool-2","prompt":"Only prompt"}`,
			want:   cursorTaskMeta{ToolCallID: "tool-2", Prompt: "Only prompt"},
		},
		{
			name:   "invalid json tolerated",
			params: `{`,
			want:   cursorTaskMeta{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseCursorTaskParams(json.RawMessage(tt.params)); got != tt.want {
				t.Fatalf("parseCursorTaskParams() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCursorTaskRequestBeforeToolCallMergesOnCreate(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "session-1"
	a.handleCursorTask(json.RawMessage(`{"toolCallId":"tool-1","agentId":"agent-1","description":"Summarize JS","model":"composer-2.5-fast","prompt":"Summarize src JS files"}`))

	event := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		ToolCallId: "tool-1",
		Kind:       "other",
		Title:      "Task: Subagent task",
		Status:     toolStatusInProgress,
		RawInput:   map[string]any{"_toolName": "task"},
	})
	if event == nil || event.Type != streams.EventTypeToolCall {
		t.Fatalf("event = %+v, want initial tool_call", event)
	}
	sa := event.NormalizedPayload.SubagentTask()
	if sa.Description != "Summarize JS" || sa.Prompt != "Summarize src JS files" || sa.Model != "composer-2.5-fast" || sa.AgentID != "agent-1" {
		t.Fatalf("subagent payload = %+v", sa)
	}
	if pending := len(a.cursorTaskMetaBySession["session-1"]); pending != 0 {
		t.Fatalf("pending cursor/task entries = %d, want 0 after match", pending)
	}
}

func TestCursorTaskRequestAfterToolCallEmitsUpdate(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "session-1"
	start := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		ToolCallId: "tool-1",
		Kind:       "other",
		Title:      "Task: Subagent task",
		Status:     toolStatusInProgress,
		RawInput:   map[string]any{"_toolName": "task"},
	})
	if start == nil || start.Type != streams.EventTypeToolCall {
		t.Fatalf("start event = %+v", start)
	}

	a.handleCursorTask(json.RawMessage(`{"toolCallId":"tool-1","description":"Summarize JS","model":"composer-2.5-fast","prompt":"Summarize src JS files"}`))
	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("drained events = %d, want 1 metadata update", len(events))
	}
	update := events[0]
	if update.Type != streams.EventTypeToolUpdate || update.ToolCallID != "tool-1" || update.ToolStatus != toolStatusInProgress {
		t.Fatalf("update event = %+v", update)
	}
	sa := update.NormalizedPayload.SubagentTask()
	if sa.Description != "Summarize JS" || sa.Prompt != "Summarize src JS files" || sa.Model != "composer-2.5-fast" {
		t.Fatalf("updated payload = %+v", sa)
	}
}

func TestCursorTaskUnmatchedMetaClearedAtPromptEnd(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "session-1"

	// An unmatched cursor/task arrives (no subagent tool_call this turn).
	a.handleCursorTask(json.RawMessage(`{"toolCallId":"tool-1","description":"Stale","prompt":"stale prompt"}`))
	if pending := len(a.cursorTaskMetaBySession["session-1"]); pending != 1 {
		t.Fatalf("pending cursor/task entries = %d, want 1 before prompt end", pending)
	}

	a.sweepCursorTaskMetaOnPromptEnd("session-1")
	if pending := len(a.cursorTaskMetaBySession["session-1"]); pending != 0 {
		t.Fatalf("pending cursor/task entries = %d, want 0 after prompt end", pending)
	}

	// A later turn reuses tool-1 for an unrelated subagent. Without the
	// prompt-end sweep the stale "Stale" metadata would attach here.
	event := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		ToolCallId: "tool-1",
		Kind:       "other",
		Title:      "Task: Subagent task",
		Status:     toolStatusInProgress,
		RawInput:   map[string]any{"_toolName": "task"},
	})
	if event == nil || event.Type != streams.EventTypeToolCall {
		t.Fatalf("event = %+v, want initial tool_call", event)
	}
	if sa := event.NormalizedPayload.SubagentTask(); sa.Description == "Stale" || sa.Prompt == "stale prompt" {
		t.Fatalf("stale cursor/task metadata leaked into later turn: %+v", sa)
	}
}

func TestCursorTaskCleanupDropsUnmatchedSessionState(t *testing.T) {
	a := newTestAdapter()
	a.mu.Lock()
	a.storeCursorTaskMetaLocked("session-1", cursorTaskMeta{ToolCallID: "tool-1", Prompt: "queued"})
	a.storeCursorTaskMetaLocked("session-2", cursorTaskMeta{ToolCallID: "tool-2", Prompt: "other"})
	a.clearCursorTaskMetaLocked("session-1")
	_, existsSession1 := a.cursorTaskMetaBySession["session-1"]
	remaining := a.cursorTaskMetaBySession["session-2"]["tool-2"]
	a.mu.Unlock()

	if existsSession1 {
		t.Fatal("session-1 cursor/task cache survived targeted cleanup")
	}
	if remaining.ToolCallID != "tool-2" {
		t.Fatalf("session-2 entry = %+v, want untouched", remaining)
	}
}
