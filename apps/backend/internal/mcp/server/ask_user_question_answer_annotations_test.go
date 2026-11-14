package mcp

import (
	"encoding/json"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.5
func TestExtractQuestionAnswerAnnotations(t *testing.T) {
	result := map[string]interface{}{
		"answers": []interface{}{map[string]interface{}{
			"question_id":      "header",
			"selected_options": []interface{}{"q1_opt2"},
			"annotations": []interface{}{map[string]interface{}{
				"image_id": "after", "x": float64(12), "y": float64(40),
				"width": float64(200), "height": float64(32),
				"image_width": float64(1440), "image_height": float64(900),
				"comment": "Label contrast too low",
			}},
		}},
	}
	questions := []map[string]interface{}{
		{"id": "header", "options": []map[string]interface{}{
			{"option_id": "q1_opt1", "label": "Approve"},
			{"option_id": "q1_opt2", "label": "Revise"},
		}},
	}
	want := `{
		"header": {
			"selected_option": "q1_opt2",
			"selected_label": "Revise",
			"annotations": [{
				"image_id": "after", "x": 12, "y": 40, "width": 200, "height": 32,
				"image_width": 1440, "image_height": 900, "comment": "Label contrast too low"
			}]
		}
	}`
	toolResult := extractQuestionAnswers(result, questions)
	require.False(t, toolResult.IsError)
	content, ok := toolResult.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	require.JSONEq(t, want, content.Text)
}

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.5
func TestAskUserQuestionOutputSchemaAllowsAnnotations(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(askUserQuestionOutputSchema), &schema))
	branches, ok := schema["oneOf"].([]any)
	require.True(t, ok, "output schema must distinguish async receipts from blocking answers")
	var answerBranch map[string]any
	for _, rawBranch := range branches {
		branch, ok := rawBranch.(map[string]any)
		require.True(t, ok)
		if _, ok := branch["additionalProperties"]; ok {
			answerBranch = branch
		}
	}
	require.NotNil(t, answerBranch, "blocking answer schema branch must be present")
	answer := answerBranch["additionalProperties"].(map[string]any)
	annotations, ok := answer["properties"].(map[string]any)["annotations"].(map[string]any)
	require.True(t, ok, "annotations must be an allowed answer key")
	item := annotations["items"].(map[string]any)
	require.ElementsMatch(t,
		[]any{"image_id", "x", "y", "width", "height", "image_width", "image_height", "comment"},
		item["required"])
}
