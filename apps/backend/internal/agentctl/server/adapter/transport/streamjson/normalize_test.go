package streamjson

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/claudecode"
)

// TestStreamJSONNormalization runs JSONL-driven tests for stream-json protocol normalization.
func TestStreamJSONNormalization(t *testing.T) {
	testCases := shared.LoadTestCases(t, "streamjson-messages.jsonl")
	normalizer := NewNormalizer()

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("line_%d", i+1), func(t *testing.T) {
			toolName, _ := tc.Input["tool_name"].(string)
			args, _ := tc.Input["args"].(map[string]any)

			// Detect tool type
			toolType := DetectStreamJSONToolType(toolName)
			expectedToolType, _ := tc.Expected["tool_type"].(string)
			if toolType != expectedToolType {
				t.Errorf("tool type mismatch: got %q, want %q", toolType, expectedToolType)
			}

			// Normalize using the typed normalizer
			payload := normalizer.NormalizeToolCall(toolName, args)

			// Verify the Kind is set correctly based on tool type
			switch toolType {
			case "tool_edit":
				if payload.Kind() != streams.ToolKindModifyFile {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, payload.Kind())
				}
			case "tool_read":
				if payload.Kind() != streams.ToolKindReadFile && payload.Kind() != streams.ToolKindCodeSearch {
					t.Errorf("expected Kind %q or %q, got %q", streams.ToolKindReadFile, streams.ToolKindCodeSearch, payload.Kind())
				}
			case "tool_execute":
				if payload.Kind() != streams.ToolKindShellExec {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindShellExec, payload.Kind())
				}
			}
		})
	}
}

// TestDetectStreamJSONToolType tests the stream-json protocol tool type detection function.
func TestDetectStreamJSONToolType(t *testing.T) {
	tests := []struct {
		toolName string
		want     string
	}{
		{"Edit", "tool_edit"},
		{"Read", "tool_read"},
		{"Bash", "tool_execute"},
		{"Glob", "tool_read"},  // Glob is a read operation
		{"Grep", "tool_read"},  // Grep is a read operation
		{"Write", "tool_edit"}, // Write is an edit operation
		{"WebSearch", "tool_call"},
		{"WebFetch", "tool_call"},
		{"Task", "tool_call"},
		{"", "tool_call"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			got := DetectStreamJSONToolType(tt.toolName)
			if got != tt.want {
				t.Errorf("DetectStreamJSONToolType(%q) = %q, want %q", tt.toolName, got, tt.want)
			}
		})
	}
}

// TestDetectLanguage tests the language detection from file extensions.
func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file.ts", "typescript"},
		{"file.tsx", "typescript"},
		{"file.js", "javascript"},
		{"file.jsx", "javascript"},
		{"file.py", "python"},
		{"file.go", "go"},
		{"file.rs", "rust"},
		{"file.java", "java"},
		{"file.cpp", "cpp"},
		{"file.c", "c"},
		{"file.h", "c"},
		{"file.hpp", "cpp"},
		{"file.css", "css"},
		{"file.html", "html"},
		{"file.json", "json"},
		{"file.md", "markdown"},
		{"file.yaml", "yaml"},
		{"file.yml", "yaml"},
		{"file.sh", "bash"},
		{"file.bash", "bash"},
		{"file.unknown", "plaintext"},
		{"file", "plaintext"},
		{"", "plaintext"},
		{"/path/to/deep/file.ts", "typescript"},
		{"src/main.go", "go"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := shared.DetectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestGenerateUnifiedDiff tests the unified diff generation.
func TestGenerateUnifiedDiff(t *testing.T) {
	t.Run("returns empty when old is empty", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("", "new content", "file.ts", 1)
		if result != "" {
			t.Errorf("expected empty string for empty old string, got %q", result)
		}
	})

	t.Run("returns empty when new is empty", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("old content", "", "file.ts", 1)
		if result != "" {
			t.Errorf("expected empty string for empty new string, got %q", result)
		}
	})

	t.Run("generates diff with header and hunks", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("const x = 1;", "const x = 2;", "file.ts", 5)
		if result == "" {
			t.Fatal("expected non-empty diff")
		}

		// Check it contains expected elements
		if !strings.Contains(result, "diff --git a/file.ts b/file.ts") {
			t.Error("expected diff header")
		}
		if !strings.Contains(result, "--- a/file.ts") {
			t.Error("expected old file marker")
		}
		if !strings.Contains(result, "+++ b/file.ts") {
			t.Error("expected new file marker")
		}
		if !strings.Contains(result, "-const x = 1;") {
			t.Error("expected removed line")
		}
		if !strings.Contains(result, "+const x = 2;") {
			t.Error("expected added line")
		}
	})

	t.Run("defaults startLine to 1 when 0", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("a", "b", "file.go", 0)
		if result == "" {
			t.Fatal("expected non-empty diff")
		}
		// Verify it contains @@ -1 (defaulted from 0)
		if !strings.Contains(result, "@@ -1,") {
			t.Error("expected startLine to default to 1")
		}
	})
}

// TestNormalizerEdit tests the normalizer's edit handling.
func TestNormalizerEdit(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts file_path and content for Edit", func(t *testing.T) {
		args := map[string]any{
			"file_path":  "/workspace/app.ts",
			"old_string": "const x = 1;",
			"new_string": "const x = 2;",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolEdit, args)
		if result.Kind() != streams.ToolKindModifyFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, result.Kind())
		}
		if result.ModifyFile() == nil {
			t.Fatal("expected ModifyFile to be set")
		}
		if result.ModifyFile().FilePath != "/workspace/app.ts" {
			t.Errorf("expected FilePath '/workspace/app.ts', got %q", result.ModifyFile().FilePath)
		}
		if len(result.ModifyFile().Mutations) != 1 {
			t.Fatalf("expected 1 mutation, got %d", len(result.ModifyFile().Mutations))
		}
		mutation := result.ModifyFile().Mutations[0]
		if mutation.OldContent != "const x = 1;" {
			t.Errorf("expected OldContent 'const x = 1;', got %q", mutation.OldContent)
		}
		if mutation.NewContent != "const x = 2;" {
			t.Errorf("expected NewContent 'const x = 2;', got %q", mutation.NewContent)
		}
	})

	t.Run("generates diff when both strings present", func(t *testing.T) {
		args := map[string]any{
			"file_path":  "/workspace/app.ts",
			"old_string": "const x = 1;",
			"new_string": "const x = 2;",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolEdit, args)
		if result.ModifyFile() == nil || len(result.ModifyFile().Mutations) == 0 {
			t.Fatal("expected mutation")
		}
		if result.ModifyFile().Mutations[0].Diff == "" {
			t.Error("expected diff to be generated")
		}
	})

	t.Run("no diff when old_string is empty", func(t *testing.T) {
		args := map[string]any{
			"file_path":  "/workspace/new.ts",
			"old_string": "",
			"new_string": "new content",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolEdit, args)
		if result.ModifyFile() == nil || len(result.ModifyFile().Mutations) == 0 {
			t.Fatal("expected mutation")
		}
		if result.ModifyFile().Mutations[0].Diff != "" {
			t.Error("expected no diff when old_string is empty")
		}
	})

	t.Run("Write tool uses content field", func(t *testing.T) {
		args := map[string]any{
			"file_path": "/workspace/CONTRIBUTING.md",
			"content":   "# Contributing\n\nThank you for your interest!",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolWrite, args)
		if result.Kind() != streams.ToolKindModifyFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, result.Kind())
		}
		if result.ModifyFile() == nil {
			t.Fatal("expected ModifyFile to be set")
		}
		if result.ModifyFile().FilePath != "/workspace/CONTRIBUTING.md" {
			t.Errorf("expected FilePath '/workspace/CONTRIBUTING.md', got %q", result.ModifyFile().FilePath)
		}
		if len(result.ModifyFile().Mutations) != 1 {
			t.Fatalf("expected 1 mutation, got %d", len(result.ModifyFile().Mutations))
		}
		mutation := result.ModifyFile().Mutations[0]
		if mutation.Type != streams.MutationCreate {
			t.Errorf("expected MutationType %q, got %q", streams.MutationCreate, mutation.Type)
		}
		if mutation.Content != "# Contributing\n\nThank you for your interest!" {
			t.Errorf("expected Content from content field, got %q", mutation.Content)
		}
	})
}

// TestNormalizerRead tests the normalizer's read handling.
func TestNormalizerRead(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts file_path, offset, limit", func(t *testing.T) {
		args := map[string]any{
			"file_path": "/workspace/config.json",
			"offset":    float64(10),
			"limit":     float64(50),
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolRead, args)
		if result.Kind() != streams.ToolKindReadFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindReadFile, result.Kind())
		}
		if result.ReadFile() == nil {
			t.Fatal("expected ReadFile to be set")
		}
		if result.ReadFile().FilePath != "/workspace/config.json" {
			t.Errorf("expected FilePath '/workspace/config.json', got %q", result.ReadFile().FilePath)
		}
		if result.ReadFile().Offset != 10 {
			t.Errorf("expected Offset 10, got %d", result.ReadFile().Offset)
		}
		if result.ReadFile().Limit != 50 {
			t.Errorf("expected Limit 50, got %d", result.ReadFile().Limit)
		}
	})
}

// TestNormalizerExecute tests the normalizer's execute handling.
func TestNormalizerExecute(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts command and description", func(t *testing.T) {
		args := map[string]any{
			"command":     "ls -la",
			"description": "List files",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolBash, args)
		if result.Kind() != streams.ToolKindShellExec {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindShellExec, result.Kind())
		}
		if result.ShellExec() == nil {
			t.Fatal("expected ShellExec to be set")
		}
		if result.ShellExec().Command != "ls -la" {
			t.Errorf("expected Command 'ls -la', got %q", result.ShellExec().Command)
		}
		if result.ShellExec().Description != "List files" {
			t.Errorf("expected Description 'List files', got %q", result.ShellExec().Description)
		}
	})

	t.Run("handles timeout", func(t *testing.T) {
		args := map[string]any{
			"command": "npm test",
			"timeout": float64(30000),
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolBash, args)
		if result.ShellExec() == nil {
			t.Fatal("expected ShellExec to be set")
		}
		if result.ShellExec().Timeout != 30000 {
			t.Errorf("expected Timeout 30000, got %d", result.ShellExec().Timeout)
		}
	})

	t.Run("handles background flag", func(t *testing.T) {
		args := map[string]any{
			"command":           "npm start",
			"run_in_background": true,
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolBash, args)
		if result.ShellExec() == nil {
			t.Fatal("expected ShellExec to be set")
		}
		if !result.ShellExec().Background {
			t.Error("expected Background to be true")
		}
	})
}

// TestNormalizerCodeSearch tests the normalizer's code search handling.
func TestNormalizerCodeSearch(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("Glob extracts pattern as glob", func(t *testing.T) {
		args := map[string]any{
			"pattern": "**/*.ts",
			"path":    "/workspace",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolGlob, args)
		if result.Kind() != streams.ToolKindCodeSearch {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindCodeSearch, result.Kind())
		}
		if result.CodeSearch() == nil {
			t.Fatal("expected CodeSearch to be set")
		}
		if result.CodeSearch().Glob != "**/*.ts" {
			t.Errorf("expected Glob '**/*.ts', got %q", result.CodeSearch().Glob)
		}
		if result.CodeSearch().Path != "/workspace" {
			t.Errorf("expected Path '/workspace', got %q", result.CodeSearch().Path)
		}
	})

	t.Run("Grep extracts query", func(t *testing.T) {
		args := map[string]any{
			"pattern": "func.*Error",
			"path":    "/workspace",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolGrep, args)
		if result.Kind() != streams.ToolKindCodeSearch {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindCodeSearch, result.Kind())
		}
		if result.CodeSearch() == nil {
			t.Fatal("expected CodeSearch to be set")
		}
		if result.CodeSearch().Pattern != "func.*Error" {
			t.Errorf("expected Pattern 'func.*Error', got %q", result.CodeSearch().Pattern)
		}
	})
}

// TestNormalizerHttpRequest tests the normalizer's HTTP request handling.
func TestNormalizerHttpRequest(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("WebFetch uses GET method", func(t *testing.T) {
		args := map[string]any{
			"url": "https://example.com/api",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolWebFetch, args)
		if result.Kind() != streams.ToolKindHttpRequest {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindHttpRequest, result.Kind())
		}
		if result.HttpRequest() == nil {
			t.Fatal("expected HttpRequest to be set")
		}
		if result.HttpRequest().URL != "https://example.com/api" {
			t.Errorf("expected URL 'https://example.com/api', got %q", result.HttpRequest().URL)
		}
		if result.HttpRequest().Method != "GET" {
			t.Errorf("expected Method 'GET', got %q", result.HttpRequest().Method)
		}
	})

	t.Run("WebSearch uses SEARCH method", func(t *testing.T) {
		args := map[string]any{
			"query": "golang testing best practices",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolWebSearch, args)
		if result.Kind() != streams.ToolKindHttpRequest {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindHttpRequest, result.Kind())
		}
		if result.HttpRequest() == nil {
			t.Fatal("expected HttpRequest to be set")
		}
		if result.HttpRequest().URL != "golang testing best practices" {
			t.Errorf("expected URL 'golang testing best practices', got %q", result.HttpRequest().URL)
		}
		if result.HttpRequest().Method != "SEARCH" {
			t.Errorf("expected Method 'SEARCH', got %q", result.HttpRequest().Method)
		}
	})
}

// TestNormalizerSubagentTask tests the normalizer's subagent task handling.
func TestNormalizerSubagentTask(t *testing.T) {
	normalizer := NewNormalizer()

	args := map[string]any{
		"description":   "Find all test files",
		"prompt":        "Search for test files in the codebase",
		"subagent_type": "Explore",
	}
	result := normalizer.NormalizeToolCall(claudecode.ToolTask, args)
	if result.Kind() != streams.ToolKindSubagentTask {
		t.Errorf("expected Kind %q, got %q", streams.ToolKindSubagentTask, result.Kind())
	}
	if result.SubagentTask() == nil {
		t.Fatal("expected SubagentTask to be set")
	}
	if result.SubagentTask().Description != "Find all test files" {
		t.Errorf("expected Description 'Find all test files', got %q", result.SubagentTask().Description)
	}
	if result.SubagentTask().Prompt != "Search for test files in the codebase" {
		t.Errorf("expected Prompt 'Search for test files in the codebase', got %q", result.SubagentTask().Prompt)
	}
	if result.SubagentTask().SubagentType != "Explore" {
		t.Errorf("expected SubagentType 'Explore', got %q", result.SubagentTask().SubagentType)
	}
}

// TestNormalizerCreateTask tests the normalizer's create task handling.
func TestNormalizerCreateTask(t *testing.T) {
	normalizer := NewNormalizer()

	args := map[string]any{
		"subject":     "Fix authentication bug",
		"description": "The login flow fails when...",
	}
	result := normalizer.NormalizeToolCall(claudecode.ToolTaskCreate, args)
	if result.Kind() != streams.ToolKindCreateTask {
		t.Errorf("expected Kind %q, got %q", streams.ToolKindCreateTask, result.Kind())
	}
	if result.CreateTask() == nil {
		t.Fatal("expected CreateTask to be set")
	}
	if result.CreateTask().Title != "Fix authentication bug" {
		t.Errorf("expected Title 'Fix authentication bug', got %q", result.CreateTask().Title)
	}
	if result.CreateTask().Description != "The login flow fails when..." {
		t.Errorf("expected Description 'The login flow fails when...', got %q", result.CreateTask().Description)
	}
}

// TestNormalizerManageTodos tests the normalizer's manage todos handling.
func TestNormalizerManageTodos(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("TaskUpdate operation", func(t *testing.T) {
		args := map[string]any{
			"taskId": "task-123",
			"status": "completed",
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolTaskUpdate, args)
		if result.Kind() != streams.ToolKindManageTodos {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindManageTodos, result.Kind())
		}
		if result.ManageTodos() == nil {
			t.Fatal("expected ManageTodos to be set")
		}
		if result.ManageTodos().Operation != "update" {
			t.Errorf("expected Operation 'update', got %q", result.ManageTodos().Operation)
		}
	})

	t.Run("TaskList operation", func(t *testing.T) {
		args := map[string]any{}
		result := normalizer.NormalizeToolCall(claudecode.ToolTaskList, args)
		if result.Kind() != streams.ToolKindManageTodos {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindManageTodos, result.Kind())
		}
		if result.ManageTodos() == nil {
			t.Fatal("expected ManageTodos to be set")
		}
		if result.ManageTodos().Operation != "list" {
			t.Errorf("expected Operation 'list', got %q", result.ManageTodos().Operation)
		}
	})

	t.Run("TodoWrite operation with items", func(t *testing.T) {
		args := map[string]any{
			"items": []any{
				map[string]any{
					"id":          "todo-1",
					"description": "Write tests",
					"status":      "pending",
				},
			},
		}
		result := normalizer.NormalizeToolCall(claudecode.ToolTodoWrite, args)
		if result.Kind() != streams.ToolKindManageTodos {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindManageTodos, result.Kind())
		}
		if result.ManageTodos() == nil {
			t.Fatal("expected ManageTodos to be set")
		}
		if result.ManageTodos().Operation != "write" {
			t.Errorf("expected Operation 'write', got %q", result.ManageTodos().Operation)
		}
		if len(result.ManageTodos().Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result.ManageTodos().Items))
		}
		if result.ManageTodos().Items[0].ID != "todo-1" {
			t.Errorf("expected item ID 'todo-1', got %q", result.ManageTodos().Items[0].ID)
		}
	})
}

// TestNormalizerGeneric tests the normalizer's generic handling.
func TestNormalizerGeneric(t *testing.T) {
	normalizer := NewNormalizer()

	args := map[string]any{
		"custom_field": "custom_value",
	}
	result := normalizer.NormalizeToolCall("UnknownTool", args)
	if result.Kind() != streams.ToolKindGeneric {
		t.Errorf("expected Kind %q, got %q", streams.ToolKindGeneric, result.Kind())
	}
	if result.Generic() == nil {
		t.Fatal("expected Generic to be set")
	}
	if result.Generic().Name != "UnknownTool" {
		t.Errorf("expected Name 'UnknownTool', got %q", result.Generic().Name)
	}
}

// TestNormalizerToolResult tests the normalizer's tool result handling.
func TestNormalizerToolResult(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("shell exec result from string", func(t *testing.T) {
		// Use factory function to create payload
		payload := streams.NewShellExec("ls", "", "", 0, false)
		normalizer.NormalizeToolResult(payload, "file1.txt\nfile2.txt")
		if payload.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if payload.ShellExec().Output.Stdout != "file1.txt\nfile2.txt" {
			t.Errorf("expected Stdout 'file1.txt\\nfile2.txt', got %q", payload.ShellExec().Output.Stdout)
		}
	})

	t.Run("shell exec result from map", func(t *testing.T) {
		// Use factory function to create payload
		payload := streams.NewShellExec("npm test", "", "", 0, false)
		normalizer.NormalizeToolResult(payload, map[string]any{
			"stdout":    "All tests passed",
			"stderr":    "",
			"exit_code": float64(0),
		})
		if payload.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if payload.ShellExec().Output.Stdout != "All tests passed" {
			t.Errorf("expected Stdout 'All tests passed', got %q", payload.ShellExec().Output.Stdout)
		}
		if payload.ShellExec().Output.ExitCode != 0 {
			t.Errorf("expected ExitCode 0, got %d", payload.ShellExec().Output.ExitCode)
		}
	})

	t.Run("http request result", func(t *testing.T) {
		// Use factory function to create payload
		payload := streams.NewHttpRequest("https://example.com", "GET")
		normalizer.NormalizeToolResult(payload, "Response body here")
		if payload.HttpRequest().Response != "Response body here" {
			t.Errorf("expected Response 'Response body here', got %q", payload.HttpRequest().Response)
		}
	})

	t.Run("generic result", func(t *testing.T) {
		// Use factory function to create payload
		payload := streams.NewGeneric("CustomTool", nil)
		normalizer.NormalizeToolResult(payload, map[string]any{"data": "value"})
		if payload.Generic().Output == nil {
			t.Fatal("expected Output to be set")
		}
	})
}

// TestSplitLines tests the line splitting utility.
func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty string", "", 0},
		{"single line no newline", "hello", 1},
		{"single line with newline", "hello\n", 2}, // splits to ["hello", ""]
		{"two lines", "hello\nworld", 2},
		{"crlf line endings", "hello\r\nworld", 2},
		{"multiple lines", "a\nb\nc\nd", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shared.SplitLines(tt.input)
			if len(got) != tt.want {
				t.Errorf("shared.SplitLines(%q) returned %d lines, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}
