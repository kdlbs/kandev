package streams

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
	"github.com/stretchr/testify/require"
)

const allToolTestLimit = 256 << 10

func textContents(text string) []ToolCallContentItem {
	return []ToolCallContentItem{{Type: "content", Content: &ContentBlock{Type: "text", Text: text}}}
}

func duplicateContents(text string) []ToolCallContentItem {
	return []ToolCallContentItem{
		{Type: "content", Content: &ContentBlock{Type: "text", Text: text}},
		{Type: "content", Content: &ContentBlock{Type: "text", Text: text}},
	}
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.1
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.2
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.3
func TestAllToolDeliveryCategoriesAndDuplicateCarriers(t *testing.T) {
	large := strings.Repeat("output世", 350000)
	for _, status := range []string{"complete", "error", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			for name, build := range allToolCategoryFixtures {
				t.Run(name, func(t *testing.T) {
					event := build(large)
					event.Type = EventTypeToolUpdate
					event.ToolCallID = "tool-" + name
					event.ParentToolCallID = "parent"
					event.ToolStatus = status
					kind := ToolKind("")
					if event.NormalizedPayload != nil {
						kind = event.NormalizedPayload.Kind()
					}
					sourceContents := len(event.ToolCallContents[0].Content.Text)
					prepared := PrepareToolEventWithLimit(event, allToolTestLimit)
					encoded, err := json.Marshal(prepared)
					require.NoError(t, err)
					require.LessOrEqual(t, len(encoded), allToolTestLimit+int(2*deliverylimits.ShellTailBytes))
					require.Equal(t, status, prepared.ToolStatus)
					require.Equal(t, "tool-"+name, prepared.ToolCallID)
					require.Equal(t, "parent", prepared.ParentToolCallID)
					if event.NormalizedPayload != nil {
						require.Equal(t, kind, prepared.NormalizedPayload.Kind())
					}
					require.True(t, allToolHasTruncationMarker(prepared), "%s retained no truncation marker", name)
					require.Equal(t, sourceContents, len(event.ToolCallContents[0].Content.Text), "%s mutated the source carrier", name)
					require.Equal(t, prepared, PrepareToolEventWithLimit(prepared, allToolTestLimit), "%s preparation is not idempotent", name)
				})
			}
		})
	}
}

var allToolCategoryFixtures = map[string]func(string) AgentEvent{
	"search": func(large string) AgentEvent {
		payload := NewCodeSearch("needle", "pattern", "/repo", "*.go")
		payload.CodeSearch().Output = &CodeSearchOutput{Files: []string{"a.go", "b.go"}, FileCount: 2}
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"read": func(large string) AgentEvent {
		payload := NewReadFile("main.go", 10, 20)
		payload.ReadFile().Output = &ReadFileOutput{Content: large, LineCount: 40, Language: "go"}
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"edit": func(large string) AgentEvent {
		payload := NewModifyFile("main.go", []FileMutation{{Type: MutationReplace, Content: large, Diff: large}})
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"shell": func(large string) AgentEvent {
		payload := NewShellExec("rg needle", "/repo", "search", 0, false)
		payload.ShellExec().Output = &ShellExecOutput{Stdout: strings.Repeat("x", int(deliverylimits.ShellTailBytes)), Truncated: true}
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"http": func(large string) AgentEvent {
		payload := &NormalizedPayload{kind: ToolKindHttpRequest, httpRequest: &HttpRequestPayload{URL: "https://example.test/api", Method: "GET", Response: large}}
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"subagent": func(large string) AgentEvent {
		payload := NewSubagentTask("task", large, "explore")
		payload.SubagentTask().ResultText = large
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"plan": func(large string) AgentEvent {
		payload := &NormalizedPayload{kind: ToolKindShowPlan, showPlan: &ShowPlanPayload{Summary: large, Steps: []string{large, large}}}
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"todos": func(large string) AgentEvent {
		payload := &NormalizedPayload{kind: ToolKindManageTodos, manageTodos: &ManageTodosPayload{
			Operation: "add",
			Items:     []TodoItem{{ID: "1", Description: large, Status: "pending", ActiveForm: large}},
		}}
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"generic": func(large string) AgentEvent {
		payload := NewGeneric("unknown-tool", map[string]any{"query": large})
		payload.Generic().Output = map[string]any{"content": []any{map[string]any{"type": "text", "text": large}}}
		return AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(large)}
	},
	"unknown": func(large string) AgentEvent {
		return AgentEvent{ToolCallID: "unknown", ToolCallContents: duplicateContents(large)}
	},
}

func allToolHasTruncationMarker(event AgentEvent) bool {
	for _, item := range event.ToolCallContents {
		if item.Content == nil {
			continue
		}
		if strings.Contains(item.Content.Text, "truncated by Kandev") ||
			strings.Contains(item.Content.Text, "omitted oversized") {
			return true
		}
	}
	if event.NormalizedPayload == nil {
		return false
	}
	switch event.NormalizedPayload.Kind() {
	case ToolKindReadFile:
		return event.NormalizedPayload.ReadFile().Output.Truncated
	case ToolKindModifyFile:
		return len(event.NormalizedPayload.ModifyFile().Mutations) == 0
	case ToolKindCodeSearch:
		return event.NormalizedPayload.CodeSearch().Output.Truncated
	case ToolKindGeneric:
		output, ok := event.NormalizedPayload.Generic().Output.(map[string]any)
		return ok && output["truncated"] == true
	case ToolKindShellExec:
		return true
	default:
		return true
	}
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-003.1
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-003.3
func TestAllToolDeliveryExistingRepresentation(t *testing.T) {
	t.Run("read_uses_typed_truncation", func(t *testing.T) {
		payload := NewReadFile("main.go", 0, 10)
		payload.ReadFile().Output = &ReadFileOutput{Content: strings.Repeat("世", 2<<20), LineCount: 10}
		prepared := PrepareToolEventWithLimit(AgentEvent{Type: EventTypeToolUpdate, NormalizedPayload: payload}, allToolTestLimit)
		output := prepared.NormalizedPayload.ReadFile().Output
		require.True(t, output.Truncated)
		require.NotEmpty(t, output.Content)
		require.Equal(t, 10, output.LineCount)
		require.Less(t, len(output.Content), 2<<20)
	})

	t.Run("edit_omits_oversized_bodies", func(t *testing.T) {
		large := strings.Repeat("patch\n", 400000)
		payload := NewModifyFile("main.go", []FileMutation{{Type: MutationPatch, OldContent: large, NewContent: large, Diff: large}})
		prepared := PrepareToolEventWithLimit(AgentEvent{Type: EventTypeToolUpdate, NormalizedPayload: payload}, allToolTestLimit)
		require.Empty(t, prepared.NormalizedPayload.ModifyFile().Mutations, "a cut body must not appear as a partial patch")
		require.Equal(t, "main.go", prepared.NormalizedPayload.ModifyFile().FilePath)
		require.True(t, allToolHasTruncationMarker(prepared))
	})

	t.Run("search_keeps_supplied_total", func(t *testing.T) {
		files := make([]string, 0, 5000)
		for index := range 5000 {
			files = append(files, strings.Repeat("path", 40)+string(rune('a'+index%26)))
		}
		payload := NewCodeSearch("needle", "", "/repo", "")
		payload.CodeSearch().Output = &CodeSearchOutput{Files: files, FileCount: len(files)}
		prepared := PrepareToolEventWithLimit(AgentEvent{Type: EventTypeToolUpdate, NormalizedPayload: payload}, allToolTestLimit)
		output := prepared.NormalizedPayload.CodeSearch().Output
		require.True(t, output.Truncated)
		require.Equal(t, len(files), output.FileCount)
		require.Less(t, len(output.Files), len(files))
	})

	t.Run("binary_block_is_not_cut", func(t *testing.T) {
		data := strings.Repeat("A", 2<<20)
		event := AgentEvent{Type: EventTypeToolUpdate, ToolCallContents: []ToolCallContentItem{
			{Type: "content", Content: &ContentBlock{Type: "image", Data: data, MimeType: "image/png"}},
		}}
		prepared := PrepareToolEventWithLimit(event, allToolTestLimit)
		for _, item := range prepared.ToolCallContents {
			require.False(t, item.Content.Type == "image" && len(item.Content.Data) != len(data), "a cut image must not be presented as valid content")
		}
		require.True(t, allToolHasTruncationMarker(prepared))
	})

	t.Run("binary_block_kept_whole_when_fitting", func(t *testing.T) {
		data := strings.Repeat("A", 1024)
		event := AgentEvent{Type: EventTypeToolUpdate, ToolCallContents: []ToolCallContentItem{
			{Type: "content", Content: &ContentBlock{Type: "image", Data: data, MimeType: "image/png"}},
		}}
		prepared := PrepareToolEventWithLimit(event, allToolTestLimit)
		require.Len(t, prepared.ToolCallContents, 1)
		require.Equal(t, data, prepared.ToolCallContents[0].Content.Data)
	})

	t.Run("shell_tails_unchanged", func(t *testing.T) {
		stdout := strings.Repeat("x", int(deliverylimits.ShellTailBytes))
		payload := NewShellExec("cmd", "/repo", "run", 0, false)
		payload.ShellExec().Output = &ShellExecOutput{Stdout: stdout, Truncated: true}
		prepared := PrepareToolEventWithLimit(AgentEvent{NormalizedPayload: payload, ToolCallContents: duplicateContents(strings.Repeat("y", allToolTestLimit))}, allToolTestLimit)
		require.Equal(t, stdout, prepared.NormalizedPayload.ShellExec().Output.Stdout)
		require.True(t, prepared.NormalizedPayload.ShellExec().Output.Truncated)
	})
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.4
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.5
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-002.2
func TestAllToolDeliveryBoundedWorkAndSnapshots(t *testing.T) {
	deep := any("leaf")
	for range 40 {
		deep = []any{deep}
	}
	event := AgentEvent{
		Type: EventTypeToolUpdate, ToolCallID: "deep", ToolStatus: "complete",
		NormalizedPayload: NewGeneric("deep-tool", map[string]any{
			"nul":     strings.Repeat("\x00", 2<<20),
			"unicode": strings.Repeat("世", 2<<20),
			"deep":    deep,
			"wide":    make([]any, 200000),
		}),
		ToolCallContents: append(duplicateContents(strings.Repeat("duplicate", 400000)), textContents(strings.Repeat("\x00", 1<<20))...),
	}
	source := event.ToolCallContents
	started := time.Now()
	prepared := PrepareToolEventWithLimit(event, allToolTestLimit)
	require.Less(t, time.Since(started), 5*time.Second, "preparation must bound traversal for huge inputs")
	encoded, err := json.Marshal(prepared)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), allToolTestLimit)
	require.Len(t, source, len(event.ToolCallContents))
	require.Equal(t, strings.Repeat("duplicate", 400000), source[0].Content.Text, "preparation mutated the source carrier")
	require.Equal(t, prepared, PrepareToolEventWithLimit(prepared, allToolTestLimit), "repeat preparation changed the accepted bytes")

	snapshot := prepared.NormalizedPayload.Snapshot()
	require.NotNil(t, snapshot)
	prepared.NormalizedPayload.Generic().Input = map[string]any{"changed": true}
	require.NotEqual(t, prepared.NormalizedPayload.Generic().Input, snapshot.Generic().Input)
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.4
func TestAllToolDeliveryBoundedSerializationCost(t *testing.T) {
	huge := strings.Repeat("x", 8<<20)
	wide := make([]ToolCallContentItem, 0, 100_000)
	for range 100_000 {
		wide = append(wide, ToolCallContentItem{Type: "content", Content: &ContentBlock{Type: "text", Text: "small"}})
	}
	cases := map[string]AgentEvent{
		"huge_diff": {
			Type: EventTypeToolUpdate, ToolStatus: "complete",
			NormalizedPayload: NewModifyFile("main.go", []FileMutation{{Type: MutationPatch, Diff: huge, NewContent: huge}}),
		},
		"huge_binary": {
			Type: EventTypeToolUpdate, ToolStatus: "complete",
			ToolCallContents: []ToolCallContentItem{{Type: "content", Content: &ContentBlock{
				Type: "image", Data: huge, MimeType: "image/png",
			}}},
		},
		"huge_todo": {
			Type: EventTypeToolUpdate, ToolStatus: "complete",
			NormalizedPayload: &NormalizedPayload{kind: ToolKindManageTodos, manageTodos: &ManageTodosPayload{
				Operation: "add", Items: []TodoItem{{ID: "1", Description: huge, ActiveForm: huge}},
			}},
		},
		"wide_contents": {
			Type: EventTypeToolUpdate, ToolStatus: "complete",
			ToolCallContents: append(duplicateContents("first"), wide...),
		},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			started := time.Now()
			allocations := testing.AllocsPerRun(3, func() {
				prepared := PrepareToolEventWithLimit(event, allToolTestLimit)
				encoded, err := json.Marshal(prepared)
				require.NoError(t, err)
				require.LessOrEqual(t, len(encoded), allToolTestLimit+int(2*deliverylimits.ShellTailBytes))
			})
			require.Less(t, time.Since(started), 5*time.Second)
			require.Less(t, allocations, float64(2000), "%s preparation allocated an unbounded buffer", name)
		})
	}
	widePrepared := PrepareToolEventWithLimit(cases["wide_contents"], allToolTestLimit)
	require.LessOrEqual(t, cap(widePrepared.ToolCallContents), toolDisplayItemNodes+1)
}
