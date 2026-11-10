package clarification

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/clarification/protocol"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

const choiceImageJSON = `[{"id":"layout","prompt":"Choose a layout","options":[{"option_id":"wide","label":"Wide","description":"Wide layout","images":[{"id":"wide","path":"wide.png","alt":"Wide preview"}]},{"option_id":"compact","label":"Compact","description":"Compact layout"}]}]`

func decodeChoiceImages(t *testing.T, raw string) []Question {
	t.Helper()
	var questions []Question
	require.NoError(t, json.Unmarshal([]byte(raw), &questions))
	return questions
}

func TestChoiceImagesProjectionAndIdentity(t *testing.T) {
	questions := decodeChoiceImages(t, choiceImageJSON)
	require.Empty(t, NormalizeAndValidateQuestions(questions))
	require.True(t, protocol.HasQuestionImages(questions))
	encoded, err := json.Marshal(questions[0])
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"path":"wide.png"`)
	var data map[string]any
	require.NoError(t, json.Unmarshal(encoded, &data))
	messages := []*taskmodels.Message{{Metadata: map[string]any{"question_id": "layout", "question": data}}}
	restored := bundleQuestions(messages)
	require.Equal(t, questions, restored)
	listed, err := json.Marshal(BundleQuestionStatuses(messages))
	require.NoError(t, err)
	require.Contains(t, string(listed), `"path":"wide.png"`)
	same := decodeChoiceImages(t, choiceImageJSON)
	require.True(t, questionsEqual(questions, same))
	require.Equal(t, PendingIDForRequest("s", "key", questions, ""), PendingIDForRequest("s", "key", same, ""))
	changed := decodeChoiceImages(t, strings.ReplaceAll(choiceImageJSON, "wide.png", "new.png"))
	require.False(t, questionsEqual(questions, changed))
	require.NotEqual(t, PendingIDForRequest("s", "key", questions, ""), PendingIDForRequest("s", "key", changed, ""))
}

func TestChoiceImagesValidation(t *testing.T) {
	for _, tc := range []struct {
		name, list string
		valid      bool
	}{
		{"one", `[{"id":"a","path":"a.png","alt":"A"}]`, true},
		{"empty", `[]`, true},
		{"null", `null`, false}, {"null item", `[null]`, false}, {"object", `{}`, false},
		{"two", `[{"id":"a","path":"a.png","alt":"A"},{"id":"b","path":"b.png","alt":"B"}]`, false},
		{"traversal", `[{"id":"a","path":"../a.png","alt":"A"}]`, false},
		{"url", `[{"id":"a","path":"https://host/a.png","alt":"A"}]`, false},
		{"svg", `[{"id":"a","path":"a.svg","alt":"A"}]`, false},
		{"unknown", `[{"id":"a","path":"a.png","alt":"A","url":"other"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `[{"prompt":"Choose","options":[{"label":"A","images":` + tc.list + `},{"label":"B"}]}]`
			var questions []Question
			err := json.Unmarshal([]byte(raw), &questions)
			if err != nil {
				require.False(t, tc.valid)
				return
			}
			require.Equal(t, tc.valid, NormalizeAndValidateQuestions(questions) == "")
		})
	}
}

func TestChoiceImagesSharedLimitsAndLegacyJSON(t *testing.T) {
	raw := strings.Replace(choiceImageJSON, `"prompt":"Choose a layout"`, `"prompt":"Choose a layout","images":[{"id":"wide","path":"wide.png","alt":"Wide preview"}]`, 1)
	require.Empty(t, NormalizeAndValidateQuestions(decodeChoiceImages(t, raw)))
	conflict := strings.Replace(raw, `"path":"wide.png"`, `"path":"other.png"`, 1)
	require.NotEmpty(t, NormalizeAndValidateQuestions(decodeChoiceImages(t, conflict)))
	excess := strings.Replace(raw, `"images":[{"id":"wide","path":"wide.png","alt":"Wide preview"}]`, `"images":[{"id":"a","path":"a.png","alt":"A"},{"id":"b","path":"b.png","alt":"B"},{"id":"c","path":"c.png","alt":"C"},{"id":"d","path":"d.png","alt":"D"}]`, 1)
	require.NotEmpty(t, NormalizeAndValidateQuestions(decodeChoiceImages(t, excess)))
	empty := decodeChoiceImages(t, `[{"prompt":"Choose","options":[{"label":"A","images":[]},{"label":"B"}]}]`)
	omitted := decodeChoiceImages(t, `[{"prompt":"Choose","options":[{"label":"A"},{"label":"B"}]}]`)
	require.Equal(t, PendingIDForRequest("s", "key", empty, ""), PendingIDForRequest("s", "key", omitted, ""))
}
