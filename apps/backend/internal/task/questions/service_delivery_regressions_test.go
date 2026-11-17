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
	forwardedQuestion, err := svc.Get(ctx, result.Question.ID)
	require.NoError(t, err)
	winningVersion := forwardedQuestion.Version + 1
	_, won, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"a"}}},
	})
	require.NoError(t, err)
	require.True(t, won)
	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	require.NoError(t, err)
	parentOutcomes := 0
	ownerOutcomes := 0
	for _, delivery := range deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryParentOutcome ||
			delivery.Kind == models.AsyncQuestionDeliveryOwner {
			require.Equal(t, winningVersion, delivery.OutcomeVersion)
			require.Equal(t, float64(winningVersion), delivery.Metadata[questions.MetadataAsyncQuestionOutcomeVersion],
				"correlated metadata must pin the persisted winning outcome version")
		}
		if delivery.Kind == models.AsyncQuestionDeliveryOwner {
			ownerOutcomes++
		}
		if delivery.Kind == models.AsyncQuestionDeliveryParentOutcome {
			parentOutcomes++
		}
	}
	require.Equal(t, 1, parentOutcomes)
	require.Equal(t, 1, ownerOutcomes)
}

func TestAsyncQuestionEmptyAnswerDoesNotResolve(t *testing.T) {
	svc, repo := newQuestionsService(t)
	created := createOperatorQuestion(t, svc, repo, "session-empty-answer")

	_, won, err := svc.ResolveOperator(context.Background(), created.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{
			QuestionID: created.Question.Questions[0].ID,
			CustomText: " \n\t ",
		}},
	})

	require.ErrorIs(t, err, questions.ErrValidation)
	require.False(t, won)
	current, getErr := svc.Get(context.Background(), created.Question.ID)
	require.NoError(t, getErr)
	require.Equal(t, models.AsyncQuestionStatusPending, current.Status)
	require.Nil(t, current.Outcome)
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

type failingDeliveryFailureStore struct {
	questions.Store
	markErr  error
	listErr  error
	listCall int
}

func (s *failingDeliveryFailureStore) MarkAsyncQuestionDeliveryFailed(
	context.Context, string, int, time.Time, string,
) error {
	return s.markErr
}

func (s *failingDeliveryFailureStore) ListAsyncQuestionDeliveries(
	ctx context.Context, now time.Time, limit int,
) ([]*models.AsyncQuestionDelivery, error) {
	s.listCall++
	if s.listCall > 1 {
		return nil, s.listErr
	}
	return s.Store.ListAsyncQuestionDeliveries(ctx, now, limit)
}

type failingDeliveryQueue struct {
	err   error
	calls int
}

func (q *failingDeliveryQueue) AdmitAsyncQuestionDelivery(
	context.Context, *models.AsyncQuestionDelivery,
) error {
	q.calls++
	return q.err
}

func (q *failingDeliveryQueue) RemoveAsyncQuestionDelivery(
	context.Context, *models.AsyncQuestionDelivery,
) error {
	return nil
}

func TestAsyncQuestionDeliveryFailurePersistenceStopsReconcileLoop(t *testing.T) {
	base, repo := newQuestionsService(t)
	created := createOperatorQuestion(t, base, repo, "session-delivery-failure-persist")
	_, won, err := base.ResolveOperator(context.Background(), created.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{
			QuestionID:      created.Question.Questions[0].ID,
			SelectedOptions: []string{"default"},
		}},
	})
	require.NoError(t, err)
	require.True(t, won)

	markErr := errors.New("could not persist delivery retry")
	store := &failingDeliveryFailureStore{
		Store: repo, markErr: markErr, listErr: errors.New("second delivery page should not be read"),
	}
	queue := &failingDeliveryQueue{err: errors.New("queue unavailable")}
	svc := questions.NewService(store, nil, zap.NewNop())
	svc.SetTranscriptWriter(testTranscriptWriter{})
	svc.SetDeliveryQueue(queue)

	admitted, reconcileErr := svc.ReconcileOnce(context.Background())

	require.ErrorIs(t, reconcileErr, markErr)
	require.Equal(t, 1, admitted, "the transcript intent precedes the failed outcome admission")
	require.Equal(t, 1, queue.calls)
	require.Equal(t, 1, store.listCall, "a persistence failure must return before the due intent is re-listed")
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
