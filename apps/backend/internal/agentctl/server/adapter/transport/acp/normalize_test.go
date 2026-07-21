package acp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestACPNormalization runs JSONL-driven tests for ACP protocol normalization.
func TestACPNormalization(t *testing.T) {
	testCases := shared.LoadTestCases(t, "acp-messages.jsonl")
	normalizer := NewNormalizer("")

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
			case toolTypeEdit:
				if payload.Kind() != streams.ToolKindModifyFile {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, payload.Kind())
				}
			case toolTypeRead:
				if payload.Kind() != streams.ToolKindReadFile {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindReadFile, payload.Kind())
				}
			case toolTypeExecute:
				if payload.Kind() != streams.ToolKindShellExec {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindShellExec, payload.Kind())
				}
			case toolTypeSearch:
				if payload.Kind() != streams.ToolKindCodeSearch {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindCodeSearch, payload.Kind())
				}
			case toolTypeGeneric:
				if payload.Kind() != streams.ToolKindGeneric {
					t.Errorf("expected Kind %q, got %q", streams.ToolKindGeneric, payload.Kind())
				}
			}
		})
	}
}

// TestNormalizeGeneric_ExcludesAdapterKeys verifies the adapter-injected
// title/meta subagent-detection keys don't leak into a generic (unrecognized)
// tool's client payload — only the real tool args should reach GenericPayload.Input.
func TestNormalizeGeneric_ExcludesAdapterKeys(t *testing.T) {
	n := NewNormalizer("")
	args := map[string]any{
		"kind":      "other",
		"raw_input": map[string]any{"foo": "bar"},
		argKeyTitle: "SomeTool",
		argKeyMeta:  map[string]any{"claudeCode": map[string]any{"toolName": "SomeTool"}},
	}
	payload := n.NormalizeToolCall("SomeTool", args)
	if payload.Kind() != streams.ToolKindGeneric {
		t.Fatalf("Kind = %q, want generic", payload.Kind())
	}
	input, ok := payload.Generic().Input.(map[string]any)
	if !ok {
		t.Fatalf("Generic().Input is not a map: %T", payload.Generic().Input)
	}
	if _, present := input[argKeyTitle]; present {
		t.Errorf("generic input leaked adapter key %q", argKeyTitle)
	}
	if _, present := input[argKeyMeta]; present {
		t.Errorf("generic input leaked adapter key %q", argKeyMeta)
	}
	if _, present := input["raw_input"]; !present {
		t.Error("generic input dropped raw_input")
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
			want:     toolTypeEdit,
		},
		{
			name:     "read kind from args",
			toolKind: "",
			args:     map[string]any{"kind": "read"},
			want:     toolTypeRead,
		},
		{
			name:     "execute kind from args",
			toolKind: "",
			args:     map[string]any{"kind": "execute"},
			want:     toolTypeExecute,
		},
		{
			name:     "edit from toolKind parameter",
			toolKind: "edit",
			args:     map[string]any{},
			want:     toolTypeEdit,
		},
		{
			name:     "read from toolKind parameter",
			toolKind: "read",
			args:     map[string]any{},
			want:     toolTypeRead,
		},
		{
			name:     "view from toolKind parameter",
			toolKind: "view",
			args:     map[string]any{},
			want:     toolTypeRead,
		},
		{
			name:     "bash from toolKind parameter",
			toolKind: "bash",
			args:     map[string]any{},
			want:     toolTypeExecute,
		},
		{
			name:     "run from toolKind parameter",
			toolKind: "run",
			args:     map[string]any{},
			want:     toolTypeExecute,
		},
		{
			name:     "search kind returns tool_search",
			toolKind: "search",
			args:     map[string]any{"kind": "search"},
			want:     toolTypeSearch,
		},
		{
			name:     "unknown kind falls back to tool_call",
			toolKind: "custom_tool",
			args:     map[string]any{"kind": "custom_tool"},
			want:     toolTypeGeneric,
		},
		{
			name:     "empty kind and args falls back to tool_call",
			toolKind: "",
			args:     map[string]any{},
			want:     toolTypeGeneric,
		},
		{
			name:     "args kind takes priority over toolKind",
			toolKind: "read",
			args:     map[string]any{"kind": "edit"},
			want:     toolTypeEdit,
		},
		{
			name:     "read with directory type returns tool_search",
			toolKind: "read",
			args: map[string]any{
				"kind": "read",
				"raw_input": map[string]any{
					"type": "directory",
				},
			},
			want: toolTypeSearch,
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
			got := shared.DetectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestGenerateUnifiedDiff tests the unified diff generation.
func TestGenerateUnifiedDiff(t *testing.T) {
	t.Run("returns empty when both are empty", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("", "", "file.ts", 1)
		if result != "" {
			t.Errorf("expected empty string when both old and new are empty, got %q", result)
		}
	})

	t.Run("returns empty when old equals new", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("same content", "same content", "file.ts", 1)
		if result != "" {
			t.Errorf("expected empty string when old equals new, got %q", result)
		}
	})

	t.Run("generates all-additions diff when old is empty", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("", "new content", "file.ts", 1)
		if result == "" {
			t.Fatal("expected non-empty diff for create operation")
		}
		if !strings.Contains(result, "+new content") {
			t.Error("expected added line in diff")
		}
	})

	t.Run("generates all-deletions diff when new is empty", func(t *testing.T) {
		result := shared.GenerateUnifiedDiff("old content", "", "file.ts", 1)
		if result == "" {
			t.Fatal("expected non-empty diff for delete operation")
		}
		if !strings.Contains(result, "-old content") {
			t.Error("expected removed line in diff")
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
	normalizer := NewNormalizer("")

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
		if result.Kind() != streams.ToolKindModifyFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, result.Kind())
		}
		if result.ModifyFile() == nil {
			t.Fatal("expected ModifyFile to be set")
		}
		if result.ModifyFile().FilePath != "file.ts" {
			t.Errorf("expected FilePath 'file.ts', got %q", result.ModifyFile().FilePath)
		}
		if len(result.ModifyFile().Mutations) != 1 {
			t.Fatalf("expected 1 mutation, got %d", len(result.ModifyFile().Mutations))
		}
		mutation := result.ModifyFile().Mutations[0]
		// OldContent and NewContent are no longer set (only Diff is generated)
		if mutation.Diff == "" {
			t.Error("expected Diff to be generated")
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
		if result.ModifyFile().FilePath != "/workspace/fallback.ts" {
			t.Errorf("expected FilePath '/workspace/fallback.ts', got %q", result.ModifyFile().FilePath)
		}
	})

	t.Run("extracts file_path from raw_input", func(t *testing.T) {
		result := normalizer.NormalizeToolCall("edit", map[string]any{
			"kind": "edit",
			"raw_input": map[string]any{
				"file_path": "/workspace/file.ts",
				"old_str_1": "a",
				"new_str_1": "b",
			},
		})
		if result.ModifyFile().FilePath != "/workspace/file.ts" {
			t.Errorf("expected FilePath '/workspace/file.ts', got %q", result.ModifyFile().FilePath)
		}
	})

	t.Run("extracts filePath from raw_input", func(t *testing.T) {
		result := normalizer.NormalizeToolCall("edit", map[string]any{
			"kind": "edit",
			"raw_input": map[string]any{
				"filePath":  "src/main.go",
				"old_str_1": "a",
				"new_str_1": "b",
			},
		})
		if result.ModifyFile().FilePath != "src/main.go" {
			t.Errorf("expected FilePath 'src/main.go', got %q", result.ModifyFile().FilePath)
		}
	})

	t.Run("extracts omp startLine/endLine into mutation", func(t *testing.T) {
		result := normalizer.NormalizeToolCall("edit", map[string]any{
			"kind": "edit",
			"raw_input": map[string]any{
				"path":      "main.go",
				"old_str_1": "a",
				"new_str_1": "b",
				"startLine": float64(43),
				"endLine":   float64(94),
			},
		})
		m := result.ModifyFile().Mutations[0]
		if m.StartLine != 43 || m.EndLine != 94 {
			t.Errorf("expected StartLine=43 EndLine=94, got %d/%d", m.StartLine, m.EndLine)
		}
	})

	t.Run("still extracts claude old_str line numbers", func(t *testing.T) {
		result := normalizer.NormalizeToolCall("edit", map[string]any{
			"kind": "edit",
			"raw_input": map[string]any{
				"path":                        "main.go",
				"old_str_1":                   "a",
				"new_str_1":                   "b",
				"old_str_start_line_number_1": float64(10),
				"old_str_end_line_number_1":   float64(12),
			},
		})
		m := result.ModifyFile().Mutations[0]
		if m.StartLine != 10 || m.EndLine != 12 {
			t.Errorf("expected StartLine=10 EndLine=12, got %d/%d", m.StartLine, m.EndLine)
		}
	})
}

// TestNormalizerRead tests the normalizer's read handling.
func TestNormalizerRead(t *testing.T) {
	normalizer := NewNormalizer("")

	t.Run("extracts path from raw_input", func(t *testing.T) {
		args := map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": "config.json",
			},
		}
		result := normalizer.NormalizeToolCall("read", args)
		if result.Kind() != streams.ToolKindReadFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindReadFile, result.Kind())
		}
		if result.ReadFile() == nil {
			t.Fatal("expected ReadFile to be set")
		}
		if result.ReadFile().FilePath != "config.json" {
			t.Errorf("expected FilePath 'config.json', got %q", result.ReadFile().FilePath)
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
		if result.ReadFile().FilePath != "/workspace/README.md" {
			t.Errorf("expected FilePath '/workspace/README.md', got %q", result.ReadFile().FilePath)
		}
	})

	t.Run("directory type returns code search", func(t *testing.T) {
		args := map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": ".",
				"type": "directory",
			},
		}
		result := normalizer.NormalizeToolCall("read", args)
		if result.Kind() != streams.ToolKindCodeSearch {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindCodeSearch, result.Kind())
		}
		if result.CodeSearch() == nil {
			t.Fatal("expected CodeSearch to be set")
		}
		if result.CodeSearch().Path != "." {
			t.Errorf("expected Path '.', got %q", result.CodeSearch().Path)
		}
	})

	t.Run("strips omp line selector from path and records range", func(t *testing.T) {
		args := map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": "apps/backend/internal/sentry/handlers.go:43-94",
			},
		}
		result := normalizer.NormalizeToolCall("read", args)
		rf := result.ReadFile()
		if rf == nil {
			t.Fatal("expected ReadFile to be set")
		}
		if rf.FilePath != "apps/backend/internal/sentry/handlers.go" {
			t.Errorf("expected stripped FilePath, got %q", rf.FilePath)
		}
		if rf.Offset != 43 || rf.Limit != 52 {
			t.Errorf("expected Offset=43 Limit=52, got Offset=%d Limit=%d", rf.Offset, rf.Limit)
		}
	})

	t.Run("strips omp selector arriving on a tool_call_update", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{"kind": "read"})
		normalizer.UpdatePayloadInput(payload, map[string]any{"path": "main.go:50+150"}, nil)
		rf := payload.ReadFile()
		if rf == nil {
			t.Fatal("expected ReadFile to be set")
		}
		if rf.FilePath != "main.go" {
			t.Errorf("expected stripped FilePath 'main.go', got %q", rf.FilePath)
		}
		if rf.Offset != 50 || rf.Limit != 150 {
			t.Errorf("expected Offset=50 Limit=150, got Offset=%d Limit=%d", rf.Offset, rf.Limit)
		}
	})

	t.Run("populates range from update even when FilePath already set", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":      "read",
			"raw_input": map[string]any{"path": "main.go"},
		})
		if payload.ReadFile().FilePath != "main.go" {
			t.Fatalf("expected initial FilePath 'main.go', got %q", payload.ReadFile().FilePath)
		}
		normalizer.UpdatePayloadInput(payload, map[string]any{"path": "main.go:50+150"}, nil)
		rf := payload.ReadFile()
		if rf.FilePath != "main.go" {
			t.Errorf("expected FilePath unchanged 'main.go', got %q", rf.FilePath)
		}
		if rf.Offset != 50 || rf.Limit != 150 {
			t.Errorf("expected Offset=50 Limit=150 from update, got Offset=%d Limit=%d", rf.Offset, rf.Limit)
		}
	})

	t.Run("keeps multi-file read path raw so the UI can split it", func(t *testing.T) {
		args := map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": "deployments/values.backupprod.yaml:1-80,deployments/values.au-backupprod.yaml:1-80",
			},
		}
		rf := normalizer.NormalizeToolCall("read", args).ReadFile()
		if rf == nil {
			t.Fatal("expected ReadFile to be set")
		}
		// Multiple files cannot be represented by a single Offset/Limit; keep the
		// raw comma-joined path verbatim and let the frontend render one link per
		// file. Offset/Limit stay zero (no single top-level range).
		if rf.FilePath != "deployments/values.backupprod.yaml:1-80,deployments/values.au-backupprod.yaml:1-80" {
			t.Errorf("expected raw multi-file FilePath preserved, got %q", rf.FilePath)
		}
		if rf.Offset != 0 || rf.Limit != 0 {
			t.Errorf("expected Offset=0 Limit=0 for multi-file read, got Offset=%d Limit=%d", rf.Offset, rf.Limit)
		}
	})

	t.Run("keeps multi-file read path raw arriving on a tool_call_update", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{"kind": "read"})
		normalizer.UpdatePayloadInput(payload, map[string]any{"path": "a/x.go:1-10,a/y.go:1-10"}, nil)
		rf := payload.ReadFile()
		if rf.FilePath != "a/x.go:1-10,a/y.go:1-10" {
			t.Errorf("expected raw multi-file FilePath preserved, got %q", rf.FilePath)
		}
		if rf.Offset != 0 || rf.Limit != 0 {
			t.Errorf("expected Offset=0 Limit=0 for multi-file read, got Offset=%d Limit=%d", rf.Offset, rf.Limit)
		}
	})
}

// TestNormalizerResult tests the normalizer's result handling.
func TestNormalizerResult(t *testing.T) {
	normalizer := NewNormalizer("")

	t.Run("handles read file result", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": "file.txt",
			},
		})

		normalizer.NormalizeToolResult(payload, map[string]any{
			"rawOutput": map[string]any{
				"output": "line 1\nline 2\nline 3",
			},
		})

		if payload.ReadFile().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if payload.ReadFile().Output.Content != "line 1\nline 2\nline 3" {
			t.Errorf("expected Content, got %q", payload.ReadFile().Output.Content)
		}
		if payload.ReadFile().Output.LineCount != 3 {
			t.Errorf("expected LineCount 3, got %d", payload.ReadFile().Output.LineCount)
		}
	})

	t.Run("handles directory listing result", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": ".",
				"type": "directory",
			},
		})

		normalizer.NormalizeToolResult(payload, map[string]any{
			"rawOutput": map[string]any{
				"output": "Here's the files:\n./file1.ts\n./file2.go\n./src/main.ts",
			},
		})

		if payload.CodeSearch().Output == nil {
			t.Fatal("expected Output to be set")
		}
		// Should skip "Here's the files:" header
		if payload.CodeSearch().Output.FileCount != 3 {
			t.Errorf("expected FileCount 3, got %d", payload.CodeSearch().Output.FileCount)
		}
	})

	t.Run("handles shell execution result", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("execute", map[string]any{
			"kind": "execute",
			"raw_input": map[string]any{
				"command": "pwd",
				"cwd":     ".",
			},
		})

		normalizer.NormalizeToolResult(payload, map[string]any{
			"rawOutput": map[string]any{
				"output": "Here are the results from executing the command.\n<return-code>\n0\n</return-code>\n<output>\n/Users/cfl/project\n\n</output>",
			},
		})

		if payload.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		require.NotNil(t, payload.ShellExec().Output.ExitCode)
		require.Equal(t, 0, *payload.ShellExec().Output.ExitCode)
		if payload.ShellExec().Output.Stdout != "/Users/cfl/project" {
			t.Errorf("expected Stdout '/Users/cfl/project', got %q", payload.ShellExec().Output.Stdout)
		}
	})

	t.Run("handles shell execution with stderr", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("execute", map[string]any{
			"kind": "execute",
			"raw_input": map[string]any{
				"command": "cat nonexistent",
			},
		})

		normalizer.NormalizeToolResult(payload, map[string]any{
			"rawOutput": map[string]any{
				"output": "<return-code>\n1\n</return-code>\n<output>\n</output>\n<stderr>\ncat: nonexistent: No such file or directory\n</stderr>",
			},
		})

		if payload.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		require.NotNil(t, payload.ShellExec().Output.ExitCode)
		require.Equal(t, 1, *payload.ShellExec().Output.ExitCode)
		if payload.ShellExec().Output.Stderr != "cat: nonexistent: No such file or directory" {
			t.Errorf("expected Stderr, got %q", payload.ShellExec().Output.Stderr)
		}
	})
}

// TestNormalizerExecute tests the normalizer's execute handling.
func TestNormalizerExecute(t *testing.T) {
	normalizer := NewNormalizer("")

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
		if result.Kind() != streams.ToolKindShellExec {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindShellExec, result.Kind())
		}
		if result.ShellExec() == nil {
			t.Fatal("expected ShellExec to be set")
		}
		if result.ShellExec().Command != "npm test" {
			t.Errorf("expected Command 'npm test', got %q", result.ShellExec().Command)
		}
		if result.ShellExec().WorkDir != "/workspace" {
			t.Errorf("expected WorkDir '/workspace', got %q", result.ShellExec().WorkDir)
		}
		if result.ShellExec().Timeout != 30 {
			t.Errorf("expected Timeout 30, got %d", result.ShellExec().Timeout)
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
		if !result.ShellExec().Background {
			t.Error("expected Background to be true when wait is false")
		}
	})
}

// TestParseShellOutputPlainString tests that plain string output (Claude Code format)
// is correctly treated as stdout.
func TestParseShellOutputPlainString(t *testing.T) {
	normalizer := NewNormalizer("")

	t.Run("plain string rawOutput becomes stdout", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("execute", map[string]any{
			"kind":      "execute",
			"raw_input": map[string]any{"command": "pwd"},
		})

		// Claude Code sends rawOutput as a direct string, not XML-wrapped
		normalizer.NormalizeToolResult(payload, "/Users/cfl/Projects/1code")

		if payload.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if payload.ShellExec().Output.Stdout != "/Users/cfl/Projects/1code" {
			t.Errorf("expected Stdout '/Users/cfl/Projects/1code', got %q", payload.ShellExec().Output.Stdout)
		}
		require.Nil(t, payload.ShellExec().Output.ExitCode)
	})

	t.Run("XML-wrapped output still works", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("execute", map[string]any{
			"kind":      "execute",
			"raw_input": map[string]any{"command": "ls"},
		})

		normalizer.NormalizeToolResult(payload, map[string]any{
			"rawOutput": map[string]any{
				"output": "<return-code>\n0\n</return-code>\n<output>\nfile1.txt\nfile2.txt\n</output>",
			},
		})

		if payload.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if payload.ShellExec().Output.Stdout != "file1.txt\nfile2.txt" {
			t.Errorf("expected Stdout 'file1.txt\\nfile2.txt', got %q", payload.ShellExec().Output.Stdout)
		}
	})
}

// TestUpdatePayloadInput tests incremental rawInput updates (Claude Code pattern).
func TestUpdatePayloadInput(t *testing.T) {
	normalizer := NewNormalizer("")

	t.Run("updates empty command from rawInput", func(t *testing.T) {
		// Initial tool_call with empty rawInput (Claude Code sends this)
		payload := normalizer.NormalizeToolCall("execute", map[string]any{
			"kind":      "execute",
			"raw_input": map[string]any{},
		})

		if payload.ShellExec().Command != "" {
			t.Errorf("expected empty initial Command, got %q", payload.ShellExec().Command)
		}

		// Subsequent tool_call_update provides the actual command
		normalizer.UpdatePayloadInput(payload, map[string]any{
			"command":     "pwd",
			"description": "Print working directory",
		}, nil)

		if payload.ShellExec().Command != "pwd" {
			t.Errorf("expected Command 'pwd', got %q", payload.ShellExec().Command)
		}
		if payload.ShellExec().Description != "Print working directory" {
			t.Errorf("expected Description 'Print working directory', got %q", payload.ShellExec().Description)
		}
	})

	t.Run("does not overwrite existing command", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("execute", map[string]any{
			"kind": "execute",
			"raw_input": map[string]any{
				"command": "ls -la",
			},
		})

		normalizer.UpdatePayloadInput(payload, map[string]any{
			"command": "pwd",
		}, nil)

		if payload.ShellExec().Command != "ls -la" {
			t.Errorf("expected Command 'ls -la' (unchanged), got %q", payload.ShellExec().Command)
		}
	})

	t.Run("handles nil payload gracefully", func(t *testing.T) {
		// Should not panic
		normalizer.UpdatePayloadInput(nil, map[string]any{"command": "pwd"}, nil)
	})

	t.Run("handles non-map rawInput gracefully", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("execute", map[string]any{
			"kind":      "execute",
			"raw_input": map[string]any{},
		})
		// Should not panic
		normalizer.UpdatePayloadInput(payload, "not a map", nil)
	})

	t.Run("generic input merges without overwriting existing rawInput keys", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("Monitor", map[string]any{
			"kind": "other",
			"raw_input": map[string]any{
				"command": "tail -f old.log",
				"timeout": "",
			},
		})

		normalizer.UpdatePayloadInput(payload, map[string]any{
			"command": "tail -f new.log",
			"timeout": "60s",
			"cwd":     "/tmp",
		}, nil)

		rawInput := payload.Generic().Input.(map[string]any)["raw_input"].(map[string]any)
		require.Equal(t, "tail -f old.log", rawInput["command"])
		require.Equal(t, "60s", rawInput["timeout"])
		require.Equal(t, "/tmp", rawInput["cwd"])
	})

	t.Run("generic input fills empty supplemental keys and skips non-empty ones", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("Monitor", map[string]any{
			"kind":      "other",
			"raw_input": map[string]any{},
			"path":      "/existing/path",
		})

		normalizer.UpdatePayloadInput(payload, nil, map[string]any{
			"path":  "/new/path",
			"title": "My Monitor",
		})

		args := payload.Generic().Input.(map[string]any)
		require.Equal(t, "/existing/path", args["path"])
		require.Equal(t, "My Monitor", args["title"])
	})
}

// TestEnrichModifyFileFromContents tests enriching modify_file payloads from tool_call_contents.
func TestEnrichModifyFileFromContents(t *testing.T) {
	oldText := "old content"

	t.Run("enriches file path from diff content", func(t *testing.T) {
		mf := &streams.ModifyFilePayload{
			FilePath:  "",
			Mutations: []streams.FileMutation{{Type: streams.MutationPatch}},
		}
		contents := []acp.ToolCallContent{
			{Diff: &acp.ToolCallContentDiff{Path: "/workspace/README.md", NewText: "new content"}},
		}
		enrichModifyFileFromContents(mf, contents)
		if mf.FilePath != "/workspace/README.md" {
			t.Errorf("expected FilePath '/workspace/README.md', got %q", mf.FilePath)
		}
	})

	t.Run("generates unified diff when oldText is provided", func(t *testing.T) {
		mf := &streams.ModifyFilePayload{
			FilePath:  "",
			Mutations: []streams.FileMutation{{Type: streams.MutationPatch}},
		}
		contents := []acp.ToolCallContent{
			{Diff: &acp.ToolCallContentDiff{Path: "file.ts", OldText: &oldText, NewText: "new content"}},
		}
		enrichModifyFileFromContents(mf, contents)
		if mf.Mutations[0].Diff == "" {
			t.Error("expected Diff to be generated")
		}
		if mf.Mutations[0].Type != streams.MutationPatch {
			t.Errorf("expected type to remain patch, got %q", mf.Mutations[0].Type)
		}
		if !strings.Contains(mf.Mutations[0].Diff, "-old content") {
			t.Error("expected diff to contain removed old content")
		}
		if !strings.Contains(mf.Mutations[0].Diff, "+new content") {
			t.Error("expected diff to contain added new content")
		}
	})

	t.Run("sets create mutation when only newText", func(t *testing.T) {
		mf := &streams.ModifyFilePayload{
			FilePath:  "",
			Mutations: []streams.FileMutation{{Type: streams.MutationPatch}},
		}
		contents := []acp.ToolCallContent{
			{Diff: &acp.ToolCallContentDiff{Path: "file.ts", NewText: "new file content"}},
		}
		enrichModifyFileFromContents(mf, contents)
		if mf.Mutations[0].Type != streams.MutationCreate {
			t.Errorf("expected type 'create', got %q", mf.Mutations[0].Type)
		}
		if mf.Mutations[0].Content != "new file content" {
			t.Errorf("expected content 'new file content', got %q", mf.Mutations[0].Content)
		}
	})

	t.Run("does not overwrite existing file path", func(t *testing.T) {
		mf := &streams.ModifyFilePayload{
			FilePath:  "existing.ts",
			Mutations: []streams.FileMutation{{Type: streams.MutationPatch}},
		}
		contents := []acp.ToolCallContent{
			{Diff: &acp.ToolCallContentDiff{Path: "other.ts", NewText: "content"}},
		}
		enrichModifyFileFromContents(mf, contents)
		if mf.FilePath != "existing.ts" {
			t.Errorf("expected FilePath to remain 'existing.ts', got %q", mf.FilePath)
		}
	})

	t.Run("does not overwrite existing diff", func(t *testing.T) {
		mf := &streams.ModifyFilePayload{
			FilePath:  "file.ts",
			Mutations: []streams.FileMutation{{Type: streams.MutationPatch, Diff: "existing diff"}},
		}
		contents := []acp.ToolCallContent{
			{Diff: &acp.ToolCallContentDiff{Path: "file.ts", OldText: &oldText, NewText: "new"}},
		}
		enrichModifyFileFromContents(mf, contents)
		if mf.Mutations[0].Diff != "existing diff" {
			t.Errorf("expected Diff to remain 'existing diff', got %q", mf.Mutations[0].Diff)
		}
	})

	t.Run("handles empty content gracefully", func(t *testing.T) {
		mf := &streams.ModifyFilePayload{
			FilePath:  "",
			Mutations: []streams.FileMutation{{Type: streams.MutationPatch}},
		}
		enrichModifyFileFromContents(mf, nil)
		enrichModifyFileFromContents(mf, []acp.ToolCallContent{})
		if mf.FilePath != "" {
			t.Errorf("expected FilePath to remain empty, got %q", mf.FilePath)
		}
	})
}

// TestUpdatePayloadInput_ModifyFile tests incremental rawInput updates for modify_file payloads.
func TestUpdatePayloadInput_ModifyFile(t *testing.T) {
	normalizer := NewNormalizer("")

	t.Run("updates empty file path from rawInput", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("edit", map[string]any{
			"kind":      "edit",
			"raw_input": map[string]any{},
		})
		if payload.ModifyFile().FilePath != "" {
			t.Errorf("expected empty initial FilePath, got %q", payload.ModifyFile().FilePath)
		}
		normalizer.UpdatePayloadInput(payload, map[string]any{
			"file_path": "/workspace/file.ts",
		}, nil)
		if payload.ModifyFile().FilePath != "/workspace/file.ts" {
			t.Errorf("expected FilePath '/workspace/file.ts', got %q", payload.ModifyFile().FilePath)
		}
	})

	t.Run("does not overwrite existing file path", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("edit", map[string]any{
			"kind": "edit",
			"raw_input": map[string]any{
				"path": "existing.ts",
			},
		})
		normalizer.UpdatePayloadInput(payload, map[string]any{
			"file_path": "other.ts",
		}, nil)
		if payload.ModifyFile().FilePath != "existing.ts" {
			t.Errorf("expected FilePath to remain 'existing.ts', got %q", payload.ModifyFile().FilePath)
		}
	})

	t.Run("updates empty file path from supplemental locations", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("edit", map[string]any{
			"kind":      "edit",
			"raw_input": map[string]any{},
		})
		normalizer.UpdatePayloadInput(payload, nil, map[string]any{
			keyLocations: []map[string]any{
				{keyPath: "/workspace/file.ts"},
			},
		})
		if payload.ModifyFile().FilePath != "/workspace/file.ts" {
			t.Errorf("expected FilePath '/workspace/file.ts', got %q", payload.ModifyFile().FilePath)
		}
	})
}

// TestUpdatePayloadInput_ReadFile tests incremental rawInput updates for read_file payloads.
func TestUpdatePayloadInput_ReadFile(t *testing.T) {
	normalizer := NewNormalizer("")

	t.Run("updates empty file path from rawInput", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":      "read",
			"raw_input": map[string]any{},
		})
		if payload.ReadFile().FilePath != "" {
			t.Errorf("expected empty initial FilePath, got %q", payload.ReadFile().FilePath)
		}
		normalizer.UpdatePayloadInput(payload, map[string]any{
			"file_path": "/workspace/README.md",
		}, nil)
		if payload.ReadFile().FilePath != "/workspace/README.md" {
			t.Errorf("expected FilePath '/workspace/README.md', got %q", payload.ReadFile().FilePath)
		}
	})

	t.Run("does not overwrite existing file path", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": "existing.md",
			},
		})
		normalizer.UpdatePayloadInput(payload, map[string]any{
			"file_path": "other.md",
		}, nil)
		if payload.ReadFile().FilePath != "existing.md" {
			t.Errorf("expected FilePath to remain 'existing.md', got %q", payload.ReadFile().FilePath)
		}
	})

	t.Run("updates empty file path from filePath rawInput", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":      "read",
			"raw_input": map[string]any{},
		})
		normalizer.UpdatePayloadInput(payload, map[string]any{
			"filePath": "/workspace/src/main.go",
		}, nil)
		if payload.ReadFile().FilePath != "/workspace/src/main.go" {
			t.Errorf("expected FilePath '/workspace/src/main.go', got %q", payload.ReadFile().FilePath)
		}
	})

	t.Run("updates empty file path from supplemental locations", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":      "read",
			"raw_input": map[string]any{},
		})
		normalizer.UpdatePayloadInput(payload, nil, map[string]any{
			"locations": []any{
				map[string]any{"path": "/workspace/README.md"},
			},
		})
		if payload.ReadFile().FilePath != "/workspace/README.md" {
			t.Errorf("expected FilePath '/workspace/README.md', got %q", payload.ReadFile().FilePath)
		}
	})

	t.Run("updates empty file path from adapter supplemental locations shape", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":      "read",
			"raw_input": map[string]any{},
		})
		// toolCallUpdateSupplemental builds []map[string]any, not []any.
		normalizer.UpdatePayloadInput(payload, nil, map[string]any{
			"locations": []map[string]any{
				{"path": "/workspace/README.md", "line": float64(1)},
			},
		})
		if payload.ReadFile().FilePath != "/workspace/README.md" {
			t.Errorf("expected FilePath '/workspace/README.md', got %q", payload.ReadFile().FilePath)
		}
	})

	t.Run("switches stale single-file state to raw multi-file path", func(t *testing.T) {
		// An earlier update streamed a single-file path with a range; a later
		// update carries the full multi-file path. The payload must drop the
		// stale single-file Offset/Limit and keep the raw comma-joined path for
		// the UI to split (mirrors normalizeRead).
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind": "read",
			"raw_input": map[string]any{
				"path": "a.go:1-10",
			},
		})
		if rf := payload.ReadFile(); rf.FilePath != "a.go" || rf.Offset != 1 || rf.Limit != 10 {
			t.Fatalf("unexpected initial state: %+v", rf)
		}
		normalizer.UpdatePayloadInput(payload, map[string]any{
			"file_path": "a.go:1-10,b.go:1-10",
		}, nil)
		rf := payload.ReadFile()
		if rf.FilePath != "a.go:1-10,b.go:1-10" {
			t.Errorf("expected raw multi-file FilePath, got %q", rf.FilePath)
		}
		if rf.Offset != 0 || rf.Limit != 0 {
			t.Errorf("expected Offset/Limit cleared, got Offset=%d Limit=%d", rf.Offset, rf.Limit)
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
				t.Errorf("SplitLines(%q) returned %d lines, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}
