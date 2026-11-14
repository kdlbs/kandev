package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAsyncQuestionsParkedGateProjectsPendingAction(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-parked-projection", "session-parked-projection")
	createAsyncQuestionForTest(t, repo, question)
	startedAt := time.Now().UTC()
	const turnID = "turn-parked-projection"
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: question.SessionID, TaskID: question.TaskID,
		StartedAt: startedAt, CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if _, err := repo.CreateAsyncQuestionWait(ctx, &models.AsyncQuestionWait{
		TaskID: question.TaskID, WorkspaceID: question.WorkspaceID,
		SessionID: question.SessionID, TurnID: turnID,
		DependencyIDs: []string{question.ID}, Status: models.AsyncQuestionWaitParked,
	}); err != nil {
		t.Fatalf("create parked gate: %v", err)
	}
	got, err := repo.GetPendingActionsBySessionIDs(ctx, []string{question.SessionID})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if got[question.SessionID] != models.TaskPendingActionClarification {
		t.Fatalf("pending action = %#v, want clarification", got[question.SessionID])
	}
}

func TestAsyncQuestionsOpenRequestProjectsNoPendingAction(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-open-projection", "session-open-projection")
	createAsyncQuestionForTest(t, repo, question)
	got, err := repo.GetPendingActionsBySessionIDs(ctx, []string{question.SessionID})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, exists := got[question.SessionID]; exists {
		t.Fatalf("an open request without an explicit wait projected action %#v", got[question.SessionID])
	}
}

func TestAsyncQuestionsWaitStoresWorkspaceScope(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-wait-workspace", "session-wait-workspace")
	createAsyncQuestionForTest(t, repo, question)
	wait, err := repo.CreateAsyncQuestionWait(ctx, &models.AsyncQuestionWait{
		WorkspaceID: question.WorkspaceID,
		TaskID:      question.TaskID,
		SessionID:   question.SessionID,
		DependencyIDs: []string{
			question.ID,
		},
		Status: models.AsyncQuestionWaitRequested,
	})
	if err != nil {
		t.Fatalf("create wait: %v", err)
	}
	stored, err := repo.GetAsyncQuestionWait(ctx, wait.ID)
	if err != nil {
		t.Fatalf("read wait: %v", err)
	}
	if stored.WorkspaceID != question.WorkspaceID {
		t.Fatalf("workspace_id = %q, want %q", stored.WorkspaceID, question.WorkspaceID)
	}
}
