package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPromptTaskExpectedIdentityRejectsReplacementAfterClaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-prompt-identity", "session-prompt-identity"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, "execution-prompt-identity")

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatalf("resolve session identity: %v", err)
	}

	_, err = svc.promptTask(
		ctx, taskID, sessionID, "queued prompt", "", false, nil, true,
		promptTaskOptions{
			expectedSessionIdentity: &identity,
			afterClaim: func() error {
				_, updateErr := repo.DB().ExecContext(
					ctx,
					`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`,
					"replacement-incarnation",
					sessionID,
				)
				return updateErr
			},
		},
	)
	if !errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
		t.Fatalf("promptTask error = %v, want session identity mismatch", err)
	}
	if got := len(agentMgr.capturedPrompts); got != 0 {
		t.Fatalf("replacement session prompts = %d, want 0", got)
	}
}
