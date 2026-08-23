package mcp

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreToolDescriptionsStayFocused(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	limits := map[string]int{
		"create_task_kandev":                   700,
		"message_task_kandev":                  650,
		"stop_task_kandev":                     700,
		"spawn_session_kandev":                 600,
		"add_branch_to_task_kandev":            650,
		"add_workspace_sources_kandev":         500,
		"step_complete_kandev":                 650,
		"ask_user_question_kandev":             600,
		"show_rich_output_kandev":              650,
		"show_walkthrough_kandev":              650,
		"publish_review_findings_kandev":       650,
		"update_repository_base_branch_kandev": 500,
		"add_task_dependency_kandev":           500,
		"list_related_tasks_kandev":            500,
	}
	for name, limit := range limits {
		t.Run(name, func(t *testing.T) {
			tool, ok := s.mcpServer.ListTools()[name]
			require.True(t, ok)
			assert.LessOrEqual(t, utf8.RuneCountInString(tool.Tool.Description), limit)
		})
	}
}

func TestTaskTitleDescriptionStaysFocused(t *testing.T) {
	s := New(&testBackend{}, "test-session", "task-current", 10005, newTestLogger(t), "", false, ModeTaskTitlePending)
	tool, ok := s.mcpServer.ListTools()["set_task_title_kandev"]
	require.True(t, ok)
	assert.LessOrEqual(t, utf8.RuneCountInString(tool.Tool.Description), 500)
}

func TestDelegationPolicyLivesInSharedContext(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	for _, name := range []string{"create_task_kandev", "message_task_kandev", "spawn_session_kandev"} {
		tool, ok := s.mcpServer.ListTools()[name]
		require.True(t, ok)
		assert.NotContains(t, tool.Tool.Description, "DELEGATION POLICY")
	}
}

func TestCoreToolRiskAnnotations(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	tests := []struct {
		name        string
		readOnly    bool
		destructive bool
		idempotent  bool
		openWorld   bool
	}{
		{name: "list_task_sessions_kandev", readOnly: true, destructive: false, idempotent: true, openWorld: false},
		{name: "archive_task_kandev", readOnly: false, destructive: false, idempotent: true, openWorld: false},
		{name: "delete_task_kandev", readOnly: false, destructive: true, idempotent: false, openWorld: false},
		{name: "stop_task_kandev", readOnly: false, destructive: true, idempotent: true, openWorld: false},
		{name: "add_workspace_sources_kandev", readOnly: false, destructive: false, idempotent: true, openWorld: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, ok := s.mcpServer.ListTools()[tt.name]
			require.True(t, ok)
			annotations := tool.Tool.Annotations
			require.NotNil(t, annotations.ReadOnlyHint)
			require.NotNil(t, annotations.DestructiveHint)
			require.NotNil(t, annotations.IdempotentHint)
			require.NotNil(t, annotations.OpenWorldHint)
			assert.Equal(t, tt.readOnly, *annotations.ReadOnlyHint)
			assert.Equal(t, tt.destructive, *annotations.DestructiveHint)
			assert.Equal(t, tt.idempotent, *annotations.IdempotentHint)
			assert.Equal(t, tt.openWorld, *annotations.OpenWorldHint)
		})
	}
}

func TestConfigTaskRiskAnnotationsMatchTaskMode(t *testing.T) {
	s := New(&testBackend{}, "test-session", "", 10005, newTestLogger(t), "", false, ModeConfig)

	archive := s.mcpServer.ListTools()["archive_task_kandev"].Tool.Annotations
	require.NotNil(t, archive.DestructiveHint)
	require.NotNil(t, archive.IdempotentHint)
	require.NotNil(t, archive.OpenWorldHint)
	assert.False(t, *archive.DestructiveHint)
	assert.True(t, *archive.IdempotentHint)
	assert.False(t, *archive.OpenWorldHint)

	deleteTool := s.mcpServer.ListTools()["delete_task_kandev"].Tool.Annotations
	require.NotNil(t, deleteTool.DestructiveHint)
	require.NotNil(t, deleteTool.OpenWorldHint)
	assert.True(t, *deleteTool.DestructiveHint)
	assert.False(t, *deleteTool.OpenWorldHint)
}

func TestAskUserQuestionPublishesAndReturnsStructuredOutput(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"answers": []interface{}{map[string]interface{}{
			"question_id":      "q1",
			"selected_options": []interface{}{"q1_opt1"},
		}},
	}}
	s := newTaskModeServer(t, backend, "task-current")

	tool, ok := s.mcpServer.ListTools()["ask_user_question_kandev"]
	require.True(t, ok)
	require.NotEmpty(t, tool.Tool.RawOutputSchema)
	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(tool.Tool.RawOutputSchema, &schema))
	assert.Equal(t, "object", schema["type"])
	assert.Contains(t, schema["properties"], "rejected")
	assert.Contains(t, schema["properties"], "reject_reason")
	assert.NotNil(t, schema["additionalProperties"])

	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{{
			"id":     "q1",
			"prompt": "Which database?",
			"options": []map[string]interface{}{
				{"label": "Postgres", "description": "Relational"},
				{"label": "SQLite", "description": "Embedded"},
			},
		}},
	})
	require.False(t, result.IsError)
	structured, ok := result.StructuredContent.(map[string]interface{})
	require.True(t, ok)
	answer, ok := structured["q1"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "q1_opt1", answer["selected_option"])
	content, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	assert.JSONEq(t, `{"q1":{"selected_option":"q1_opt1"}}`, content.Text)
}
