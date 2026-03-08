package main

import (
	"context"
	"strings"
	"sync"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestIsScriptMode(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"e2e:message(\"hello\")", true},
		{"e2e:thinking(\"thought\")", true},
		{"e2e:delay(100)", true},
		{"e2e:mcp:kandev:create_task_plan({})", true},
		{"/e2e:simple-message", false},
		{"hello world", false},
		{"", false},
		{"/all", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			if got := isScriptMode(tt.cmd); got != tt.want {
				t.Errorf("isScriptMode(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestExtractStringArg(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		prefix string
		want   string
	}{
		{"simple string", `e2e:message("hello world")`, "e2e:message(", "hello world"},
		{"escaped quotes", `e2e:message("say \"hi\"")`, "e2e:message(", `say "hi"`},
		{"newline escape", `e2e:message("line1\nline2")`, "e2e:message(", "line1\nline2"},
		{"tab escape", `e2e:message("col1\tcol2")`, "e2e:message(", "col1\tcol2"},
		{"escaped backslash", `e2e:message("path\\file")`, "e2e:message(", `path\file`},
		{"thinking command", `e2e:thinking("deep thought")`, "e2e:thinking(", "deep thought"},
		{"tool_result command", `e2e:tool_result("success")`, "e2e:tool_result(", "success"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractStringArg(tt.line, tt.prefix)
			if got != tt.want {
				t.Errorf("extractStringArg(%q, %q) = %q, want %q", tt.line, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestExtractIntArg(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		prefix string
		want   int
	}{
		{"simple int", "e2e:delay(100)", "e2e:delay(", 100},
		{"zero", "e2e:delay(0)", "e2e:delay(", 0},
		{"large number", "e2e:delay(5000)", "e2e:delay(", 5000},
		{"invalid returns 0", "e2e:delay(abc)", "e2e:delay(", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIntArg(tt.line, tt.prefix)
			if got != tt.want {
				t.Errorf("extractIntArg(%q, %q) = %d, want %d", tt.line, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestExtractParenContent(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		prefix string
		want   string
	}{
		{"simple content", `e2e:tool_use("Read", {"path":"/tmp"})`, "e2e:tool_use(", `"Read", {"path":"/tmp"}`},
		{"no trailing paren", `e2e:tool_use("Read"`, "e2e:tool_use(", `"Read"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractParenContent(tt.line, tt.prefix)
			if got != tt.want {
				t.Errorf("extractParenContent(%q, %q) = %q, want %q", tt.line, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestExtractFirstStringArg(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantVal  string
		wantRest string
	}{
		{"simple string", `"hello", rest`, "hello", ", rest"},
		{"string with escaped quote", `"say \"hi\"", more`, `say "hi"`, ", more"},
		{"just a string", `"only"`, "only", ""},
		{"empty input", "", "", ""},
		{"non-quoted input", "bare", "bare", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, rest := extractFirstStringArg(tt.input)
			if val != tt.wantVal {
				t.Errorf("extractFirstStringArg(%q) val = %q, want %q", tt.input, val, tt.wantVal)
			}
			if rest != tt.wantRest {
				t.Errorf("extractFirstStringArg(%q) rest = %q, want %q", tt.input, rest, tt.wantRest)
			}
		})
	}
}

func TestUnescapeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no escapes", "hello", "hello"},
		{"newline", `line1\nline2`, "line1\nline2"},
		{"tab", `col1\tcol2`, "col1\tcol2"},
		{"escaped quote", `say \"hi\"`, `say "hi"`},
		{"escaped backslash", `path\\file`, `path\file`},
		{"mixed escapes", `a\\b\nc\"d`, "a\\b\nc\"d"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeString(tt.input)
			if got != tt.want {
				t.Errorf("unescapeString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSubstituteContextPlaceholders(t *testing.T) {
	fullPrompt := `<kandev-system>
Kandev Task ID: task-abc-123
Kandev Session ID: sess-xyz-789
</kandev-system>
e2e:mcp:kandev:create_task_plan({"task_id":"{task_id}"})`

	t.Run("replaces task_id", func(t *testing.T) {
		args := map[string]any{"task_id": "{task_id}"}
		substituteContextPlaceholders(args, fullPrompt)
		if args["task_id"] != "task-abc-123" {
			t.Errorf("task_id = %q, want %q", args["task_id"], "task-abc-123")
		}
	})

	t.Run("replaces session_id", func(t *testing.T) {
		args := map[string]any{"session_id": "{session_id}"}
		substituteContextPlaceholders(args, fullPrompt)
		if args["session_id"] != "sess-xyz-789" {
			t.Errorf("session_id = %q, want %q", args["session_id"], "sess-xyz-789")
		}
	})

	t.Run("replaces both in same map", func(t *testing.T) {
		args := map[string]any{
			"task_id":    "{task_id}",
			"session_id": "{session_id}",
			"other":      "untouched",
		}
		substituteContextPlaceholders(args, fullPrompt)
		if args["task_id"] != "task-abc-123" {
			t.Errorf("task_id = %q, want %q", args["task_id"], "task-abc-123")
		}
		if args["session_id"] != "sess-xyz-789" {
			t.Errorf("session_id = %q, want %q", args["session_id"], "sess-xyz-789")
		}
		if args["other"] != "untouched" {
			t.Errorf("other = %q, want %q", args["other"], "untouched")
		}
	})

	t.Run("no placeholders means empty substitution", func(t *testing.T) {
		args := map[string]any{"task_id": "{task_id}"}
		substituteContextPlaceholders(args, "no kandev system block")
		if args["task_id"] != "" {
			t.Errorf("task_id should be empty when no context, got %q", args["task_id"])
		}
	})

	t.Run("skips non-string values", func(t *testing.T) {
		args := map[string]any{
			"count": 42,
			"flag":  true,
		}
		substituteContextPlaceholders(args, fullPrompt)
		if args["count"] != 42 {
			t.Errorf("count changed unexpectedly")
		}
		if args["flag"] != true {
			t.Errorf("flag changed unexpectedly")
		}
	})
}

func TestExtractRegexMatch(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"task ID found", "Kandev Task ID: abc-123", "abc-123"},
		{"task ID with extra whitespace", "Kandev Task ID:   def-456", "def-456"},
		{"no match", "no task id here", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRegexMatch(taskIDRegex, tt.input)
			if got != tt.want {
				t.Errorf("extractRegexMatch(taskIDRegex, %q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Mock sessionUpdater for testing ---

// recordedUpdate stores a captured ACP session notification.
type recordedUpdate struct {
	notification acp.SessionNotification
}

// mockUpdater implements sessionUpdater for tests, collecting all updates.
type mockUpdater struct {
	mu      sync.Mutex
	updates []recordedUpdate
}

func (m *mockUpdater) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates = append(m.updates, recordedUpdate{notification: n})
	return nil
}

func (m *mockUpdater) RequestPermission(_ context.Context, _ acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Selected: &acp.RequestPermissionOutcomeSelected{OptionId: "allow"},
		},
	}, nil
}

func (m *mockUpdater) getUpdates() []recordedUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]recordedUpdate{}, m.updates...)
}

func newTestEmitter() (*emitter, *mockUpdater) {
	mock := &mockUpdater{}
	e := &emitter{
		ctx:  context.Background(),
		conn: mock,
		sid:  "test-session",
	}
	return e, mock
}

// --- Update assertion helpers ---

func isTextUpdate(u recordedUpdate) bool {
	return u.notification.Update.AgentMessageChunk != nil
}

func isThoughtUpdate(u recordedUpdate) bool {
	return u.notification.Update.AgentThoughtChunk != nil
}

func isToolCallUpdate(u recordedUpdate) bool {
	return u.notification.Update.ToolCall != nil
}

func isToolCallCompleteUpdate(u recordedUpdate) bool {
	return u.notification.Update.ToolCallUpdate != nil
}

func getTextContent(u recordedUpdate) string {
	if u.notification.Update.AgentMessageChunk == nil {
		return ""
	}
	if u.notification.Update.AgentMessageChunk.Content.Text == nil {
		return ""
	}
	return u.notification.Update.AgentMessageChunk.Content.Text.Text
}

func getThoughtContent(u recordedUpdate) string {
	if u.notification.Update.AgentThoughtChunk == nil {
		return ""
	}
	if u.notification.Update.AgentThoughtChunk.Content.Text == nil {
		return ""
	}
	return u.notification.Update.AgentThoughtChunk.Content.Text.Text
}

func getToolCallTitle(u recordedUpdate) string {
	if u.notification.Update.ToolCall == nil {
		return ""
	}
	return u.notification.Update.ToolCall.Title
}

func getToolCallID(u recordedUpdate) acp.ToolCallId {
	if u.notification.Update.ToolCall != nil {
		return u.notification.Update.ToolCall.ToolCallId
	}
	if u.notification.Update.ToolCallUpdate != nil {
		return u.notification.Update.ToolCallUpdate.ToolCallId
	}
	return ""
}

// --- Script execution tests ---

// TestExecuteScriptMessage verifies e2e:message emits an agent text update.
func TestExecuteScriptMessage(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	executeScript(e, "", `e2e:message("hello from script")`)

	updates := mock.getUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if !isTextUpdate(updates[0]) {
		t.Fatal("expected text update")
	}
	if got := getTextContent(updates[0]); got != "hello from script" {
		t.Errorf("text = %q, want %q", got, "hello from script")
	}
}

// TestExecuteScriptThinking verifies e2e:thinking emits a thought update.
func TestExecuteScriptThinking(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	executeScript(e, "", `e2e:thinking("deep thought")`)

	updates := mock.getUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if !isThoughtUpdate(updates[0]) {
		t.Fatal("expected thought update")
	}
	if got := getThoughtContent(updates[0]); got != "deep thought" {
		t.Errorf("thought = %q, want %q", got, "deep thought")
	}
}

// TestExecuteScriptMultiLine verifies multiple commands are processed sequentially.
func TestExecuteScriptMultiLine(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	script := "e2e:thinking(\"step 1\")\ne2e:message(\"step 2\")\ne2e:message(\"step 3\")"
	executeScript(e, "", script)

	updates := mock.getUpdates()
	if len(updates) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(updates))
	}
	if got := getThoughtContent(updates[0]); got != "step 1" {
		t.Errorf("thought = %q, want %q", got, "step 1")
	}
	if got := getTextContent(updates[1]); got != "step 2" {
		t.Errorf("text = %q, want %q", got, "step 2")
	}
	if got := getTextContent(updates[2]); got != "step 3" {
		t.Errorf("text = %q, want %q", got, "step 3")
	}
}

// TestExecuteScriptSkipsEmptyAndComments verifies blank lines and comments are skipped.
func TestExecuteScriptSkipsEmptyAndComments(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	script := "e2e:message(\"one\")\n\n# this is a comment\n   \ne2e:message(\"two\")"
	executeScript(e, "", script)

	updates := mock.getUpdates()
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}
	if got := getTextContent(updates[0]); got != "one" {
		t.Errorf("text = %q, want %q", got, "one")
	}
	if got := getTextContent(updates[1]); got != "two" {
		t.Errorf("text = %q, want %q", got, "two")
	}
}

// TestExecuteScriptToolUse verifies e2e:tool_use emits a tool_call update.
func TestExecuteScriptToolUse(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	executeScript(e, "", `e2e:tool_use("Read", {"file_path":"/tmp/test.go"})`)

	updates := mock.getUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if !isToolCallUpdate(updates[0]) {
		t.Fatal("expected tool_call update")
	}
	if got := getToolCallTitle(updates[0]); got != "Read" {
		t.Errorf("title = %q, want %q", got, "Read")
	}
}

// TestExecuteScriptToolUseAndResult verifies tool_use + tool_result pair.
func TestExecuteScriptToolUseAndResult(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	script := "e2e:tool_use(\"Read\", {\"file_path\":\"/tmp/f.go\"})\ne2e:tool_result(\"file contents here\")"
	executeScript(e, "", script)

	updates := mock.getUpdates()
	if len(updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(updates))
	}

	// First: tool_call start
	if !isToolCallUpdate(updates[0]) {
		t.Fatal("expected tool_call update")
	}
	toolID := getToolCallID(updates[0])
	if toolID == "" {
		t.Fatal("tool_call should have an id")
	}

	// Second: tool_call_update (completion) referencing same ID
	if !isToolCallCompleteUpdate(updates[1]) {
		t.Fatal("expected tool_call_update")
	}
	if got := getToolCallID(updates[1]); got != toolID {
		t.Errorf("tool_call_update id = %q, want %q", got, toolID)
	}
}

// TestExecuteScriptToolUseNoInput verifies tool_use with no input arg.
func TestExecuteScriptToolUseNoInput(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	executeScript(e, "", `e2e:tool_use("Glob")`)

	updates := mock.getUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	if got := getToolCallTitle(updates[0]); got != "Glob" {
		t.Errorf("title = %q, want %q", got, "Glob")
	}
}

func TestParseMCPConfigFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no flag", []string{"mock-agent"}, ""},
		{"separate flag and value", []string{"mock-agent", "--mcp-config", `{"mcpServers":{"kandev":{"url":"http://localhost:10005/sse"}}}`}, `{"mcpServers":{"kandev":{"url":"http://localhost:10005/sse"}}}`},
		{"equals syntax", []string{"mock-agent", `--mcp-config={"mcpServers":{}}`}, `{"mcpServers":{}}`},
		{"with other flags", []string{"mock-agent", "--model", "fast", "--mcp-config", `{"mcpServers":{}}`}, `{"mcpServers":{}}`},
		{"dangling flag", []string{"mock-agent", "--mcp-config"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMCPConfigFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("parseMCPConfigFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// TestExtractMCPResultText verifies the MCP result text extraction.
func TestExtractMCPResultText(t *testing.T) {
	t.Run("single text content", func(t *testing.T) {
		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "hello"},
			},
		}
		got := extractMCPResultText(result)
		if got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("multiple text contents joined with newline", func(t *testing.T) {
		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "line1"},
				mcp.TextContent{Type: "text", Text: "line2"},
			},
		}
		got := extractMCPResultText(result)
		if got != "line1\nline2" {
			t.Errorf("got %q, want %q", got, "line1\nline2")
		}
	})

	t.Run("empty content returns empty string", func(t *testing.T) {
		result := &mcp.CallToolResult{
			Content: []mcp.Content{},
		}
		got := extractMCPResultText(result)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

// TestExecuteScriptToolUseBadJSON verifies that malformed JSON emits a script error.
func TestExecuteScriptToolUseBadJSON(t *testing.T) {
	e, mock := newTestEmitter()
	resetState()

	executeScript(e, "", `e2e:tool_use("Read", {bad json})`)

	updates := mock.getUpdates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update (error), got %d", len(updates))
	}
	if !isTextUpdate(updates[0]) {
		t.Fatal("expected text update for error")
	}
	text := getTextContent(updates[0])
	if !strings.Contains(text, "Script error") {
		t.Errorf("expected script error message, got %q", text)
	}
}
