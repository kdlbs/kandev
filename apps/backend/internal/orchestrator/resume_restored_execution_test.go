package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type restoredExecutionManager struct {
	*mockAgentManager
	boundAttempt string
}

func (m *restoredExecutionManager) BindResumeAttempt(_ context.Context, _, attemptID string) error {
	m.boundAttempt = attemptID
	return nil
}

func TestEnsureSessionRunning_BindsRestoredExecutionAfterFailedResume(t *testing.T) {
	ctx := context.Background()
	const taskID, sessionID, executionID = "task-restored", "session-restored", "execution-restored"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	manager := &restoredExecutionManager{mockAgentManager: &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
	}}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	registry := svc.resumeAttemptStore()
	failed, owner := registry.begin(ctx, taskID, sessionID)
	require.True(t, owner)
	failed.finish(registry)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	attempt, err := svc.ensureSessionRunningWithAttempt(ctx, sessionID, session, launchOriginManual)
	require.NoError(t, err)
	require.NotNil(t, attempt, "the restored execution needs callback ownership after the failed resume")
	t.Cleanup(func() { attempt.finish(registry) })
	require.Equal(t, attempt.identity(), manager.boundAttempt)
	require.True(t, svc.resumeAttemptAllowsExecution(sessionID, executionID, manager.boundAttempt))
	require.False(t, svc.resumeAttemptAllowsExecution(sessionID, executionID, failed.identity()))
	require.False(t, svc.resumeAttemptAllowsExecution(sessionID, executionID, ""))

	require.True(t, registry.accept(attempt, executionID))
	attempt.finish(registry)
	next, err := svc.ensureSessionRunningWithAttempt(ctx, sessionID, session, launchOriginManual)
	require.NoError(t, err)
	require.Nil(t, next, "the accepted restored execution keeps normal prompt cancellation")
	require.True(t, svc.resumeAttemptAllowsExecution(sessionID, executionID, manager.boundAttempt))
}

func TestResumeAttemptPromptReadyDoesNotAcceptAPrompt(t *testing.T) {
	registry := newResumeAttemptRegistry()
	attempt, owner := registry.begin(context.Background(), "task", "session")
	require.True(t, owner)
	require.False(t, registry.markPromptReady(attempt), "readiness needs an execution identity")
	attempt.setExecutionID("execution")
	require.True(t, registry.markPromptReady(attempt))
	require.False(t, attempt.accepted, "silent startup must not claim provider prompt acceptance")
	require.True(t, registry.invalidate("session"), "startup cancellation must remain available")
	require.False(t, registry.markPromptReady(attempt), "late readiness must not revive cancellation")
	attempt.finish(registry)
	require.False(t, registry.canPromptWithoutResume("session", "execution"))
}

func TestResumeAttemptPromptReadyPreservesOnlyTheCurrentExecution(t *testing.T) {
	registry := newResumeAttemptRegistry()
	attempt, owner := registry.begin(context.Background(), "task", "session")
	require.True(t, owner)
	attempt.setExecutionID("execution")
	require.True(t, registry.markPromptReady(attempt))
	attempt.finish(registry)
	require.True(t, registry.canPromptWithoutResume("session", "execution"))
	require.False(t, registry.canPromptWithoutResume("session", "other-execution"))

	successor, owner := registry.begin(context.Background(), "task", "session")
	require.True(t, owner)
	require.False(t, registry.markPromptReady(attempt), "an older attempt cannot take ownership")
	successor.finish(registry)
	require.False(t, registry.canPromptWithoutResume("session", "execution"),
		"a failed successor requires a fresh callback binding")
}
