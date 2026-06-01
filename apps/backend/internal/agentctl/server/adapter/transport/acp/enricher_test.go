package acp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Golden-style cases distilled from acpdbg captures (claude/codex/opencode/cursor).
func TestAgentEnrichment_GoldenFixtures(t *testing.T) {
	t.Run("claude read update with file_path in rawInput", func(t *testing.T) {
		n := NewNormalizer("claude-acp")
		payload := n.NormalizeToolCall("read", map[string]any{
			"kind":  "read",
			"title": "Read File",
		})
		n.EnrichFromToolCallUpdate(payload, strPtr("Read File"), nil, map[string]any{
			"file_path": "/workspace/README.md",
		}, nil)
		if got := payload.ReadFile().FilePath; got != "/workspace/README.md" {
			t.Fatalf("FilePath = %q, want /workspace/README.md", got)
		}
	})

	t.Run("claude read update with toolResponse filePath in meta", func(t *testing.T) {
		n := NewNormalizer("claude-acp")
		payload := n.NormalizeToolCall("read", map[string]any{"kind": "read"})
		n.EnrichFromToolCallUpdate(payload, nil, map[string]any{
			"claudeCode": map[string]any{
				"toolResponse": map[string]any{
					"file": map[string]any{"filePath": "/workspace/src/index.ts"},
				},
			},
		}, nil, nil)
		if got := payload.ReadFile().FilePath; got != "/workspace/src/index.ts" {
			t.Fatalf("FilePath = %q, want /workspace/src/index.ts", got)
		}
	})

	t.Run("opencode read update with filePath", func(t *testing.T) {
		n := NewNormalizer("opencode-acp")
		payload := n.NormalizeToolCall("read", map[string]any{"kind": "read", "title": "read"})
		n.EnrichFromToolCallUpdate(payload, strPtr("read"), nil, map[string]any{
			"filePath": "/workspace/README.md",
		}, nil)
		if got := payload.ReadFile().FilePath; got != "/workspace/README.md" {
			t.Fatalf("FilePath = %q, want /workspace/README.md", got)
		}
	})

	t.Run("codex read from locations on initial frame", func(t *testing.T) {
		n := NewNormalizer("codex-acp")
		payload := n.NormalizeToolCall("read", map[string]any{
			"kind":  "read",
			"title": "Read README.md",
			"locations": []any{
				map[string]any{"path": "/workspace/README.md"},
			},
		})
		if got := payload.ReadFile().FilePath; got != "/workspace/README.md" {
			t.Fatalf("FilePath = %q, want /workspace/README.md", got)
		}
	})

	t.Run("codex read from title when structured fields absent", func(t *testing.T) {
		n := NewNormalizer("codex-acp")
		payload := n.NormalizeToolCall("read", map[string]any{
			"kind":  "read",
			"title": "Read apps/web/lib/utils.ts",
		})
		if got := payload.ReadFile().FilePath; got != "apps/web/lib/utils.ts" {
			t.Fatalf("FilePath = %q, want apps/web/lib/utils.ts", got)
		}
	})

	t.Run("codex search from title", func(t *testing.T) {
		n := NewNormalizer("codex-acp")
		payload := n.NormalizeToolCall("search", map[string]any{
			"kind":  "search",
			"title": "Search tool_call in apps/backend",
		})
		cs := payload.CodeSearch()
		if cs.Query != "tool_call" || cs.Path != "apps/backend" {
			t.Fatalf("CodeSearch = (%q, %q), want (tool_call, apps/backend)", cs.Query, cs.Path)
		}
	})

	t.Run("codex search fills path from title when query in rawInput", func(t *testing.T) {
		n := NewNormalizer("codex-acp")
		payload := n.NormalizeToolCall("search", map[string]any{
			"kind":  "search",
			"title": "Search tool_call in apps/backend",
			"raw_input": map[string]any{
				"query": "tool_call",
			},
		})
		cs := payload.CodeSearch()
		if cs.Query != "tool_call" || cs.Path != "apps/backend" {
			t.Fatalf("CodeSearch = (%q, %q), want (tool_call, apps/backend)", cs.Query, cs.Path)
		}
	})

	t.Run("codex edit from changes map", func(t *testing.T) {
		n := NewNormalizer("codex-acp")
		payload := n.NormalizeToolCall("edit", map[string]any{"kind": "edit"})
		n.EnrichFromToolCallUpdate(payload, nil, nil, map[string]any{
			"changes": map[string]any{
				"src/main.go": map[string]any{
					"unified_diff": "--- a/src/main.go\n+++ b/src/main.go\n",
				},
			},
		}, nil)
		mf := payload.ModifyFile()
		if mf.FilePath != "src/main.go" {
			t.Fatalf("FilePath = %q, want src/main.go", mf.FilePath)
		}
		if mf.Mutations[0].Diff == "" {
			t.Fatal("expected unified diff from codex changes map")
		}
	})

	t.Run("cursor read does not infer path from generic title", func(t *testing.T) {
		n := NewNormalizer("cursor-acp")
		payload := n.NormalizeToolCall("read", map[string]any{
			"kind":  "read",
			"title": "Read File",
		})
		n.EnrichFromToolCallUpdate(payload, strPtr("Read File"), nil, nil, nil)
		if got := payload.ReadFile().FilePath; got != "" {
			t.Fatalf("FilePath = %q, want empty", got)
		}
	})

	t.Run("common layer without agent does not parse codex titles", func(t *testing.T) {
		n := NewNormalizer("")
		payload := n.NormalizeToolCall("search", map[string]any{
			"kind":  "search",
			"title": "Search tool_call in apps/backend",
		})
		cs := payload.CodeSearch()
		if cs.Query != "" || cs.Path != "" {
			t.Fatalf("CodeSearch = (%q, %q), want empty fields without enricher", cs.Query, cs.Path)
		}
	})
}

func TestCodexParsedCmdSearch(t *testing.T) {
	query, path := codexParsedCmdSearch(map[string]any{
		"parsed_cmd": []any{
			map[string]any{"type": "grep", "query": "first", "path": "a/"},
			map[string]any{"type": "grep", "query": "second", "path": "b/"},
		},
	})
	if query != "first" || path != "a/" {
		t.Fatalf("codexParsedCmdSearch() = (%q, %q), want (first, a/)", query, path)
	}
}

func TestCodexCanonicalChange(t *testing.T) {
	path, diff := codexCanonicalChange(map[string]any{
		"z.go": map[string]any{"unified_diff": "diff-z"},
		"a.go": map[string]any{"unified_diff": "diff-a"},
	})
	if path != "a.go" || diff != "diff-a" {
		t.Fatalf("codexCanonicalChange() = (%q, %q), want (a.go, diff-a)", path, diff)
	}
}

func TestCodexTitleHints(t *testing.T) {
	tests := []struct {
		name        string
		readTitle   string
		wantRead    string
		searchTitle string
		wantQuery   string
		wantPath    string
	}{
		{
			name:      "read rejects generic label",
			readTitle: "Read File",
			wantRead:  "",
		},
		{
			name:      "read accepts path",
			readTitle: "Read README.md",
			wantRead:  "README.md",
		},
		{
			name:        "search query only",
			searchTitle: "Search tool_call",
			wantQuery:   "tool_call",
		},
		{
			name:        "search query and path",
			searchTitle: "Search tool_call in apps/backend",
			wantQuery:   "tool_call",
			wantPath:    "apps/backend",
		},
		{
			name:        "list directory",
			searchTitle: "List /workspace",
			wantPath:    "/workspace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.readTitle != "" {
				if got := codexReadTitleHint(tt.readTitle); got != tt.wantRead {
					t.Errorf("codexReadTitleHint(%q) = %q, want %q", tt.readTitle, got, tt.wantRead)
				}
			}
			if tt.searchTitle != "" {
				query, path := codexSearchTitleHints(tt.searchTitle)
				if query != tt.wantQuery || path != tt.wantPath {
					t.Errorf("codexSearchTitleHints(%q) = (%q, %q), want (%q, %q)",
						tt.searchTitle, query, path, tt.wantQuery, tt.wantPath)
				}
			}
		})
	}
}

func TestNormalizeToolCall_CodexShapes(t *testing.T) {
	normalizer := NewNormalizer("codex-acp")

	t.Run("read from file_path raw_input", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":      "read",
			"title":     "Read",
			"raw_input": map[string]any{"file_path": "/tmp/test.go"},
		})
		if got := payload.ReadFile().FilePath; got != "/tmp/test.go" {
			t.Fatalf("FilePath = %q, want /tmp/test.go", got)
		}
	})

	t.Run("read from locations", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":  "read",
			"title": "Read README.md",
			"locations": []any{
				map[string]any{"path": "/workspace/README.md"},
			},
		})
		if got := payload.ReadFile().FilePath; got != "/workspace/README.md" {
			t.Fatalf("FilePath = %q, want /workspace/README.md", got)
		}
	})

	t.Run("read from codex title when path fields absent", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("read", map[string]any{
			"kind":  "read",
			"title": "Read apps/web/lib/utils.ts",
		})
		if got := payload.ReadFile().FilePath; got != "apps/web/lib/utils.ts" {
			t.Fatalf("FilePath = %q, want apps/web/lib/utils.ts", got)
		}
	})

	t.Run("search from codex title", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("search", map[string]any{
			"kind":  "search",
			"title": "Search tool_call in apps/backend",
		})
		cs := payload.CodeSearch()
		if cs.Query != "tool_call" || cs.Path != "apps/backend" {
			t.Fatalf("CodeSearch = (%q, %q), want (tool_call, apps/backend)", cs.Query, cs.Path)
		}
	})

	t.Run("search fills missing path from title when query present in raw_input", func(t *testing.T) {
		payload := normalizer.NormalizeToolCall("search", map[string]any{
			"kind":  "search",
			"title": "Search tool_call in apps/backend",
			"raw_input": map[string]any{
				"query": "tool_call",
			},
		})
		cs := payload.CodeSearch()
		if cs.Query != "tool_call" || cs.Path != "apps/backend" {
			t.Fatalf("CodeSearch = (%q, %q), want (tool_call, apps/backend)", cs.Query, cs.Path)
		}
	})
}

func TestNormalizeToolCall_CursorGenericTitle(t *testing.T) {
	normalizer := NewNormalizer("cursor-acp")
	payload := normalizer.NormalizeToolCall("read", map[string]any{
		"kind":  "read",
		"title": "Read File",
	})
	if got := payload.ReadFile().FilePath; got != "" {
		t.Fatalf("FilePath = %q, want empty", got)
	}
	if payload.Kind() != streams.ToolKindReadFile {
		t.Fatalf("Kind = %q, want read_file", payload.Kind())
	}
}

func strPtr(s string) *string {
	return &s
}
