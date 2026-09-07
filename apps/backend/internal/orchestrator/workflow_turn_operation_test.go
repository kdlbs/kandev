package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const triggerScopedWorkflowJSON = `{
  "version": 1,
  "type": "kandev_workflow",
  "workflows": [{
    "name": "Trigger-scoped operations",
    "steps": [
      {
        "name": "Start",
        "position": 0,
        "events": {"on_turn_start": [{"type": "move_to_next"}]}
      },
      {
        "name": "Complete",
        "position": 1,
        "events": {"on_turn_complete": [{"type": "move_to_next"}]}
      },
      {"name": "Done", "position": 2, "events": {}}
    ]
  }]
}`

func TestWorkflowTurnStartAndCompleteDoNotShareOperationIdentity(t *testing.T) {
	ctx := context.Background()
	stepGetter, stepIDs := buildWorkflowFromJSON(t, triggerScopedWorkflowJSON)
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", stepIDs["Start"])
	setSessionExecID(t, repo, "session-1", "execution-1")

	service := createEngineService(t, repo, stepGetter, &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
	})
	service.turnService = &repoBackedTurnService{repo: repo}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-1",
		TaskSessionID: "session-1",
		StartedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed active turn: %v", err)
	}

	session, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("load session before turn start: %v", err)
	}
	if !service.processOnTurnStartViaEngine(ctx, "task-1", session) {
		t.Fatal("on_turn_start did not transition from Start to Complete")
	}
	assertStepByName(t, ctx, repo, "session-1", "Complete", stepIDs)

	setSessionState(t, ctx, repo, "session-1", models.TaskSessionStateRunning)
	session, err = repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("reload session before turn complete: %v", err)
	}
	if !service.processOnTurnCompleteViaEngine(ctx, "task-1", session) {
		t.Fatal("on_turn_complete was incorrectly deduplicated with on_turn_start")
	}
	assertStepByName(t, ctx, repo, "session-1", "Done", stepIDs)
}
