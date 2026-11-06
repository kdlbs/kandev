package questions_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
)

type answerBeforeWaitRecheckStore struct {
	questions.Store
	requestID  string
	questionID string
	listCalls  int
}

func (s *answerBeforeWaitRecheckStore) ListAsyncQuestions(
	ctx context.Context,
	opts models.ListAsyncQuestionsOptions,
) (*models.AsyncQuestionPage, error) {
	s.listCalls++
	if s.listCalls == 2 {
		_, _, err := s.ResolveAsyncQuestion(ctx, s.requestID, models.AsyncQuestionOutcome{
			Answers: []protocol.Answer{{QuestionID: s.questionID, SelectedOptions: []string{"default"}}},
		}, nil)
		if err != nil {
			return nil, err
		}
	}
	return s.Store.ListAsyncQuestions(ctx, opts)
}

func TestAsyncQuestionsWaitReadyLeavesNoGate(t *testing.T) {
	base, repo := newQuestionsService(t)
	request := createOperatorQuestion(t, base, repo, "session-wait-ready-race")
	store := &answerBeforeWaitRecheckStore{
		Store:      repo,
		requestID:  request.Question.ID,
		questionID: request.Question.Questions[0].ID,
	}
	svc := questions.NewService(store, nil, zap.NewNop())

	result, err := svc.Wait(context.Background(), questions.WaitInput{
		TaskID: "task-session-wait-ready-race", SessionID: "session-wait-ready-race",
		Dependencies: []string{request.Question.ID}, Reason: "remaining work needs the decision",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.Status != questions.WaitStatusReady || result.Wait != nil {
		t.Fatalf("result = %+v, want ready without a gate", result)
	}
	if svc.HasParkedWait(context.Background(), "session-wait-ready-race") {
		t.Fatal("a dependency resolved before the wait re-check, but its gate remains active")
	}
}
