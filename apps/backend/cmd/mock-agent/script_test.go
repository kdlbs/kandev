package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
		{
			name:   "simple string",
			line:   `e2e:message("hello world")`,
			prefix: "e2e:message(",
			want:   "hello world",
		},
		{
			name:   "string with escaped quotes",
			line:   `e2e:message("say \"hi\"")`,
			prefix: "e2e:message(",
			want:   `say "hi"`,
		},
		{
			name:   "string with newline escape",
			line:   `e2e:message("line1\nline2")`,
			prefix: "e2e:message(",
			want:   "line1\nline2",
		},
		{
			name:   "string with tab escape",
			line:   `e2e:message("col1\tcol2")`,
			prefix: "e2e:message(",
			want:   "col1\tcol2",
		},
		{
			name:   "string with escaped backslash",
			line:   `e2e:message("path\\file")`,
			prefix: "e2e:message(",
			want:   `path\file`,
		},
		{
			name:   "thinking command",
			line:   `e2e:thinking("deep thought")`,
			prefix: "e2e:thinking(",
			want:   "deep thought",
		},
		{
			name:   "tool_result command",
			line:   `e2e:tool_result("success")`,
			prefix: "e2e:tool_result(",
			want:   "success",
		},
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
		{
			name:   "simple content",
			line:   `e2e:tool_use("Read", {"path":"/tmp"})`,
			prefix: "e2e:tool_use(",
			want:   `"Read", {"path":"/tmp"}`,
		},
		{
			name:   "no trailing paren",
			line:   `e2e:tool_use("Read"`,
			prefix: "e2e:tool_use(",
			want:   `"Read"`,
		},
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
		{
			name:     "simple string",
			input:    `"hello", rest`,
			wantVal:  "hello",
			wantRest: ", rest",
		},
		{
			name:     "string with escaped quote",
			input:    `"say \"hi\"", more`,
			wantVal:  `say "hi"`,
			wantRest: ", more",
		},
		{
			name:     "just a string",
			input:    `"only"`,
			wantVal:  "only",
			wantRest: "",
		},
		{
			name:     "empty input",
			input:    "",
			wantVal:  "",
			wantRest: "",
		},
		{
			name:     "non-quoted input",
			input:    "bare",
			wantVal:  "bare",
			wantRest: "",
		},
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
		{
			name:  "task ID found",
			input: "Kandev Task ID: abc-123",
			want:  "abc-123",
		},
		{
			name:  "task ID with extra whitespace",
			input: "Kandev Task ID:   def-456",
			want:  "def-456",
		},
		{
			name:  "no match",
			input: "no task id here",
			want:  "",
		},
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

// TestExecuteScriptMessage verifies e2e:message emits an assistant text block.
func TestExecuteScriptMessage(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	// Reset global state
	resetState()

	executeScript(enc, "", `e2e:message("hello from script")`)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	assertMsgType(t, msgs[0], TypeAssistant)
	assertContentText(t, msgs[0], "hello from script")
}

// TestExecuteScriptThinking verifies e2e:thinking emits a thinking block.
func TestExecuteScriptThinking(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	executeScript(enc, "", `e2e:thinking("deep thought")`)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	assertMsgType(t, msgs[0], TypeAssistant)
	assertContentThinking(t, msgs[0], "deep thought")
}

// TestExecuteScriptMultiLine verifies multiple commands are processed sequentially.
func TestExecuteScriptMultiLine(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resetState()

	script := "e2e:thinking(\"step 1\")\ne2e:message(\"step 2\")\ne2e:message(\"step 3\")"
	executeScript(enc, "", script)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	assertContentThinking(t, msgs[0], "step 1")
	assertContentText(t, msgs[1], "step 2")
	assertContentText(t, msgs[2], "step 3")
}

// TestExecuteScriptSkipsEmptyAndComments verifies blank lines and comments are skipped.
func TestExecuteScriptSkipsEmptyAndComments(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resetState()

	script := "e2e:message(\"one\")\n\n# this is a comment\n   \ne2e:message(\"two\")"
	executeScript(enc, "", script)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	assertContentText(t, msgs[0], "one")
	assertContentText(t, msgs[1], "two")
}

// TestExecuteScriptToolUse verifies e2e:tool_use emits a tool_use block.
func TestExecuteScriptToolUse(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resetState()

	executeScript(enc, "", `e2e:tool_use("Read", {"file_path":"/tmp/test.go"})`)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	assertMsgType(t, msgs[0], TypeAssistant)
	content := getFirstContent(t, msgs[0])
	if content["type"] != BlockToolUse {
		t.Errorf("content type = %q, want %q", content["type"], BlockToolUse)
	}
	if content["name"] != "Read" {
		t.Errorf("tool name = %q, want %q", content["name"], "Read")
	}
	input, ok := content["input"].(map[string]any)
	if !ok {
		t.Fatal("input is not a map")
	}
	if input["file_path"] != "/tmp/test.go" {
		t.Errorf("file_path = %q, want %q", input["file_path"], "/tmp/test.go")
	}
}

// TestExecuteScriptToolUseAndResult verifies tool_use + tool_result pair.
func TestExecuteScriptToolUseAndResult(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resetState()

	script := "e2e:tool_use(\"Read\", {\"file_path\":\"/tmp/f.go\"})\ne2e:tool_result(\"file contents here\")"
	executeScript(enc, "", script)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// First: tool_use
	toolUseContent := getFirstContent(t, msgs[0])
	toolID := toolUseContent["id"].(string)
	if toolID == "" {
		t.Fatal("tool_use should have an id")
	}

	// Second: tool_result referencing same ID
	assertMsgType(t, msgs[1], TypeUser)
	resultContent := getFirstContent(t, msgs[1])
	if resultContent["type"] != BlockToolResult {
		t.Errorf("result type = %q, want %q", resultContent["type"], BlockToolResult)
	}
	if resultContent["tool_use_id"] != toolID {
		t.Errorf("tool_use_id = %q, want %q", resultContent["tool_use_id"], toolID)
	}
	if resultContent["content"] != "file contents here" {
		t.Errorf("content = %q, want %q", resultContent["content"], "file contents here")
	}
}

// TestExecuteScriptToolUseNoInput verifies tool_use with no input arg.
func TestExecuteScriptToolUseNoInput(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resetState()

	executeScript(enc, "", `e2e:tool_use("Glob")`)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	content := getFirstContent(t, msgs[0])
	if content["name"] != "Glob" {
		t.Errorf("tool name = %q, want %q", content["name"], "Glob")
	}
}

func TestParseMCPConfigFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no flag",
			args: []string{"mock-agent"},
			want: "",
		},
		{
			name: "separate flag and value",
			args: []string{"mock-agent", "--mcp-config", `{"mcpServers":{"kandev":{"url":"http://localhost:10005/sse"}}}`},
			want: `{"mcpServers":{"kandev":{"url":"http://localhost:10005/sse"}}}`,
		},
		{
			name: "equals syntax",
			args: []string{"mock-agent", `--mcp-config={"mcpServers":{}}`},
			want: `{"mcpServers":{}}`,
		},
		{
			name: "with other flags",
			args: []string{"mock-agent", "--model", "fast", "--mcp-config", `{"mcpServers":{}}`},
			want: `{"mcpServers":{}}`,
		},
		{
			name: "dangling flag",
			args: []string{"mock-agent", "--mcp-config"},
			want: "",
		},
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
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	resetState()

	executeScript(enc, "", `e2e:tool_use("Read", {bad json})`)

	msgs := decodeJSONLines(t, buf.Bytes())
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (error), got %d", len(msgs))
	}
	assertMsgType(t, msgs[0], TypeAssistant)
	content := getFirstContent(t, msgs[0])
	text, _ := content["text"].(string)
	if !strings.Contains(text, "Script error") {
		t.Errorf("expected script error message, got %q", text)
	}
}

// --- Test helpers ---

// decodeJSONLines splits output into JSON-decoded maps.
func decodeJSONLines(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var results []map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		results = append(results, m)
	}
	return results
}

func assertMsgType(t *testing.T, msg map[string]any, wantType string) {
	t.Helper()
	if msg["type"] != wantType {
		t.Errorf("message type = %q, want %q", msg["type"], wantType)
	}
}

func getFirstContent(t *testing.T, msg map[string]any) map[string]any {
	t.Helper()
	message, ok := msg["message"].(map[string]any)
	if !ok {
		t.Fatal("message field is not a map")
	}
	contentArr, ok := message["content"].([]any)
	if !ok || len(contentArr) == 0 {
		t.Fatal("content is not an array or is empty")
	}
	block, ok := contentArr[0].(map[string]any)
	if !ok {
		t.Fatal("first content block is not a map")
	}
	return block
}

func assertContentText(t *testing.T, msg map[string]any, wantText string) {
	t.Helper()
	content := getFirstContent(t, msg)
	if content["type"] != BlockText {
		t.Errorf("content type = %q, want %q", content["type"], BlockText)
	}
	// emitTextBlock appends a trailing newline to match real agent streaming behavior;
	// allow the expected text with or without the trailing newline.
	got, _ := content["text"].(string)
	if got != wantText && got != wantText+"\n" {
		t.Errorf("text = %q, want %q (or with trailing newline)", got, wantText)
	}
}

func assertContentThinking(t *testing.T, msg map[string]any, wantThought string) {
	t.Helper()
	content := getFirstContent(t, msg)
	if content["type"] != BlockThinking {
		t.Errorf("content type = %q, want %q", content["type"], BlockThinking)
	}
	if content["thinking"] != wantThought {
		t.Errorf("thinking = %q, want %q", content["thinking"], wantThought)
	}
}
