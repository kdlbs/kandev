package streamjson

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/claudecode"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	return log
}

func TestPrepareCommandArgs_NoServers(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "claude-code"}, newTestLogger(t))
	args := a.PrepareCommandArgs()
	if args != nil {
		t.Fatalf("expected nil args for no MCP servers, got %v", args)
	}
}

func TestPrepareCommandArgs_SSEServer(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID: "claude-code",
		McpServers: []shared.McpServerConfig{
			{Name: "kandev", URL: "http://localhost:10001/sse", Type: "sse"},
		},
	}, newTestLogger(t))

	args := a.PrepareCommandArgs()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "--mcp-config" {
		t.Fatalf("expected first arg to be --mcp-config, got %s", args[0])
	}

	// Parse the JSON to verify structure
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(args[1]), &config); err != nil {
		t.Fatalf("failed to parse MCP config JSON: %v", err)
	}

	// Must have mcpServers wrapper
	servers, ok := config["mcpServers"]
	if !ok {
		t.Fatalf("MCP config missing 'mcpServers' wrapper key, got: %s", args[1])
	}

	serversMap, ok := servers.(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers is not an object")
	}

	kandev, ok := serversMap["kandev"]
	if !ok {
		t.Fatalf("mcpServers missing 'kandev' key")
	}

	kandevMap, ok := kandev.(map[string]interface{})
	if !ok {
		t.Fatalf("kandev server config is not an object")
	}

	if kandevMap["url"] != "http://localhost:10001/sse" {
		t.Errorf("expected url 'http://localhost:10001/sse', got %v", kandevMap["url"])
	}
	if kandevMap["type"] != "sse" {
		t.Errorf("expected type 'sse', got %v", kandevMap["type"])
	}
}

func TestPrepareCommandArgs_StdioServer(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID: "claude-code",
		McpServers: []shared.McpServerConfig{
			{Name: "my-tool", Command: "node", Args: []string{"server.js", "--port", "3000"}},
		},
	}, newTestLogger(t))

	args := a.PrepareCommandArgs()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(args[1]), &config); err != nil {
		t.Fatalf("failed to parse MCP config JSON: %v", err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("MCP config missing 'mcpServers' wrapper")
	}

	tool, ok := servers["my-tool"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcpServers missing 'my-tool' key")
	}

	if tool["command"] != "node" {
		t.Errorf("expected command 'node', got %v", tool["command"])
	}

	toolArgs, ok := tool["args"].([]interface{})
	if !ok {
		t.Fatalf("expected args array, got %T", tool["args"])
	}
	if len(toolArgs) != 3 {
		t.Errorf("expected 3 args, got %d", len(toolArgs))
	}
}

func TestPrepareCommandArgs_MultipleServers(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID: "claude-code",
		McpServers: []shared.McpServerConfig{
			{Name: "kandev", URL: "http://localhost:10001/sse", Type: "sse"},
			{Name: "other", Command: "python", Args: []string{"-m", "mcp_server"}},
		},
	}, newTestLogger(t))

	args := a.PrepareCommandArgs()

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(args[1]), &config); err != nil {
		t.Fatalf("failed to parse MCP config JSON: %v", err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("MCP config missing 'mcpServers' wrapper")
	}

	if len(servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(servers))
	}
	if _, ok := servers["kandev"]; !ok {
		t.Error("missing 'kandev' server")
	}
	if _, ok := servers["other"]; !ok {
		t.Error("missing 'other' server")
	}
}

func TestBuildHooks_Autonomous(t *testing.T) {
	tests := []struct {
		name   string
		policy string
	}{
		{"empty policy", ""},
		{"explicit autonomous", "autonomous"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAdapter(&shared.Config{
				AgentID:          "claude-code",
				PermissionPolicy: tt.policy,
			}, newTestLogger(t))

			hooks := a.buildHooks()
			if hooks != nil {
				t.Errorf("buildHooks() = %v, want nil for autonomous mode", hooks)
			}
		})
	}
}

func TestBuildHooks_Supervised(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID:          "claude-code",
		PermissionPolicy: "supervised",
	}, newTestLogger(t))

	hooks := a.buildHooks()
	if hooks == nil {
		t.Fatal("buildHooks() returned nil, want hooks")
	}

	// Should have PreToolUse
	preToolUse, ok := hooks["PreToolUse"]
	if !ok {
		t.Fatal("buildHooks() missing PreToolUse key")
	}
	entries, ok := preToolUse.([]claudecode.HookEntry)
	if !ok {
		t.Fatalf("PreToolUse is %T, want []claudecode.HookEntry", preToolUse)
	}
	if len(entries) != 1 {
		t.Fatalf("PreToolUse has %d entries, want 1", len(entries))
	}
	if entries[0].HookCallbackIDs[0] != "tool_approval" {
		t.Errorf("PreToolUse[0].HookCallbackIDs[0] = %q, want %q", entries[0].HookCallbackIDs[0], "tool_approval")
	}

	// Should have Stop
	stop, ok := hooks["Stop"]
	if !ok {
		t.Fatal("buildHooks() missing Stop key")
	}
	stopEntries, ok := stop.([]claudecode.HookEntry)
	if !ok {
		t.Fatalf("Stop is %T, want []claudecode.HookEntry", stop)
	}
	if len(stopEntries) != 1 {
		t.Fatalf("Stop has %d entries, want 1", len(stopEntries))
	}
	if stopEntries[0].HookCallbackIDs[0] != "stop_git_check" {
		t.Errorf("Stop[0].HookCallbackIDs[0] = %q, want %q", stopEntries[0].HookCallbackIDs[0], "stop_git_check")
	}
}

func TestBuildHooks_Plan(t *testing.T) {
	a := NewAdapter(&shared.Config{
		AgentID:          "claude-code",
		PermissionPolicy: "plan",
	}, newTestLogger(t))

	hooks := a.buildHooks()
	if hooks == nil {
		t.Fatal("buildHooks() returned nil, want hooks")
	}

	// Plan mode should have 2 PreToolUse entries: ExitPlanMode approval + auto-approve rest
	preToolUse, ok := hooks["PreToolUse"]
	if !ok {
		t.Fatal("buildHooks() missing PreToolUse key")
	}
	entries, ok := preToolUse.([]claudecode.HookEntry)
	if !ok {
		t.Fatalf("PreToolUse is %T, want []claudecode.HookEntry", preToolUse)
	}
	if len(entries) != 2 {
		t.Fatalf("PreToolUse has %d entries, want 2", len(entries))
	}

	// First entry: ExitPlanMode requires approval
	if entries[0].Matcher != `^ExitPlanMode$` {
		t.Errorf("entries[0].Matcher = %q, want %q", entries[0].Matcher, `^ExitPlanMode$`)
	}
	if entries[0].HookCallbackIDs[0] != "tool_approval" {
		t.Errorf("entries[0].HookCallbackIDs[0] = %q, want %q", entries[0].HookCallbackIDs[0], "tool_approval")
	}

	// Second entry: everything else auto-approved
	if entries[1].HookCallbackIDs[0] != "auto_approve" {
		t.Errorf("entries[1].HookCallbackIDs[0] = %q, want %q", entries[1].HookCallbackIDs[0], "auto_approve")
	}

	// Should have Stop
	if _, ok := hooks["Stop"]; !ok {
		t.Error("buildHooks() missing Stop key")
	}
}

func TestSendControlResult_SendsAndTraces(t *testing.T) {
	// Create adapter with a mock stdin to capture output
	var buf syncBuf
	a := NewAdapter(&shared.Config{AgentID: "test-agent"}, newTestLogger(t))
	a.stdin = &buf

	// Wire up a client that writes to our buffer
	a.client = claudecode.NewClient(&buf, &emptyReader{}, newTestLogger(t))

	a.sendControlResult("req-123", &claudecode.PermissionResult{
		Behavior: claudecode.BehaviorAllow,
	}, "test_trace_event")

	// Verify something was written to stdin
	if buf.Len() == 0 {
		t.Error("sendControlResult did not write to stdin")
	}

	// Parse the sent message
	var resp claudecode.ControlResponseMessage
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse sent response: %v", err)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req-123")
	}
	if resp.Response == nil {
		t.Fatal("Response is nil")
	}
	if resp.Response.Subtype != "success" {
		t.Errorf("Response.Subtype = %q, want %q", resp.Response.Subtype, "success")
	}
}

// --- handleUserMessage tests ---

// drainEvents collects all pending events from the adapter's update channel.
func drainEvents(a *Adapter) []AgentEvent {
	var events []AgentEvent
	for {
		select {
		case ev := <-a.updatesCh:
			events = append(events, ev)
		default:
			return events
		}
	}
}

func TestHandleUserMessage_ReplaySkipped(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
	a.sessionID = "sess-1"
	a.operationID = "op-1"

	msg := &claudecode.CLIMessage{
		Type:     claudecode.MessageTypeUser,
		UUID:     "msg-uuid-1",
		IsReplay: true,
		Message: &claudecode.AssistantMessage{
			Role:    "user",
			Content: json.RawMessage(`"hello"`),
		},
	}

	a.handleUserMessage(msg, "sess-1", "op-1")

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events for replay message, got %d", len(events))
	}

	// UUID should still be tracked for --resume-session-at
	a.mu.RLock()
	lastUUID := a.lastMessageUUID
	a.mu.RUnlock()
	if lastUUID != "msg-uuid-1" {
		t.Errorf("lastMessageUUID = %q, want %q", lastUUID, "msg-uuid-1")
	}
}

func TestHandleUserMessage_EchoedPromptSkipped(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	msg := &claudecode.CLIMessage{
		Type: claudecode.MessageTypeUser,
		UUID: "msg-uuid-2",
		Message: &claudecode.AssistantMessage{
			Role:    "user",
			Content: json.RawMessage(`"hello world"`),
		},
	}

	a.handleUserMessage(msg, "sess-1", "op-1")

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events for echoed prompt, got %d", len(events))
	}
}

func TestHandleUserMessage_SlashCommandOutputSkipped(t *testing.T) {
	// Slash command output arrives as string content in a user message.
	// It is always isReplay=true in practice, but even without that flag,
	// string-content user messages are skipped — the output is delivered
	// via the result message instead.
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	msg := &claudecode.CLIMessage{
		Type: claudecode.MessageTypeUser,
		UUID: "msg-uuid-3",
		Message: &claudecode.AssistantMessage{
			Role:    "user",
			Content: json.RawMessage(`"<local-command-stdout>compact summary here</local-command-stdout>"`),
		},
	}

	a.handleUserMessage(msg, "sess-1", "op-1")

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events for slash command output (delivered via result), got %d", len(events))
	}
}

func TestHandleUserMessage_ToolResultStillProcessed(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	// Pre-register a pending tool call
	a.pendingToolCalls["tool-123"] = nil

	msg := &claudecode.CLIMessage{
		Type: claudecode.MessageTypeUser,
		UUID: "msg-uuid-4",
		Message: &claudecode.AssistantMessage{
			Role: "user",
			Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"tool-123","content":"output"}]`),
		},
	}

	a.handleUserMessage(msg, "sess-1", "op-1")

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event for tool result, got %d", len(events))
	}
	if events[0].Type != "tool_update" {
		t.Errorf("event type = %q, want %q", events[0].Type, "tool_update")
	}
	if events[0].ToolCallID != "tool-123" {
		t.Errorf("tool_call_id = %q, want %q", events[0].ToolCallID, "tool-123")
	}
}

func TestHandleUserMessage_SystemPromptEchoSkipped(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	// Simulate the real bug: Claude Code echoes back the full prompt including kandev-system tags
	content := `"<kandev-system>MCP TOOLS info</kandev-system>\n\nhello"`
	msg := &claudecode.CLIMessage{
		Type: claudecode.MessageTypeUser,
		UUID: "msg-uuid-5",
		Message: &claudecode.AssistantMessage{
			Role:    "user",
			Content: json.RawMessage(content),
		},
	}

	a.handleUserMessage(msg, "sess-1", "op-1")

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events for system prompt echo, got %d (text: %q)", len(events), events[0].Text)
	}
}

// --- handleMessage session_id tracking tests ---

func TestHandleMessage_SessionIDTracking(t *testing.T) {
	tests := []struct {
		name            string
		initialSession  string
		msgType         string
		msgSessionID    string
		wantSessionID   string
		wantSessionSame bool
	}{
		{
			name:          "system message sets session ID",
			msgType:       claudecode.MessageTypeSystem,
			msgSessionID:  "new-session-1",
			wantSessionID: "new-session-1",
		},
		{
			name:           "user message updates session ID",
			initialSession: "old-session",
			msgType:        claudecode.MessageTypeUser,
			msgSessionID:   "new-session-2",
			wantSessionID:  "new-session-2",
		},
		{
			name:           "assistant message updates session ID",
			initialSession: "old-session",
			msgType:        claudecode.MessageTypeAssistant,
			msgSessionID:   "compacted-session",
			wantSessionID:  "compacted-session",
		},
		{
			name:           "result message updates session ID",
			initialSession: "old-session",
			msgType:        claudecode.MessageTypeResult,
			msgSessionID:   "result-session",
			wantSessionID:  "result-session",
		},
		{
			name:           "empty session ID does not overwrite",
			initialSession: "keep-this",
			msgType:        claudecode.MessageTypeUser,
			msgSessionID:   "",
			wantSessionID:  "keep-this",
		},
		{
			name:           "same session ID is a no-op",
			initialSession: "same-session",
			msgType:        claudecode.MessageTypeAssistant,
			msgSessionID:   "same-session",
			wantSessionID:  "same-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))
			if tt.initialSession != "" {
				a.sessionID = tt.initialSession
			}

			msg := &claudecode.CLIMessage{
				Type:      tt.msgType,
				SessionID: tt.msgSessionID,
			}

			// handleMessage needs a message body for user/assistant types to avoid nil panics
			if tt.msgType == claudecode.MessageTypeUser || tt.msgType == claudecode.MessageTypeAssistant {
				msg.Message = &claudecode.AssistantMessage{
					Role:    "user",
					Content: json.RawMessage(`"test"`),
				}
			}
			// Result messages need a result field
			if tt.msgType == claudecode.MessageTypeResult {
				msg.Result = json.RawMessage(`"done"`)
			}

			a.handleMessage(msg)
			drainEvents(a) // discard any events

			a.mu.RLock()
			gotSessionID := a.sessionID
			a.mu.RUnlock()

			if gotSessionID != tt.wantSessionID {
				t.Errorf("sessionID = %q, want %q", gotSessionID, tt.wantSessionID)
			}
		})
	}
}

// syncBuf is a thread-safe buffer for tests.
type syncBuf struct {
	data []byte
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *syncBuf) Len() int    { return len(b.data) }
func (b *syncBuf) Bytes() []byte { return b.data }

// emptyReader always returns EOF.
type emptyReader struct{}

func (e *emptyReader) Read(p []byte) (int, error) { return 0, nil }

// --- TodoWrite normalizer tests ---

func TestNormalizeTodoWrite_ExtractsTodos(t *testing.T) {
	n := NewNormalizer()

	// TodoWrite uses "todos" array with "content" (not "items" with "description")
	args := map[string]any{
		"todos": []any{
			map[string]any{
				"content":    "Count lines in config.yaml",
				"status":     "pending",
				"activeForm": "Counting lines in config.yaml",
			},
			map[string]any{
				"content":    "Count lines in Makefile",
				"status":     "completed",
				"activeForm": "Counting lines in Makefile",
			},
		},
	}

	payload := n.NormalizeToolCall("TodoWrite", args)
	if payload.Kind() != "manage_todos" {
		t.Fatalf("kind = %q, want %q", payload.Kind(), "manage_todos")
	}
	todos := payload.ManageTodos()
	if todos.Operation != "write" {
		t.Errorf("operation = %q, want %q", todos.Operation, "write")
	}
	if len(todos.Items) != 2 {
		t.Fatalf("items count = %d, want 2", len(todos.Items))
	}
	if todos.Items[0].Description != "Count lines in config.yaml" {
		t.Errorf("items[0].Description = %q, want %q", todos.Items[0].Description, "Count lines in config.yaml")
	}
	if todos.Items[0].Status != "pending" {
		t.Errorf("items[0].Status = %q, want %q", todos.Items[0].Status, "pending")
	}
	if todos.Items[0].ActiveForm != "Counting lines in config.yaml" {
		t.Errorf("items[0].ActiveForm = %q, want %q", todos.Items[0].ActiveForm, "Counting lines in config.yaml")
	}
	if todos.Items[1].Status != "completed" {
		t.Errorf("items[1].Status = %q, want %q", todos.Items[1].Status, "completed")
	}
}

// --- tool_use_result enrichment tests ---

func TestHandleUserMessage_SubagentToolUseResult(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	// Pre-register a pending Task tool call
	n := NewNormalizer()
	taskPayload := n.NormalizeToolCall("Task", map[string]any{
		"description":  "Count lines",
		"prompt":       "Count lines in file",
		"subagent_type": "Bash",
	})
	a.pendingToolCalls["task-1"] = taskPayload

	// Sub-agent result with array content and tool_use_result metadata
	msg := &claudecode.CLIMessage{
		Type: claudecode.MessageTypeUser,
		UUID: "msg-1",
		Message: &claudecode.AssistantMessage{
			Role:    "user",
			Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"task-1","content":[{"type":"text","text":"61 config.yaml"},{"type":"text","text":"agentId: abc123"}]}]`),
		},
		ToolUseResult: json.RawMessage(`{"status":"completed","agentId":"abc123","totalDurationMs":1609,"totalTokens":4540,"totalToolUseCount":1}`),
	}

	a.handleUserMessage(msg, "sess-1", "op-1")

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	payload := events[0].NormalizedPayload
	if payload == nil {
		t.Fatal("payload is nil")
	}
	st := payload.SubagentTask()
	if st == nil {
		t.Fatal("SubagentTask is nil")
	}
	if st.Status != "completed" {
		t.Errorf("status = %q, want %q", st.Status, "completed")
	}
	if st.AgentID != "abc123" {
		t.Errorf("agentID = %q, want %q", st.AgentID, "abc123")
	}
	if st.DurationMs != 1609 {
		t.Errorf("durationMs = %d, want %d", st.DurationMs, 1609)
	}
	if st.TotalTokens != 4540 {
		t.Errorf("totalTokens = %d, want %d", st.TotalTokens, 4540)
	}
	if st.ToolUseCount != 1 {
		t.Errorf("toolUseCount = %d, want %d", st.ToolUseCount, 1)
	}
}

func TestHandleUserMessage_TodoWriteToolUseResult(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	// Pre-register a pending TodoWrite tool call
	n := NewNormalizer()
	todoPayload := n.NormalizeToolCall("TodoWrite", map[string]any{
		"todos": []any{
			map[string]any{"content": "Task A", "status": "pending"},
		},
	})
	a.pendingToolCalls["todo-1"] = todoPayload

	msg := &claudecode.CLIMessage{
		Type: claudecode.MessageTypeUser,
		UUID: "msg-2",
		Message: &claudecode.AssistantMessage{
			Role:    "user",
			Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"todo-1","content":"Todos modified"}]`),
		},
		ToolUseResult: json.RawMessage(`{"oldTodos":[],"newTodos":[{"content":"Task A","status":"completed","activeForm":"Working on Task A"}]}`),
	}

	a.handleUserMessage(msg, "sess-1", "op-1")

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	payload := events[0].NormalizedPayload
	if payload == nil || payload.ManageTodos() == nil {
		t.Fatal("payload or ManageTodos is nil")
	}
	items := payload.ManageTodos().Items
	if len(items) != 1 {
		t.Fatalf("items count = %d, want 1", len(items))
	}
	if items[0].Description != "Task A" {
		t.Errorf("items[0].Description = %q, want %q", items[0].Description, "Task A")
	}
	if items[0].Status != "completed" {
		t.Errorf("items[0].Status = %q, want %q", items[0].Status, "completed")
	}
	if items[0].ActiveForm != "Working on Task A" {
		t.Errorf("items[0].ActiveForm = %q, want %q", items[0].ActiveForm, "Working on Task A")
	}
}

// --- system task_started subtype test ---

func TestHandleSystemMessage_TaskStartedSkipped(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	// Simulate the system task_started message that Claude Code sends for sub-agents
	msg := &claudecode.CLIMessage{
		Type:      claudecode.MessageTypeSystem,
		Subtype:   "task_started",
		SessionID: "sess-1",
	}

	a.handleSystemMessage(msg)

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events for task_started system message, got %d", len(events))
	}
}

func TestHandleSystemMessage_InitStillWorks(t *testing.T) {
	a := NewAdapter(&shared.Config{AgentID: "test"}, newTestLogger(t))

	// Normal init system message (subtype empty)
	msg := &claudecode.CLIMessage{
		Type:          claudecode.MessageTypeSystem,
		SessionID:     "sess-1",
		SessionStatus: "new",
	}

	a.handleSystemMessage(msg)

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event for init system message, got %d", len(events))
	}
	if events[0].Type != "session_status" {
		t.Errorf("event type = %q, want %q", events[0].Type, "session_status")
	}
}
