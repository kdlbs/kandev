package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAsyncQuestionsPauseRejectsSupersededCapturedTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-async-pause", "session-async-pause", models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, "session-async-pause", "task-async-pause", "exec-async-pause")
	setSessionExecID(t, repo, "session-async-pause", "exec-async-pause")
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-successor", TaskID: "task-async-pause", TaskSessionID: "session-async-pause",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create successor turn: %v", err)
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = fixedActiveTurn{TurnService: &repoBackedTurnService{repo: repo}, id: "turn-successor"}

	captured := "turn-captured"
	_, err := svc.PauseForClarificationInputWithOptions(ctx, "session-async-pause", ClarificationPauseOptions{
		Force: true, ExpectedTurnID: &captured,
	})
	if !errors.Is(err, ErrStaleQuestionWaitPause) {
		t.Fatalf("pause error = %v, want ErrStaleQuestionWaitPause", err)
	}
	session, err := repo.GetTaskSession(ctx, "session-async-pause")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("successor session state = %s, want RUNNING", session.State)
	}
	active, err := svc.turnService.GetActiveTurn(ctx, "session-async-pause")
	if err != nil || active == nil || active.ID != "turn-successor" {
		t.Fatalf("active turn = %+v err=%v, want the successor", active, err)
	}
}

type fixedActiveTurn struct {
	TurnService
	id string
}

func (f fixedActiveTurn) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return &models.Turn{ID: f.id}, nil
}
