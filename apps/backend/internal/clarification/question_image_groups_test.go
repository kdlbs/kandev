package clarification

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const groupImages = `[{"id":"before","path":"b.png","alt":"Before"},{"id":"after","path":"a.png","alt":"After"},{"id":"new","path":"n.png","alt":"New"}]`

func groupedQuestions(t *testing.T, groups string) []Question {
	t.Helper()
	var questions []Question
	payload := `[{"id":"layout","prompt":"Approve?","options":[{"label":"Yes"},{"label":"No","images":[{"id":"choice","path":"c.png","alt":"Choice"}]}],"images":` + groupImages + `,"image_groups":` + groups + `}]`
	require.NoError(t, json.Unmarshal([]byte(payload), &questions))
	return questions
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.1
func TestQuestionImageGroupsAccepted(t *testing.T) {
	qs := groupedQuestions(t, `[{"kind":"pair","image_ids":["before","after"]},{"kind":"single","image_ids":["choice"]}]`)
	require.Empty(t, NormalizeAndValidateQuestions(qs))
	require.Len(t, qs[0].ImageGroups, 2)
	require.Equal(t, []string{"before", "after"}, qs[0].ImageGroups[0].ImageIDs)
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.2
func TestQuestionImageGroupsRejected(t *testing.T) {
	for name, tc := range map[string]struct{ groups, want string }{
		"pair of one":       {`[{"kind":"pair","image_ids":["before"]}]`, "a pair must list exactly 2 different image IDs"},
		"pair of three":     {`[{"kind":"pair","image_ids":["before","after","new"]}]`, "a pair must list exactly 2 different image IDs"},
		"pair of same":      {`[{"kind":"pair","image_ids":["before","before"]}]`, "a pair must list exactly 2 different image IDs"},
		"single of two":     {`[{"kind":"single","image_ids":["before","after"]}]`, "a single must list exactly 1 image ID"},
		"unknown kind":      {`[{"kind":"trio","image_ids":["before"]}]`, `kind must be "pair" or "single"`},
		"unknown image":     {`[{"kind":"single","image_ids":["missing"]}]`, `image "missing" is not an image of this question`},
		"repeated image":    {`[{"kind":"single","image_ids":["before"]},{"kind":"pair","image_ids":["before","after"]}]`, `image "before" is in more than one image group`},
		"too many groups":   {`[{"kind":"single","image_ids":["before"]},{"kind":"single","image_ids":["after"]},{"kind":"single","image_ids":["new"]},{"kind":"single","image_ids":["choice"]},{"kind":"single","image_ids":["before"]}]`, "at most 4 image groups"},
		"unknown field":     {`[{"kind":"single","image_ids":["before"],"extra":1}]`, ""},
		"null group list":   {`null`, ""},
		"null group object": {`[null]`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			var questions []Question
			payload := `[{"id":"layout","prompt":"Approve?","options":[{"label":"Yes"},{"label":"No","images":[{"id":"choice","path":"c.png","alt":"Choice"}]}],"images":` + groupImages + `,"image_groups":` + tc.groups + `}]`
			if err := json.Unmarshal([]byte(payload), &questions); err != nil {
				require.Empty(t, tc.want, "decode errors are only expected for malformed JSON shapes")
				return
			}
			msg := NormalizeAndValidateQuestions(questions)
			require.NotEmpty(t, msg)
			require.Contains(t, msg, `question 1 ("layout")`)
			require.Contains(t, msg, tc.want)
		})
	}
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.2
func TestQuestionImageGroupsRequireImages(t *testing.T) {
	var questions []Question
	require.NoError(t, json.Unmarshal([]byte(`[{"id":"plain","prompt":"Approve?","options":[{"label":"Yes"},{"label":"No"}],"image_groups":[{"kind":"single","image_ids":["x"]}]}]`), &questions))
	msg := NormalizeAndValidateQuestions(questions)
	require.Contains(t, msg, `question 1 ("plain")`)
	require.Contains(t, msg, "image_groups: groups need images on the question or its choices")
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.5
func TestQuestionImageGroupsAreRequestIdentity(t *testing.T) {
	pair := groupedQuestions(t, `[{"kind":"pair","image_ids":["before","after"]}]`)
	swapped := groupedQuestions(t, `[{"kind":"pair","image_ids":["after","before"]}]`)
	singles := groupedQuestions(t, `[{"kind":"single","image_ids":["before"]}]`)
	none := labelledQuestions(t, groupImages)
	require.NotEqual(t, PendingIDForRequest("s", "key", pair, ""), PendingIDForRequest("s", "key", swapped, ""))
	require.NotEqual(t, PendingIDForRequest("s", "key", pair, ""), PendingIDForRequest("s", "key", singles, ""))
	require.False(t, questionsEqual(pair, swapped))
	require.False(t, questionsEqual(pair, singles))
	require.False(t, questionsEqual(pair, none))
	require.True(t, questionsEqual(pair, groupedQuestions(t, `[{"kind":"pair","image_ids":["before","after"]}]`)))
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.7
func TestQuestionsWithoutImageGroupsKeepTheirWireShape(t *testing.T) {
	qs := labelledQuestions(t, groupImages)
	require.Empty(t, NormalizeAndValidateQuestions(qs))
	data, err := json.Marshal(qs[0])
	require.NoError(t, err)
	require.NotContains(t, string(data), "image_groups")
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.2
func TestQuestionImageGroupErrorsOmitAMissingQuestionID(t *testing.T) {
	var questions []Question
	require.NoError(t, json.Unmarshal([]byte(`[{"prompt":"Approve?","options":[{"label":"Yes"},{"label":"No"}],"images":`+groupImages+`,"image_groups":[{"kind":"pair","image_ids":["before"]}]}]`), &questions))
	msg := NormalizeAndValidateQuestions(questions)
	require.Contains(t, msg, "question 1: image_groups: a pair must list exactly 2 different image IDs")
}

// listedQuestion projects one stored question through the pending-question listing.
func listedQuestion(t *testing.T, question Question) string {
	t.Helper()
	data, err := json.Marshal(question)
	require.NoError(t, err)
	var stored map[string]any
	require.NoError(t, json.Unmarshal(data, &stored))
	listed := BundleQuestionStatuses([]*taskmodels.Message{{Metadata: map[string]any{
		"question_id": question.ID,
		"question":    stored,
	}}})
	projected, err := json.Marshal(listed)
	require.NoError(t, err)
	return string(projected)
}

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.1
func TestQuestionImageGroupsListedWithPendingQuestions(t *testing.T) {
	grouped := groupedQuestions(t, `[{"kind":"pair","image_ids":["before","after"]}]`)[0]
	require.Contains(t, listedQuestion(t, grouped), `"image_groups":[{"kind":"pair","image_ids":["before","after"]}]`)

	ungrouped := groupedQuestions(t, `[]`)[0]
	ungrouped.ImageGroups = nil
	require.NotContains(t, listedQuestion(t, ungrouped), `"image_groups"`)
}
