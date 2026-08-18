package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestProcessOnEnterResetAgentContext_ClearsLazyResumeTokenWithoutLiveExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-lazy-resume", "session-lazy-resume", "step-work")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:          "session-lazy-resume",
		SessionID:   "session-lazy-resume",
		TaskID:      "task-lazy-resume",
		ResumeToken: "old-acp-session",
		Status:      "stopped",
	}); err != nil {
		t.Fatalf("seed resumable execution: %v", err)
	}

	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	step := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "workflow-1", Name: "Review",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
		}},
	}
	session, err := repo.GetTaskSession(ctx, "session-lazy-resume")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	svc.processOnEnter(ctx, "task-lazy-resume", session, step, "review task")

	if len(agentManager.restartProcessCalls) != 0 {
		t.Fatalf("expected no reset against a missing live execution, got %d calls", len(agentManager.restartProcessCalls))
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-lazy-resume")
	if err != nil {
		t.Fatalf("load resumable execution: %v", err)
	}
	if running.ResumeToken != "" {
		t.Fatalf("reset must clear lazy resume before the next agent turn, got %q", running.ResumeToken)
	}
}
