package mcp

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func TestAsyncQuestionsSchemaModes(t *testing.T) {
	task := newTaskModeServer(t, &testBackend{}, "task-current")
	taskProps := toolInputProperties(t, task, "ask_user_question_kandev")
	assert.Contains(t, taskProps, asyncArg)
	assert.Contains(t, taskProps, requestIDArg)

	config := newTestServer(t, &testBackend{})
	configProps := toolInputProperties(t, config, "ask_user_question_kandev")
	assert.NotContains(t, configProps, asyncArg)
	assert.NotContains(t, configProps, requestIDArg)
}

func TestAsyncQuestionToolDescriptionsByCapability(t *testing.T) {
	for _, test := range []struct {
		name       string
		capability mcpprofile.Capability
		toolName   string
	}{
		{name: "operator", capability: mcpprofile.CapabilityUserQuestion, toolName: "ask_user_question_kandev"},
		{name: "parent", capability: mcpprofile.CapabilityParentQuestion, toolName: "ask_parent_question_kandev"},
	} {
		for _, asyncSupported := range []bool{false, true} {
			t.Run(test.name+map[bool]string{false: "-blocking-only", true: "-async-capable"}[asyncSupported], func(t *testing.T) {
				capabilities := []mcpprofile.Capability{test.capability}
				if asyncSupported {
					capabilities = append(capabilities, mcpprofile.CapabilityAsyncQuestions)
				}
				profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, capabilities, nil)
				server := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, profile)
				tool, ok := server.mcpServer.ListTools()[test.toolName]
				require.True(t, ok)
				if asyncSupported {
					assert.Contains(t, tool.Tool.Description, "async=true")
					assert.Contains(t, tool.Tool.Description, "list_question_answers_kandev")
					assert.Contains(t, tool.Tool.Description, "wait_for_question_answers_kandev")
					assert.Contains(t, toolInputProperties(t, server, test.toolName), asyncArg)
					assert.Contains(t, getRegisteredToolNames(server), "list_question_answers_kandev")
				} else {
					assert.NotContains(t, tool.Tool.Description, "async=true")
					assert.NotContains(t, tool.Tool.Description, "list_question_answers_kandev")
					assert.NotContains(t, tool.Tool.Description, "wait_for_question_answers_kandev")
					assert.NotContains(t, toolInputProperties(t, server, test.toolName), asyncArg)
					assert.NotContains(t, getRegisteredToolNames(server), "list_question_answers_kandev")
				}
			})
		}
	}
}

func TestAsyncQuestionsBlockingItemIDsSurviveReceiptFields(t *testing.T) {
	questions := []map[string]interface{}{
		{"id": "status", "options": []map[string]interface{}{{"label": "A", "option_id": "a"}}},
		{"id": "mode", "options": []map[string]interface{}{{"label": "A", "option_id": "a"}}},
		{"id": "question_id", "options": []map[string]interface{}{{"label": "A", "option_id": "a"}}},
		{"id": "created_at", "options": []map[string]interface{}{{"label": "A", "option_id": "a"}}},
	}
	answers := make([]interface{}, 0, len(questions))
	for _, question := range questions {
		answers = append(answers, map[string]interface{}{
			"question_id":      question["id"],
			"selected_options": []interface{}{"a"},
			"custom_text":      "answer",
		})
	}
	result := extractQuestionAnswers(map[string]interface{}{"answers": answers}, questions)
	require.False(t, result.IsError)
	structured, ok := result.StructuredContent.(map[string]interface{})
	require.True(t, ok)
	for _, id := range []string{"status", "mode", "question_id", "created_at"} {
		answer, ok := structured[id].(map[string]interface{})
		require.True(t, ok, "blocking item id %q must map to an answer object, got %T", id, structured[id])
		assert.Equal(t, "a", answer["selected_option"])
	}
}

func TestAsyncQuestionsReadModeRejectsNonBoolean(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{asyncArg: "yes"}
	_, _, result := readAsyncMode(req)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	req.Params.Arguments = map[string]interface{}{requestIDArg: "id-without-async"}
	_, _, result = readAsyncMode(req)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestAsyncQuestionsUnsupportedSurfaceRejectsAsyncCreation(t *testing.T) {
	s := newTestServer(t, &testBackend{})
	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"async": true,
		"questions": []map[string]interface{}{{
			"prompt":  "Which?",
			"options": []map[string]interface{}{{"label": "A", "description": "A"}, {"label": "B", "description": "B"}},
		}},
	})
	require.True(t, result.IsError)
}

func TestAsyncQuestionsRootCanReadChildQuestions(t *testing.T) {
	backend := &testBackend{}
	log := newTestLogger(t)
	server := NewWithProfile(backend, "root-session", "root-task", 10005, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityAsyncQuestions}, nil))
	tools := getRegisteredToolNames(server)
	assert.Contains(t, tools, "list_question_answers_kandev")
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.NotContains(t, tools, "wait_for_question_answers_kandev")
}
