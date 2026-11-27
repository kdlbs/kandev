package acp

import (
	"encoding/json"
	"reflect"
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

func TestCursorMCPFrameNormalizesIdentityArgumentsAndResult(t *testing.T) {
	a := newTestAdapter()
	a.agentID = "cursor-acp"
	a.normalizer = NewNormalizer(a.agentID)
	a.dialect = newACPDialect(a.agentID)

	arguments := map[string]any{
		"version": float64(1),
		"title":   "Build health",
		"blocks": []any{map[string]any{
			"type": "metrics",
			"items": []any{map[string]any{
				"label": "Passed",
				"value": "38",
			}},
		}},
	}
	rawInput := map[string]any{
		"providerIdentifier": "kandev",
		"toolName":           "show_rich_output_kandev",
		"args":               arguments,
	}

	initial := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		ToolCallId: "cursor-mcp-1",
		Kind:       "other",
		Title:      "kandev: show_rich_output_kandev",
		Status:     toolStatusInProgress,
		RawInput:   rawInput,
	})
	if initial == nil {
		t.Fatal("initial event is nil")
	}
	if got := initial.NormalizedPayload.Generic().Name; got != "kandev/show_rich_output_kandev" {
		t.Fatalf("generic name = %q, want kandev/show_rich_output_kandev", got)
	}
	if !initial.NormalizedPayload.IsMCPTool() {
		t.Fatal("normalized payload is not marked as an MCP tool")
	}
	if got := initial.NormalizedPayload.Generic().Input; !reflect.DeepEqual(got, arguments) {
		t.Fatalf("generic input = %#v, want %#v", got, arguments)
	}

	completed := acpsdk.ToolCallStatus("completed")
	terminal := a.convertToolCallResultUpdate("session-1", &acpsdk.SessionToolCallUpdate{
		ToolCallId: "cursor-mcp-1",
		Status:     &completed,
		RawOutput:  map[string]any{"success": true},
	})
	if terminal == nil {
		t.Fatal("terminal event is nil")
	}
	if got := terminal.NormalizedPayload.Generic().Output; !reflect.DeepEqual(got, map[string]any{"success": true}) {
		t.Fatalf("generic output = %#v, want success result", got)
	}
}

func TestCursorMCPFrameNormalizesForGrokDialect(t *testing.T) {
	a := newTestAdapter()
	a.agentID = grokAgentID
	a.normalizer = NewNormalizer(a.agentID)
	a.dialect = newACPDialect(a.agentID)

	arguments := map[string]any{"version": float64(1), "title": "Build health", "blocks": []any{}}
	event := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
		ToolCallId: "grok-mcp-1",
		Kind:       "other",
		Title:      "kandev: show_rich_output_kandev",
		Status:     toolStatusInProgress,
		RawInput: map[string]any{
			"providerIdentifier": "kandev",
			"toolName":           "show_rich_output_kandev",
			"args":               arguments,
		},
	})
	if event == nil {
		t.Fatal("event is nil")
	}
	if got := event.NormalizedPayload.Generic().Name; got != "kandev/show_rich_output_kandev" {
		t.Fatalf("generic name = %q, want kandev/show_rich_output_kandev", got)
	}
	if got := event.NormalizedPayload.Generic().Input; !reflect.DeepEqual(got, arguments) {
		t.Fatalf("generic input = %#v, want %#v", got, arguments)
	}
}

func TestCursorMCPRecognitionPreservesProviderAndRejectsIncompleteEnvelopes(t *testing.T) {
	t.Run("foreign provider retains its identity", func(t *testing.T) {
		a := newTestAdapter()
		a.agentID = cursorAgentID
		a.normalizer = NewNormalizer(a.agentID)
		a.dialect = newACPDialect(a.agentID)

		event := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
			ToolCallId: "foreign-mcp-1",
			Kind:       "other",
			Status:     toolStatusInProgress,
			RawInput: map[string]any{
				"providerIdentifier": "github",
				"toolName":           "show_rich_output_kandev",
				"args":               map[string]any{},
			},
		})
		if got := event.NormalizedPayload.Generic().Name; got != "github/show_rich_output_kandev" {
			t.Fatalf("generic name = %q, want foreign provider identity", got)
		}
	})

	t.Run("missing object args remains ordinary generic activity", func(t *testing.T) {
		a := newTestAdapter()
		a.agentID = cursorAgentID
		a.normalizer = NewNormalizer(a.agentID)
		a.dialect = newACPDialect(a.agentID)

		event := a.convertToolCallUpdate("session-1", &acpsdk.SessionUpdateToolCall{
			ToolCallId: "generic-other-1",
			Kind:       "other",
			Status:     toolStatusInProgress,
			RawInput: map[string]any{
				"providerIdentifier": "kandev",
				"toolName":           "show_rich_output_kandev",
				"args":               "not-an-object",
			},
		})
		if got := event.NormalizedPayload.Generic().Name; got != "other" {
			t.Fatalf("generic name = %q, want other", got)
		}
		if event.NormalizedPayload.IsMCPTool() {
			t.Fatal("incomplete envelope was marked as an MCP tool")
		}
	})
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
