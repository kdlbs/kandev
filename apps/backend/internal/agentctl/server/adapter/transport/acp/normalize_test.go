package acp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestACPNormalization runs JSONL-driven tests for ACP protocol normalization.
func TestACPNormalization(t *testing.T) {
	testCases := shared.LoadTestCases(t, "acp-messages.jsonl")
	normalizer := NewNormalizer()

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("line_%d", i+1), func(t *testing.T) {
			args, _ := tc.Input["args"].(map[string]any)

			// Detect tool type using the kind field (as the ACP adapter does)
			kind, _ := args["kind"].(string)
			toolType := DetectToolOperationType(kind, args)
			expectedToolType, _ := tc.Expected["tool_type"].(string)
			if toolType != expectedToolType {
				t.Errorf("tool type mismatch: got %q, want %q", toolType, expectedToolType)
			}

			// Normalize using the typed normalizer
			payload := normalizer.NormalizeToolCall(kind, args)

			// Verify the Kind is set correctly based on tool type
			switch toolType {
			case "tool_edit":
				if payload.Kind != streams.ToolKindModifyFile {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, payload.Kind)
				}
			case "tool_read":
				if payload.Kind != streams.ToolKindReadFile {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindReadFile, payload.Kind)
				}
			case "tool_execute":
				if payload.Kind != streams.ToolKindShellExec {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindShellExec, payload.Kind)
				}
			}
		})
	}
}

// TestDetectToolOperationType tests the ACP tool type detection function.
func TestDetectToolOperationType(t *testing.T) {
	tests := []struct {
		name     string
		toolKind string
		args     map[string]any
		want     string
	}{
		{
			name:     "edit kind from args",
			toolKind: "",
			args:     map[string]any{"kind": "edit"},
			want:     "tool_edit",
		},
		{
			name:     "read kind from args",
			toolKind: "",
			args:     map[string]any{"kind": "read"},
			want:     "tool_read",
		},
		{
			name:     "execute kind from args",
			toolKind: "",
			args:     map[string]any{"kind": "execute"},
			want:     "tool_execute",
		},
		{
			name:     "edit from toolKind parameter",
			toolKind: "edit",
			args:     map[string]any{},
			want:     "tool_edit",
		},
		{
			name:     "read from toolKind parameter",
			toolKind: "read",
			args:     map[string]any{},
			want:     "tool_read",
		},
		{
			name:     "view from toolKind parameter",
			toolKind: "view",
			args:     map[string]any{},
			want:     "tool_read",
		},
		{
			name:     "bash from toolKind parameter",
			toolKind: "bash",
			args:     map[string]any{},
			want:     "tool_execute",
		},
		{
			name:     "run from toolKind parameter",
			toolKind: "run",
			args:     map[string]any{},
			want:     "tool_execute",
		},
		{
			name:     "unknown kind falls back to tool_call",
			toolKind: "search",
			args:     map[string]any{"kind": "search"},
			want:     "tool_call",
		},
		{
			name:     "empty kind and args falls back to tool_call",
			toolKind: "",
			args:     map[string]any{},
			want:     "tool_call",
		},
		{
			name:     "args kind takes priority over toolKind",
			toolKind: "read",
			args:     map[string]any{"kind": "edit"},
			want:     "tool_edit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectToolOperationType(tt.toolKind, tt.args)
			if got != tt.want {
				t.Errorf("DetectToolOperationType(%q, %v) = %q, want %q", tt.toolKind, tt.args, got, tt.want)
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
			got := detectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestGenerateUnifiedDiff tests the unified diff generation.
func TestGenerateUnifiedDiff(t *testing.T) {
	t.Run("returns empty when old is empty", func(t *testing.T) {
		result := generateUnifiedDiff("", "new content", "file.ts", 1, 1)
		if result != "" {
			t.Errorf("expected empty string for empty old string, got %q", result)
		}
	})

	t.Run("returns empty when new is empty", func(t *testing.T) {
		result := generateUnifiedDiff("old content", "", "file.ts", 1, 1)
		if result != "" {
			t.Errorf("expected empty string for empty new string, got %q", result)
		}
	})

	t.Run("generates diff with header and hunks", func(t *testing.T) {
		result := generateUnifiedDiff("const x = 1;", "const x = 2;", "file.ts", 5, 5)
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
		result := generateUnifiedDiff("a", "b", "file.go", 0, 0)
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

	t.Run("extracts fields from raw_input", func(t *testing.T) {
		args := map[string]any{
			"kind": "edit",
			"raw_input": map[string]any{
				"old_str_1":                   "old code",
				"new_str_1":                   "new code",
				"path":                        "file.ts",
				"old_str_start_line_number_1": float64(10),
				"old_str_end_line_number_1":   float64(15),
			},
		}

		result := normalizer.NormalizeToolCall("edit", args)
		if result.Kind != streams.ToolKindModifyFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, result.Kind)
		}
		if result.ModifyFile == nil {
			t.Fatal("expected ModifyFile to be set")
		}
		if result.ModifyFile.FilePath != "file.ts" {
			t.Errorf("expected FilePath 'file.ts', got %q", result.ModifyFile.FilePath)
		}
		if len(result.ModifyFile.Mutations) != 1 {
			t.Fatalf("expected 1 mutation, got %d", len(result.ModifyFile.Mutations))
		}
		mutation := result.ModifyFile.Mutations[0]
		if mutation.OldContent != "old code" {
			t.Errorf("expected OldContent 'old code', got %q", mutation.OldContent)
		}
		if mutation.NewContent != "new code" {
			t.Errorf("expected NewContent 'new code', got %q", mutation.NewContent)
		}
		if mutation.StartLine != 10 {
			t.Errorf("expected StartLine 10, got %d", mutation.StartLine)
		}
		if mutation.EndLine != 15 {
			t.Errorf("expected EndLine 15, got %d", mutation.EndLine)
		}
	})

	t.Run("falls back to locations for path", func(t *testing.T) {
		args := map[string]any{
			"kind": "edit",
			"locations": []any{
				map[string]any{"path": "/workspace/fallback.ts"},
			},
			"raw_input": map[string]any{
				"old_str_1": "a",
				"new_str_1": "b",
				"path":      "",
			},
		}

		result := normalizer.NormalizeToolCall("edit", args)
		if result.ModifyFile.FilePath != "/workspace/fallback.ts" {
			t.Errorf("expected FilePath '/workspace/fallback.ts', got %q", result.ModifyFile.FilePath)
		}
	})
}

// TestNormalizerRead tests the normalizer's read handling.
func TestNormalizerRead(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts path from raw_input", func(t *testing.T) {
		args := map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": "config.json",
			},
		}
		result := normalizer.NormalizeToolCall("read", args)
		if result.Kind != streams.ToolKindReadFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindReadFile, result.Kind)
		}
		if result.ReadFile == nil {
			t.Fatal("expected ReadFile to be set")
		}
		if result.ReadFile.FilePath != "config.json" {
			t.Errorf("expected FilePath 'config.json', got %q", result.ReadFile.FilePath)
		}
	})

	t.Run("falls back to locations for path", func(t *testing.T) {
		args := map[string]any{
			"kind": "read",
			"locations": []any{
				map[string]any{"path": "/workspace/README.md"},
			},
			"raw_input": map[string]any{
				"path": "",
			},
		}
		result := normalizer.NormalizeToolCall("read", args)
		if result.ReadFile.FilePath != "/workspace/README.md" {
			t.Errorf("expected FilePath '/workspace/README.md', got %q", result.ReadFile.FilePath)
		}
	})
}

// TestNormalizerExecute tests the normalizer's execute handling.
func TestNormalizerExecute(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts command, cwd, timeout", func(t *testing.T) {
		args := map[string]any{
			"kind": "execute",
			"raw_input": map[string]any{
				"command":          "npm test",
				"cwd":              "/workspace",
				"max_wait_seconds": float64(30),
			},
		}
		result := normalizer.NormalizeToolCall("execute", args)
		if result.Kind != streams.ToolKindShellExec {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindShellExec, result.Kind)
		}
		if result.ShellExec == nil {
			t.Fatal("expected ShellExec to be set")
		}
		if result.ShellExec.Command != "npm test" {
			t.Errorf("expected Command 'npm test', got %q", result.ShellExec.Command)
		}
		if result.ShellExec.WorkDir != "/workspace" {
			t.Errorf("expected WorkDir '/workspace', got %q", result.ShellExec.WorkDir)
		}
		if result.ShellExec.Timeout != 30 {
			t.Errorf("expected Timeout 30, got %d", result.ShellExec.Timeout)
		}
	})

	t.Run("handles background flag", func(t *testing.T) {
		args := map[string]any{
			"kind": "execute",
			"raw_input": map[string]any{
				"command": "npm start",
				"wait":    false,
			},
		}
		result := normalizer.NormalizeToolCall("execute", args)
		if !result.ShellExec.Background {
			t.Error("expected Background to be true when wait is false")
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
			got := splitLines(tt.input)
			if len(got) != tt.want {
				t.Errorf("splitLines(%q) returned %d lines, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}
