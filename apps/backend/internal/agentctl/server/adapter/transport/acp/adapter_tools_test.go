package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
)

func TestToolCallUpdateSupplemental(t *testing.T) {
	tcu := &acp.SessionToolCallUpdate{
		Locations: []acp.ToolCallLocation{
			{Path: "/workspace/src/index.ts"},
		},
	}

	supplemental := toolCallUpdateSupplemental(tcu)
	if supplemental == nil {
		t.Fatal("expected supplemental map")
	}
	if got, _ := supplemental["path"].(string); got != "/workspace/src/index.ts" {
		t.Fatalf("path = %q, want /workspace/src/index.ts", got)
	}

	n := NewNormalizer("")
	payload := n.NormalizeToolCall("read", map[string]any{
		"kind":      "read",
		"raw_input": map[string]any{},
	})
	n.UpdatePayloadInput(payload, nil, supplemental)
	if got := payload.ReadFile().FilePath; got != "/workspace/src/index.ts" {
		t.Fatalf("ReadFile.FilePath = %q, want /workspace/src/index.ts", got)
	}

	searchPayload := n.NormalizeToolCall("search", map[string]any{
		"kind":      "search",
		"raw_input": map[string]any{},
	})
	n.UpdatePayloadInput(searchPayload, nil, supplemental)
	if got := searchPayload.CodeSearch().Path; got != "/workspace/src/index.ts" {
		t.Fatalf("CodeSearch.Path = %q, want /workspace/src/index.ts", got)
	}
}

func TestPathFromLocationSlice(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name: "[]any shape from initial tool_call",
			input: []any{
				map[string]any{"path": "/a.go"},
			},
			want: "/a.go",
		},
		{
			name: "[]map[string]any shape from tool_call_update supplemental",
			input: []map[string]any{
				{"path": "/b.go"},
			},
			want: "/b.go",
		},
		{name: "empty slice", input: []any{}, want: ""},
		{name: "nil", input: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathFromLocationSlice(tt.input); got != tt.want {
				t.Fatalf("pathFromLocationSlice() = %q, want %q", got, tt.want)
			}
		})
	}
}
