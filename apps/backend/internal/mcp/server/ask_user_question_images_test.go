package mcp

import (
	"encoding/json"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQuestionImagesSchemaAndValidation(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	schema, err := json.Marshal(s.mcpServer.ListTools()["ask_user_question_kandev"].Tool.InputSchema)
	require.NoError(t, err)
	require.Contains(t, string(schema), `"images"`)
	require.NotContains(t, buildQuestionSchemaItem()[propsKey], "images")
	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{{"prompt": "Approve?", "options": []map[string]interface{}{{"label": "Yes", "description": "Approve"}, {"label": "No", "description": "Reject"}}, "images": []map[string]interface{}{{"id": "a", "path": "../a.png", "alt": "A"}}}},
	})
	require.True(t, result.IsError)
}

func TestChoiceImagesSchemaAndValidation(t *testing.T) {
	optionProps := func(schema map[string]any) map[string]any {
		return schema[propsKey].(map[string]any)[optionsArg].(map[string]any)["items"].(map[string]any)[propsKey].(map[string]any)
	}
	require.Contains(t, optionProps(buildOperatorQuestionSchemaItem()), "images")
	require.NotContains(t, optionProps(buildQuestionSchemaItem()), "images")
	require.Error(t, validateQuestionImagePayloads([]map[string]any{{"options": []map[string]any{{"images": []map[string]any{{"id": "a", "path": "../a.png", "alt": "A"}}}}}}))
}

func TestChoiceImagesParentFieldsRejected(t *testing.T) {
	for _, images := range []any{[]any{}, []any{map[string]any{"id": "a", "path": "a.png", "alt": "A"}}} {
		backend := &testBackend{response: map[string]interface{}{}}
		profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityParentQuestion}, nil)
		s := NewWithProfile(backend, "child-session", "child-task", 10005, newTestLogger(t), "", false, profile)
		result := callTool(t, s, "ask_parent_question_kandev", map[string]any{"questions": []map[string]any{{"prompt": "Choose", "options": []map[string]any{{"label": "A", "description": "First", "images": images}, {"label": "B", "description": "Second"}}}}})
		require.True(t, result.IsError)
		require.Nil(t, backend.lastPayload)
	}
}
