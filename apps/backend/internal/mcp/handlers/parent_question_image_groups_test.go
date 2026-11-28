package handlers

import (
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.4
func TestParentQuestionImageGroupsRejected(t *testing.T) {
	for name, groups := range map[string]any{
		"valid groups": []map[string]any{{"kind": "single", "image_ids": []string{"a"}}},
		"null groups":  nil,
		"wrong shape":  "pair",
	} {
		t.Run(name, func(t *testing.T) {
			msg := makeWSMessage(t, ws.ActionMCPAskParentQuestion, map[string]any{
				"session_id": "child",
				"questions": []map[string]any{{
					"prompt":       "Choose",
					"options":      []map[string]any{{"label": "A"}, {"label": "B"}},
					"image_groups": groups,
				}},
			})
			_, err := parentQuestionRequestFromMessage(msg)
			require.NotNil(t, err)
			require.Equal(t, ws.ErrorCodeValidation, err.code)
			require.Contains(t, err.message, "do not support images")
		})
	}
}
