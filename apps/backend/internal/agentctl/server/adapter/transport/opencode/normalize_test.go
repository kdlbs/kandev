package opencode

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestNormalizerBash tests the normalizer's bash handling.
func TestNormalizerBash(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts command, description, timeout, background", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"command":     "ls -la",
				"description": "List files",
				"timeout":     float64(30000),
				"background":  true,
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolBash, state)
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
		if result.ShellExec().Timeout != 30000 {
			t.Errorf("expected Timeout 30000, got %d", result.ShellExec().Timeout)
		}
		if !result.ShellExec().Background {
			t.Error("expected Background to be true")
		}
	})

	t.Run("includes output with exit code from metadata", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"command": "echo hello",
			},
			Output: "hello",
			Metadata: map[string]any{
				"exit": float64(0),
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolBash, state)
		if result.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if result.ShellExec().Output.Stdout != "hello" {
			t.Errorf("expected Stdout 'hello', got %q", result.ShellExec().Output.Stdout)
		}
		if result.ShellExec().Output.ExitCode != 0 {
			t.Errorf("expected ExitCode 0, got %d", result.ShellExec().Output.ExitCode)
		}
	})

	t.Run("handles nil state gracefully", func(t *testing.T) {
		result := normalizer.NormalizeToolCall(OpenCodeToolBash, nil)
		if result.Kind() != streams.ToolKindShellExec {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindShellExec, result.Kind())
		}
	})
}

// TestNormalizerEdit tests the normalizer's edit handling.
func TestNormalizerEdit(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts path from input directly", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"path": "/workspace/file.ts",
				"diff": "@@ -1,1 +1,1 @@\n-old\n+new\n",
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolEdit, state)
		if result.Kind() != streams.ToolKindModifyFile {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindModifyFile, result.Kind())
		}
		if result.ModifyFile() == nil {
			t.Fatal("expected ModifyFile to be set")
		}
		if result.ModifyFile().FilePath != "/workspace/file.ts" {
			t.Errorf("expected FilePath '/workspace/file.ts', got %q", result.ModifyFile().FilePath)
		}
		if len(result.ModifyFile().Mutations) != 1 {
			t.Fatalf("expected 1 mutation, got %d", len(result.ModifyFile().Mutations))
		}
		if result.ModifyFile().Mutations[0].Diff != "@@ -1,1 +1,1 @@\n-old\n+new\n" {
			t.Error("expected diff to be preserved")
		}
	})

	t.Run("extracts path from nested input map", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"input": map[string]any{
					"path": "/workspace/nested.ts",
				},
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolEdit, state)
		if result.ModifyFile().FilePath != "/workspace/nested.ts" {
			t.Errorf("expected FilePath '/workspace/nested.ts', got %q", result.ModifyFile().FilePath)
		}
	})

	t.Run("falls back to file_path key", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"file_path": "/workspace/fallback.ts",
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolEdit, state)
		if result.ModifyFile().FilePath != "/workspace/fallback.ts" {
			t.Errorf("expected FilePath '/workspace/fallback.ts', got %q", result.ModifyFile().FilePath)
		}
	})

	t.Run("creates empty mutation when no diff", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"path": "/workspace/file.ts",
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolEdit, state)
		if len(result.ModifyFile().Mutations) != 1 {
			t.Fatalf("expected 1 mutation, got %d", len(result.ModifyFile().Mutations))
		}
		if result.ModifyFile().Mutations[0].Type != streams.MutationPatch {
			t.Errorf("expected MutationType %q, got %q", streams.MutationPatch, result.ModifyFile().Mutations[0].Type)
		}
	})
}

// TestNormalizerWebFetch tests the normalizer's webfetch handling.
func TestNormalizerWebFetch(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts URL and uses GET method", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"url": "https://example.com/api",
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolWebFetch, state)
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

	t.Run("includes response when output available", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"url": "https://example.com",
			},
			Output: "Response body content",
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolWebFetch, state)
		if result.HttpRequest().Response != "Response body content" {
			t.Errorf("expected Response 'Response body content', got %q", result.HttpRequest().Response)
		}
	})
}

// TestNormalizerGlob tests the normalizer's glob handling.
func TestNormalizerGlob(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts pattern and path", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"pattern": "**/*.ts",
				"path":    "/workspace",
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolGlob, state)
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

	t.Run("parses file list from output", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"pattern": "**/*.ts",
			},
			Output: "file1.ts\nfile2.ts\nsrc/file3.ts",
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolGlob, state)
		if result.CodeSearch().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if len(result.CodeSearch().Output.Files) != 3 {
			t.Errorf("expected 3 files, got %d", len(result.CodeSearch().Output.Files))
		}
	})

	t.Run("filters out truncation message from output", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"pattern": "**/*",
			},
			Output: "file1.ts\n(Results are truncated...)",
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolGlob, state)
		if len(result.CodeSearch().Output.Files) != 1 {
			t.Errorf("expected 1 file (truncation msg filtered), got %d", len(result.CodeSearch().Output.Files))
		}
	})

	t.Run("extracts count and truncated from metadata", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"pattern": "**/*",
			},
			Output: "file1.ts",
			Metadata: map[string]any{
				"count":     float64(100),
				"truncated": true,
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolGlob, state)
		if result.CodeSearch().Output.FileCount != 100 {
			t.Errorf("expected FileCount 100, got %d", result.CodeSearch().Output.FileCount)
		}
		if !result.CodeSearch().Output.Truncated {
			t.Error("expected Truncated to be true")
		}
	})
}

// TestNormalizerGrep tests the normalizer's grep handling.
func TestNormalizerGrep(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts pattern, path, and query", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"pattern": "func.*Error",
				"path":    "/workspace",
				"query":   "error handling",
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolGrep, state)
		if result.Kind() != streams.ToolKindCodeSearch {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindCodeSearch, result.Kind())
		}
		if result.CodeSearch() == nil {
			t.Fatal("expected CodeSearch to be set")
		}
		if result.CodeSearch().Pattern != "func.*Error" {
			t.Errorf("expected Pattern 'func.*Error', got %q", result.CodeSearch().Pattern)
		}
		if result.CodeSearch().Query != "error handling" {
			t.Errorf("expected Query 'error handling', got %q", result.CodeSearch().Query)
		}
	})

	t.Run("parses grep output with file:line format", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"pattern": "func",
			},
			Output: "file1.go:10:func main()\nfile1.go:20:func helper()\nfile2.go:5:func test()",
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolGrep, state)
		if result.CodeSearch().Output == nil {
			t.Fatal("expected Output to be set")
		}
		// Should deduplicate files
		if result.CodeSearch().Output.FileCount != 2 {
			t.Errorf("expected FileCount 2 (deduplicated), got %d", result.CodeSearch().Output.FileCount)
		}
	})

	t.Run("handles files-only output", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"pattern": "test",
			},
			Output: "file1.go\nfile2.go\nfile3.go",
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolGrep, state)
		if len(result.CodeSearch().Output.Files) != 3 {
			t.Errorf("expected 3 files, got %d", len(result.CodeSearch().Output.Files))
		}
	})
}

// TestNormalizerRead tests the normalizer's read handling.
func TestNormalizerRead(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("extracts path, offset, limit", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"path":   "/workspace/config.json",
				"offset": float64(10),
				"limit":  float64(50),
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolRead, state)
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

	t.Run("falls back to file_path key", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"file_path": "/workspace/fallback.json",
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolRead, state)
		if result.ReadFile().FilePath != "/workspace/fallback.json" {
			t.Errorf("expected FilePath '/workspace/fallback.json', got %q", result.ReadFile().FilePath)
		}
	})

	t.Run("includes output with content and line count", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"path": "/workspace/file.txt",
			},
			Output: "line1\nline2\nline3",
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolRead, state)
		if result.ReadFile().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if result.ReadFile().Output.Content != "line1\nline2\nline3" {
			t.Errorf("expected Content, got %q", result.ReadFile().Output.Content)
		}
		// Line count should be calculated from content
		if result.ReadFile().Output.LineCount != 3 {
			t.Errorf("expected LineCount 3, got %d", result.ReadFile().Output.LineCount)
		}
	})

	t.Run("extracts line_count and truncated from metadata", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"path": "/workspace/large.txt",
			},
			Output: "content here",
			Metadata: map[string]any{
				"line_count": float64(1000),
				"truncated":  true,
			},
		}
		result := normalizer.NormalizeToolCall(OpenCodeToolRead, state)
		if result.ReadFile().Output.LineCount != 1000 {
			t.Errorf("expected LineCount 1000, got %d", result.ReadFile().Output.LineCount)
		}
		if !result.ReadFile().Output.Truncated {
			t.Error("expected Truncated to be true")
		}
	})
}

// TestNormalizerGeneric tests the normalizer's generic handling.
func TestNormalizerGeneric(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("wraps unknown tools as generic", func(t *testing.T) {
		state := &ToolState{
			Input: map[string]any{
				"custom_field": "custom_value",
			},
		}
		result := normalizer.NormalizeToolCall("custom_tool", state)
		if result.Kind() != streams.ToolKindGeneric {
			t.Errorf("expected Kind %q, got %q", streams.ToolKindGeneric, result.Kind())
		}
		if result.Generic() == nil {
			t.Fatal("expected Generic to be set")
		}
		if result.Generic().Name != "custom_tool" {
			t.Errorf("expected Name 'custom_tool', got %q", result.Generic().Name)
		}
	})

	t.Run("includes output when available", func(t *testing.T) {
		state := &ToolState{
			Input:  map[string]any{},
			Output: "tool output",
		}
		result := normalizer.NormalizeToolCall("unknown", state)
		if result.Generic().Output != "tool output" {
			t.Errorf("expected Output 'tool output', got %v", result.Generic().Output)
		}
	})
}

// TestNormalizeToolResult tests the deprecated result handler.
func TestNormalizeToolResult(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("updates shell exec output from string", func(t *testing.T) {
		payload := streams.NewShellExec("ls", "", "", 0, false)
		normalizer.NormalizeToolResult(payload, "file1\nfile2")
		if payload.ShellExec().Output == nil {
			t.Fatal("expected Output to be set")
		}
		if payload.ShellExec().Output.Stdout != "file1\nfile2" {
			t.Errorf("expected Stdout, got %q", payload.ShellExec().Output.Stdout)
		}
	})

	t.Run("updates shell exec output from map", func(t *testing.T) {
		payload := streams.NewShellExec("cmd", "", "", 0, false)
		normalizer.NormalizeToolResult(payload, map[string]any{
			"output": "stdout content",
			"error":  "stderr content",
		})
		if payload.ShellExec().Output.Stdout != "stdout content" {
			t.Errorf("expected Stdout 'stdout content', got %q", payload.ShellExec().Output.Stdout)
		}
		if payload.ShellExec().Output.Stderr != "stderr content" {
			t.Errorf("expected Stderr 'stderr content', got %q", payload.ShellExec().Output.Stderr)
		}
	})

	t.Run("updates http request response", func(t *testing.T) {
		payload := streams.NewHttpRequest("https://example.com", "GET")
		normalizer.NormalizeToolResult(payload, "response body")
		if payload.HttpRequest().Response != "response body" {
			t.Errorf("expected Response 'response body', got %q", payload.HttpRequest().Response)
		}
	})

	t.Run("updates generic output", func(t *testing.T) {
		payload := streams.NewGeneric("tool", nil)
		normalizer.NormalizeToolResult(payload, map[string]any{"data": "value"})
		if payload.Generic().Output == nil {
			t.Fatal("expected Output to be set")
		}
	})

	t.Run("does not overwrite existing output", func(t *testing.T) {
		payload := streams.NewShellExec("ls", "", "", 0, false)
		payload.ShellExec().Output = &streams.ShellExecOutput{Stdout: "existing"}
		normalizer.NormalizeToolResult(payload, "new output")
		// Should not overwrite
		if payload.ShellExec().Output.Stdout != "existing" {
			t.Errorf("expected Stdout to remain 'existing', got %q", payload.ShellExec().Output.Stdout)
		}
	})
}

// TestNewNormalizer tests the constructor.
func TestNewNormalizer(t *testing.T) {
	normalizer := NewNormalizer()
	if normalizer == nil {
		t.Fatal("expected non-nil normalizer")
	}
}
