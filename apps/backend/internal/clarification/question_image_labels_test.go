package clarification

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func labelledQuestions(t *testing.T, images string) []Question {
	t.Helper()
	qs, err := imageQuestions(t, images)
	require.NoError(t, err)
	return qs
}

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-009.1
func TestQuestionImageLabelsAccepted(t *testing.T) {
	qs := labelledQuestions(t, `[{"id":"a","path":"a.png","alt":"A","label":"before"},{"id":"b","path":"b.png","alt":"B","label":"after_v2"},{"id":"c","path":"c.png","alt":"C"}]`)
	require.Empty(t, NormalizeAndValidateQuestions(qs))
	require.Equal(t, "before", qs[0].Images[0].Label)
}

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-009.2
func TestQuestionImageLabelsRejected(t *testing.T) {
	for name, label := range map[string]string{
		"space":     "before after",
		"too long":  strings.Repeat("a", 17),
		"symbol":    "before!",
		"newline":   "a\nb",
		"leading -": "-before",
	} {
		t.Run(name, func(t *testing.T) {
			qs := labelledQuestions(t, `[{"id":"a","path":"a.png","alt":"A","label":`+quoteJSON(label)+`}]`)
			require.NotEmpty(t, NormalizeAndValidateQuestions(qs))
		})
	}
	dup := labelledQuestions(t, `[{"id":"a","path":"a.png","alt":"A","label":"same"},{"id":"b","path":"b.png","alt":"B","label":"same"}]`)
	require.NotEmpty(t, NormalizeAndValidateQuestions(dup))
}

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-009.3
func TestQuestionImageLabelsAreRequestIdentity(t *testing.T) {
	a := labelledQuestions(t, `[{"id":"a","path":"a.png","alt":"A","label":"before"}]`)
	b := labelledQuestions(t, `[{"id":"a","path":"a.png","alt":"A","label":"after"}]`)
	none := labelledQuestions(t, `[{"id":"a","path":"a.png","alt":"A"}]`)
	require.NotEqual(t, PendingIDForRequest("s", "key", a, ""), PendingIDForRequest("s", "key", b, ""))
	require.NotEqual(t, PendingIDForRequest("s", "key", a, ""), PendingIDForRequest("s", "key", none, ""))
	require.False(t, questionsEqual(a, b))
}

func quoteJSON(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(value) + `"`
}
