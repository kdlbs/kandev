package mcp

import (
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ANSWER-ATTACHMENTS-001.1
// @covers AC-TASKS-ANSWER-ATTACHMENTS-001.2
func TestExtractQuestionAnswerAttachments(t *testing.T) {
	result := map[string]interface{}{
		"answers": []interface{}{map[string]interface{}{
			"question_id":      "flow",
			"selected_options": []interface{}{"q1_opt1"},
			"attachments": []interface{}{map[string]interface{}{
				"attachment_id": "att-1",
				"type":          "image",
				"name":          "screen.png",
				"mime_type":     "image/png",
				"size_bytes":    float64(123),
				"delivery_mode": "path",
				"path":          "/workspace/.kandev/attachments/session/screen.png",
			}},
		}},
	}
	questions := []map[string]interface{}{
		{"id": "flow", "options": []map[string]interface{}{{"option_id": "q1_opt1", "label": "A"}}},
	}
	want := `{
		"flow": {
			"selected_option": "q1_opt1",
			"selected_label": "A",
			"attachments": [{
				"attachment_id": "att-1",
				"type": "image",
				"name": "screen.png",
				"mime_type": "image/png",
				"size_bytes": 123,
				"delivery_mode": "path",
				"path": "/workspace/.kandev/attachments/session/screen.png"
			}]
		}
	}`
	toolResult := extractQuestionAnswers(result, questions)
	require.False(t, toolResult.IsError)
	content, ok := toolResult.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	require.JSONEq(t, want, content.Text)
}
