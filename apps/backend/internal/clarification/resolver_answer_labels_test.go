package clarification

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-ANSWER-LABELS-001.5
func TestResolverDetachedResumeIncludesSelectedLabels(t *testing.T) {
	t.Run("single question", func(t *testing.T) {
		const pendingID = "pending-answer-labels-single"
		message := resumeLabelQuestionMessage(pendingID, "message-q1", "q1", 0, []Option{
			{ID: "run", Label: "Run checks"},
			{ID: "skip", Label: "Skip checks"},
		})
		request := acceptResumeLabelAnswer(t, pendingID, []*models.Message{message}, []Answer{
			{QuestionID: "q1", SelectedOptions: []string{"run"}},
		})

		require.Equal(t, `User selected: ["Run checks" (run)]`, request.AnswerText)
	})

	t.Run("bundle order and unresolved labels", func(t *testing.T) {
		const pendingID = "pending-answer-labels-bundle"
		messages := []*models.Message{
			resumeLabelQuestionMessage(pendingID, "message-q2", "q2", 1, []Option{
				{ID: "shared", Label: "Review changes"},
				{ID: "other-q2", Label: "Skip review"},
			}),
			resumeLabelQuestionMessage(pendingID, "message-q3", "q3", 2, []Option{
				{ID: "unlabeled", Label: ""},
				{ID: "other-q3", Label: "Continue"},
			}),
			resumeLabelQuestionMessage(pendingID, "message-q4", "q4", 3, []Option{
				{ID: "ambiguous", Label: "First label"},
				{ID: "ambiguous", Label: "Second label"},
			}),
			resumeLabelQuestionMessage(pendingID, "message-q1", "q1", 0, []Option{
				{ID: "shared", Label: "Run checks"},
				{ID: "other-q1", Label: "Skip checks"},
			}),
		}
		request := acceptResumeLabelAnswer(t, pendingID, messages, []Answer{
			{QuestionID: "q4", SelectedOptions: []string{"ambiguous"}},
			{QuestionID: "q3", SelectedOptions: []string{"unlabeled"}},
			{QuestionID: "q1", SelectedOptions: []string{"shared"}},
			{QuestionID: "q2", SelectedOptions: []string{"shared"}},
		})

		require.Equal(t, "A1: [\"Run checks\" (shared)]\nA2: [\"Review changes\" (shared)]\nA3: [unlabeled]\nA4: [ambiguous]", request.AnswerText)
	})
}

func acceptResumeLabelAnswer(
	t *testing.T,
	pendingID string,
	messages []*models.Message,
	answers []Answer,
) DetachedClarificationResume {
	t.Helper()
	resolver, _, _, _, eventBus := newResolverDeliveryFixture(t, pendingID, messages)
	_, claimed, err := resolver.ResolveBundle(context.Background(), pendingID, Outcome{Answers: answers})
	require.NoError(t, err)
	require.True(t, claimed)
	require.Len(t, eventBus.resumeRequests, 1)
	return eventBus.resumeRequests[0]
}

func resumeLabelQuestionMessage(
	pendingID, messageID, questionID string,
	index int,
	options []Option,
) *models.Message {
	storedOptions := make([]any, 0, len(options))
	for _, option := range options {
		storedOptions = append(storedOptions, map[string]any{
			"option_id": option.ID,
			"label":     option.Label,
		})
	}
	return &models.Message{
		ID:            messageID,
		TaskID:        "task-1",
		TaskSessionID: "session-1",
		TurnID:        "turn-1",
		Metadata: map[string]any{
			metaPendingIDKey:  pendingID,
			metaQuestionIDKey: questionID,
			metaStatusKey:     string(StatusPending),
			"question_index":  index,
			metaQuestionKey: map[string]any{
				"id":      questionID,
				"prompt":  "Choose an action",
				"options": storedOptions,
			},
		},
	}
}
