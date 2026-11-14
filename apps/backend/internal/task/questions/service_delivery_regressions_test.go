package questions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
)

func TestAsyncQuestionOperatorAnswerCreatesParentOutcomeDelivery(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-child-outcome", "session-child-outcome", "workspace-child-outcome", "task-parent-outcome")
	seedExplicitQuestionBinding(t, repo, "task-parent-outcome", "session-parent-outcome", "workspace-parent-outcome", "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-child-outcome", WorkspaceID: "workspace-child-outcome", SessionID: "session-child-outcome",
		SessionIncarnationID: "inc-session-child-outcome", Recipient: models.AsyncQuestionRecipientParent,
		ParentTaskID: "task-parent-outcome", ParentSession: "session-parent-outcome",
		ParentSessionIncarnationID: "inc-session-parent-outcome",
		Questions:                  []protocol.Question{{Prompt: "Which?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		RequestID:                  "parent-outcome-operator",
	})
	require.NoError(t, err)
	_, err = svc.Forward(ctx, result.Question.ID, "task-parent-outcome", "session-parent-outcome", "operator decision")
	require.NoError(t, err)
	_, won, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"a"}}},
	})
	require.NoError(t, err)
	require.True(t, won)
	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	require.NoError(t, err)
	parentOutcomes := 0
	for _, delivery := range deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryParentOutcome {
			parentOutcomes++
		}
	}
	require.Equal(t, 1, parentOutcomes)
}

func TestAsyncQuestionForwardOnlyNotifiesOnNewPendingTransition(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-child-forward-notify", "session-child-forward-notify", "workspace-child-forward-notify", "task-parent-forward-notify")
	seedExplicitQuestionBinding(t, repo, "task-parent-forward-notify", "session-parent-forward-notify", "workspace-parent-forward-notify", "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-child-forward-notify", WorkspaceID: "workspace-child-forward-notify", SessionID: "session-child-forward-notify",
		SessionIncarnationID: "inc-session-child-forward-notify", Recipient: models.AsyncQuestionRecipientParent,
		ParentTaskID: "task-parent-forward-notify", ParentSession: "session-parent-forward-notify",
		ParentSessionIncarnationID: "inc-session-parent-forward-notify",
		Questions:                  []protocol.Question{{Prompt: "Which?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		RequestID:                  "parent-forward-notify",
	})
	require.NoError(t, err)
	recorder := &questionChangeRecorder{}
	svc.SetNotifier(recorder)
	_, err = svc.Forward(ctx, result.Question.ID, "task-parent-forward-notify", "session-parent-forward-notify", "operator decision")
	require.NoError(t, err)
	require.Len(t, recorder.changes, 1)
	require.Equal(t, "forwarded", recorder.changes[0].Kind)

	_, err = svc.Forward(ctx, result.Question.ID, "task-parent-forward-notify", "session-parent-forward-notify", "replay")
	require.NoError(t, err)
	require.Len(t, recorder.changes, 1, "an idempotent forward replay must not notify")
	_, won, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"a"}}},
	})
	require.NoError(t, err)
	require.True(t, won)
	_, err = svc.Forward(ctx, result.Question.ID, "task-parent-forward-notify", "session-parent-forward-notify", "terminal retry")
	require.NoError(t, err)
	require.Len(t, recorder.changes, 2, "a terminal forward must not send another forwarded notification")
}

func TestAsyncQuestionRejectedDeliveryUsesRejectedState(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createParentQuestion(t, svc, repo, "rejected-delivery")
	_, err := svc.Forward(ctx, result.Question.ID, result.Question.ParentTaskID, result.Question.ParentSessionID, "operator decision")
	require.NoError(t, err)
	_, won, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Rejected: true, RejectReason: "Needs a product decision",
	})
	require.NoError(t, err)
	require.True(t, won)
	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	require.NoError(t, err)
	var owner, parent *models.AsyncQuestionDelivery
	for _, delivery := range deliveries {
		switch delivery.Kind {
		case models.AsyncQuestionDeliveryOwner:
			owner = delivery
		case models.AsyncQuestionDeliveryParentOutcome:
			parent = delivery
		}
	}
	require.NotNil(t, owner)
	require.Contains(t, owner.Content, "ASYNC QUESTION REJECTED")
	require.NotContains(t, owner.Content, "ASYNC QUESTION ANSWERED")
	require.NotNil(t, parent)
	require.Contains(t, parent.Content, "ASYNC CHILD QUESTION REJECTED")
	require.Equal(t, "rejected", parent.Metadata[models.MetaKeyParentQuestionStatus])
}

type failingAuthoritySweepStore struct {
	questions.Store
	err error
}

func (s failingAuthoritySweepStore) ReconcileAsyncQuestionAuthority(
	context.Context,
	int,
	models.AsyncQuestionWakeBuilder,
) ([]*models.AsyncQuestion, error) {
	return nil, s.err
}

func TestAsyncQuestionDeliveryAdmissionContinuesAfterAuthoritySweepError(t *testing.T) {
	base, repo := newQuestionsService(t)
	ctx := context.Background()
	created := createOperatorQuestion(t, base, repo, "session-sweep-error-delivery")
	_, won, err := base.ResolveOperator(ctx, created.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: created.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
	})
	require.NoError(t, err)
	require.True(t, won)

	sweepErr := errors.New("authority query unavailable")
	svc := questions.NewService(failingAuthoritySweepStore{Store: repo, err: sweepErr}, nil, zap.NewNop())
	queue := &recordingQueue{}
	svc.SetDeliveryQueue(queue)
	svc.SetTranscriptWriter(testTranscriptWriter{})
	admitted, err := svc.ReconcileOnce(ctx)
	require.ErrorIs(t, err, sweepErr)
	require.Equal(t, 2, admitted, "due transcript and answer deliveries must still be admitted")
	require.Len(t, queue.deliveries, 1)
	require.Equal(t, models.AsyncQuestionDeliveryOwner, queue.deliveries[0].Kind)
}

func TestAsyncQuestionOwnerDeliveryOmitsSubmissionIdentity(t *testing.T) {
	svc, repo := newQuestionsService(t)
	created := createOperatorQuestion(t, svc, repo, "session-private-submission")
	_, won, err := svc.ResolveOperator(context.Background(), created.Question.ID, questions.AnswerInput{
		Answers:      []questions.Answer{{QuestionID: created.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
		SubmissionID: "private-submission-id",
	})
	require.NoError(t, err)
	require.True(t, won)
	deliveries, err := repo.ListAsyncQuestionDeliveries(context.Background(), time.Now().UTC().Add(time.Hour), 20)
	require.NoError(t, err)
	for _, delivery := range deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryOwner {
			require.NotContains(t, delivery.Content, "private-submission-id")
			require.NotContains(t, delivery.Content, "submission_id")
			return
		}
	}
	t.Fatal("owner outcome delivery not found")
}
