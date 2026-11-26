package clarification

import (
	"testing"

	"github.com/kandev/kandev/internal/clarification/protocol"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.5
func TestBundleQuestionsReadImageGroups(t *testing.T) {
	msg := &taskmodels.Message{ID: "m1", Metadata: map[string]any{
		"question_id": "q1",
		"question": map[string]any{
			"id":     "q1",
			"prompt": "Approve?",
			"images": []any{
				map[string]any{"id": "before", "path": "b.png", "alt": "Before"},
				map[string]any{"id": "after", "path": "a.png", "alt": "After"},
			},
			"image_groups": []any{
				map[string]any{"kind": "pair", "image_ids": []any{"before", "after"}},
			},
		},
	}}
	questions := bundleQuestions([]*taskmodels.Message{msg})
	require.Len(t, questions, 1)
	require.Equal(t, protocol.QuestionImageGroups{{Kind: protocol.ImageGroupPair, ImageIDs: []string{"before", "after"}}}, questions[0].ImageGroups)
}
