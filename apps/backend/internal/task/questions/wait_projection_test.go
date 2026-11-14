package questions_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
)

func TestAsyncQuestionWaitProjectionPublishesParkAndRelease(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	const sessionID = "session-wait-projection-events"
	request := createOperatorQuestion(t, svc, repo, sessionID)
	startedAt := time.Now().UTC()
	const turnID = "turn-wait-projection-events"
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: "task-" + sessionID,
		StartedAt: startedAt, CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	notifier := &questionChangeRecorder{}
	svc.SetNotifier(notifier)
	wait, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-" + sessionID, SessionID: sessionID, TurnID: turnID,
		Dependencies: []string{request.Question.ID}, Reason: "remaining work depends on the answer",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	notifier.changes = nil
	if _, err := svc.AcknowledgeWaitPause(ctx, wait.Wait.ID); err != nil {
		t.Fatalf("acknowledge pause: %v", err)
	}
	assertWaitPendingAction(t, repo, sessionID, models.TaskPendingActionClarification)
	assertWaitChangeCount(t, notifier.changes, 1, "parked")

	_, won, err := svc.ResolveOperator(ctx, request.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: request.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
	})
	if err != nil || !won {
		t.Fatalf("answer: won=%v err=%v", won, err)
	}
	assertWaitPendingAction(t, repo, sessionID, "")
	assertWaitChangeCount(t, notifier.changes, 2, "released")
}

func assertWaitPendingAction(t *testing.T, repo interface {
	GetPendingActionsBySessionIDs(context.Context, []string) (map[string]models.TaskPendingAction, error)
}, sessionID string, want models.TaskPendingAction) {
	t.Helper()
	got, err := repo.GetPendingActionsBySessionIDs(context.Background(), []string{sessionID})
	if err != nil {
		t.Fatalf("get pending actions: %v", err)
	}
	if got[sessionID] != want {
		t.Fatalf("pending action = %q, want %q", got[sessionID], want)
	}
}

func assertWaitChangeCount(t *testing.T, changes []questions.QuestionChange, want int, state string) {
	t.Helper()
	count := 0
	for _, change := range changes {
		if change.Kind == "wait_changed" {
			count++
		}
	}
	if count != want {
		t.Fatalf("notifier changes = %+v, want %d wait_changed events after %s", changes, want, state)
	}
}
