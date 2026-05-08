package mcp

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAskUserQuestion_ToolSchema_RequiresQuestionsArray asserts the tool now
// exposes a "questions" array (not legacy "prompt"/"options" top-level fields).
func TestAskUserQuestion_ToolSchema_RequiresQuestionsArray(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	toolsMap := s.mcpServer.ListTools()
	tool, ok := toolsMap["ask_user_question_kandev"]
	require.True(t, ok, "ask_user_question tool not registered")

	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")
	assert.Contains(t, props, "questions", "schema must expose 'questions'")
	assert.Contains(t, props, "context")
	assert.NotContains(t, props, "prompt", "legacy 'prompt' must not be top-level anymore")
	assert.NotContains(t, props, "options", "legacy 'options' must not be top-level anymore")

	required, _ := parsed["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	assert.True(t, requiredSet["questions"], "questions should be required")
}

// TestAskUserQuestion_RejectsLegacyPromptShape ensures payloads using the old
// flat shape return a validation error rather than silently dropping the call.
func TestAskUserQuestion_RejectsLegacyPromptShape(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"prompt": "Which database?",
		"options": []map[string]interface{}{
			{"label": "Postgres", "description": "Relational"},
			{"label": "Mongo", "description": "Document"},
		},
	})
	assert.True(t, result.IsError, "legacy flat shape must surface a validation error")
}

// TestAskUserQuestion_SingleQuestion_PayloadShape exercises the simplest
// happy path: one question with two options.
func TestAskUserQuestion_SingleQuestion_PayloadShape(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"answers": []interface{}{
				map[string]interface{}{
					"question_id":      "q1",
					"selected_options": []interface{}{"q1_opt1"},
				},
			},
		},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"id":     "q1",
				"prompt": "Which database?",
				"options": []map[string]interface{}{
					{"label": "Postgres", "description": "Relational"},
					{"label": "Mongo", "description": "Document"},
				},
			},
		},
	})
	require.False(t, result.IsError, "single-question call should succeed")

	assert.Equal(t, ws.ActionMCPAskUserQuestion, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	questions, ok := payload["questions"].([]map[string]interface{})
	require.True(t, ok, "questions should be normalized to []map")
	require.Len(t, questions, 1)
	assert.Equal(t, "q1", questions[0]["id"])

	// Result text should be a JSON map keyed by question id.
	require.NotEmpty(t, result.Content)
	textBlock, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textBlock.Text), &parsed))
	q1 := parsed["q1"].(map[string]interface{})
	assert.Equal(t, "q1_opt1", q1["selected_option"])
}

// TestAskUserQuestion_MultiQuestion_BuildsMapResponse covers the full multi-q
// path: agent receives a JSON map keyed by every question id.
func TestAskUserQuestion_MultiQuestion_BuildsMapResponse(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"answers": []interface{}{
				map[string]interface{}{
					"question_id":      "db",
					"selected_options": []interface{}{"db_opt1"},
				},
				map[string]interface{}{
					"question_id": "migration",
					"custom_text": "migrate all but flag rows older than 2 years",
				},
				map[string]interface{}{
					"question_id":      "approach",
					"selected_options": []interface{}{"approach_opt2"},
				},
			},
		},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"id":     "db",
				"prompt": "Which database?",
				"options": []map[string]interface{}{
					{"label": "Postgres", "description": "Relational"},
					{"label": "Mongo", "description": "Document"},
				},
			},
			{
				"id":     "migration",
				"prompt": "How to migrate?",
				"options": []map[string]interface{}{
					{"label": "Migrate all", "description": "Keep everything"},
					{"label": "Fresh start", "description": "Drop existing"},
				},
			},
			{
				"id":     "approach",
				"prompt": "Iterative or atomic?",
				"options": []map[string]interface{}{
					{"label": "Iterative", "description": "Step by step"},
					{"label": "Atomic", "description": "One big change"},
				},
			},
		},
	})
	require.False(t, result.IsError)

	require.NotEmpty(t, result.Content)
	textBlock, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textBlock.Text), &parsed))

	require.Len(t, parsed, 3)
	assert.Equal(t, "db_opt1", parsed["db"].(map[string]interface{})["selected_option"])
	assert.Equal(t, "migrate all but flag rows older than 2 years", parsed["migration"].(map[string]interface{})["custom_text"])
	assert.Equal(t, "approach_opt2", parsed["approach"].(map[string]interface{})["selected_option"])
}

// TestAskUserQuestion_RejectsTooManyQuestions caps the bundle at 4 questions —
// past that the agent should batch the work differently.
func TestAskUserQuestion_RejectsTooManyQuestions(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	makeQ := func(id string) map[string]interface{} {
		return map[string]interface{}{
			"id":     id,
			"prompt": "anything?",
			"options": []map[string]interface{}{
				{"label": "yes", "description": "y"},
				{"label": "no", "description": "n"},
			},
		}
	}

	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{
			makeQ("q1"), makeQ("q2"), makeQ("q3"), makeQ("q4"), makeQ("q5"),
		},
	})
	assert.True(t, result.IsError, "more than 4 questions must be rejected")
}

// TestAskUserQuestion_RejectsTooFewOptions guards against degenerate payloads
// (a question with a single option is just an alert, not a real question).
func TestAskUserQuestion_RejectsTooFewOptions(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"id":     "q1",
				"prompt": "?",
				"options": []map[string]interface{}{
					{"label": "only one", "description": "lonely"},
				},
			},
		},
	})
	assert.True(t, result.IsError, "single-option question must be rejected")
}

// TestAskUserQuestion_RejectionPath returns a friendly text result when the
// user skipped the bundle.
func TestAskUserQuestion_RejectionPath(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"rejected":      true,
			"reject_reason": "User skipped",
		},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"id":     "q1",
				"prompt": "Which?",
				"options": []map[string]interface{}{
					{"label": "Yes", "description": "y"},
					{"label": "No", "description": "n"},
				},
			},
		},
	})
	require.False(t, result.IsError)

	textBlock, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	assert.Contains(t, textBlock.Text, "rejected")
	assert.Contains(t, textBlock.Text, "User skipped")
}
