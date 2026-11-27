package backendapp

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-IMAGE-ITEMS-002.5
func TestQuestionImageGroupsMetadataRoundTrip(t *testing.T) {
	var question clarification.Question
	require.NoError(t, json.Unmarshal([]byte(`{"id":"q","prompt":"Approve?","images":[{"id":"before","path":"b.png","alt":"Before"},{"id":"after","path":"a.png","alt":"After"}],"image_groups":[{"kind":"pair","image_ids":["before","after"]}]}`), &question))
	data, err := json.Marshal(clarificationQuestionData(question, nil))
	require.NoError(t, err)
	var restored clarification.Question
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, question.ImageGroups, restored.ImageGroups)
	require.NotContains(t, clarificationQuestionData(clarification.Question{}, nil), "image_groups")
}
