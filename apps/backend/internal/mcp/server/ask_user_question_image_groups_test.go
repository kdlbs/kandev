package mcp

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/clarification/protocol"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.6
func TestQuestionImageGroupsSchema(t *testing.T) {
	props := buildOperatorQuestionSchemaItem()[propsKey].(map[string]any)
	groups, ok := props["image_groups"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, protocol.MaxQuestionImageReferences, groups["maxItems"])
	description := groups[descriptionArg].(string)
	require.Contains(t, description, "pair")
	require.Contains(t, description, "single")
	require.Contains(t, description, "exactly 2")
	item := groups["items"].(map[string]any)
	require.Equal(t, false, item["additionalProperties"])
	kind := item[propsKey].(map[string]any)["kind"].(map[string]any)
	require.ElementsMatch(t, []string{"pair", "single"}, kind["enum"])
	require.NotContains(t, buildQuestionSchemaItem()[propsKey], "image_groups")
	require.Contains(t, questionImageSchemaDescription(), "image_groups")
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.2
func TestQuestionImageGroupsValidatedAtToolBoundary(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	schema, err := json.Marshal(s.mcpServer.ListTools()["ask_user_question_kandev"].Tool.InputSchema)
	require.NoError(t, err)
	require.Contains(t, string(schema), `"image_groups"`)
	result := callTool(t, s, "ask_user_question_kandev", map[string]any{
		"questions": []map[string]any{{
			"prompt":       "Approve?",
			"options":      []map[string]any{{"label": "Yes", "description": "Approve"}, {"label": "No", "description": "Reject"}},
			"images":       []map[string]any{{"id": "a", "path": "a.png", "alt": "A"}},
			"image_groups": []map[string]any{{"kind": "pair", "image_ids": []string{"a"}}},
		}},
	})
	require.True(t, result.IsError)
	require.Contains(t, resultText(t, result), "a pair must list exactly 2 different image IDs")
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.4
func TestQuestionImageGroupsRejectedOnParentQuestions(t *testing.T) {
	backend := &testBackend{response: map[string]any{}}
	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityParentQuestion}, nil)
	s := NewWithProfile(backend, "child-session", "child-task", 10005, newTestLogger(t), "", false, profile)
	result := callTool(t, s, "ask_parent_question_kandev", map[string]any{"questions": []map[string]any{{
		"prompt":       "Choose",
		"options":      []map[string]any{{"label": "A", "description": "First"}, {"label": "B", "description": "Second"}},
		"image_groups": []map[string]any{{"kind": "single", "image_ids": []string{"a"}}},
	}}})
	require.True(t, result.IsError)
	require.Contains(t, resultText(t, result), "parent questions do not support images")
	require.Nil(t, backend.lastPayload)
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.2
func TestQuestionImageGroupDecodeErrorsNameTheQuestion(t *testing.T) {
	valid := map[string]any{"id": "first", "prompt": "One?", "options": []map[string]any{{"label": "A"}, {"label": "B"}}}
	for name, groups := range map[string]any{
		"null entry":    []any{nil},
		"wrong id type": []any{map[string]any{"kind": "single", "image_ids": []any{7}}},
		"unknown field": []any{map[string]any{"kind": "single", "image_ids": []any{"a"}, "extra": true}},
	} {
		t.Run(name, func(t *testing.T) {
			second := map[string]any{
				"id": "second", "prompt": "Two?", "options": []map[string]any{{"label": "A"}, {"label": "B"}},
				"images":       []map[string]any{{"id": "a", "path": "a.png", "alt": "A"}},
				"image_groups": groups,
			}
			err := validateQuestionImagePayloads([]map[string]any{valid, second})
			require.Error(t, err)
			require.Contains(t, err.Error(), `question 2 ("second")`)
		})
	}
}
