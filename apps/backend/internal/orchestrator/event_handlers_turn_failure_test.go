package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/automation"
	agentexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestAgentTurnFailedSettlesDurableErrorWithoutStoppingRuntime(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	mc := &mockMessageCreator{}
	svc.messageCreator = mc
	svc.beginPromptAttempt("s1", "execution-1", 7, false)

	data := retainedTurnFailureData()
	svc.handleAgentTurnFailed(ctx, data)

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Len(t, mc.sessionMessages, 1)
	message := mc.sessionMessages[0]
	require.Equal(t, "turn-1", message.turnID)
	require.Equal(t, "error", message.metadata["variant"])
	require.Equal(t, "turn", message.metadata["failure_scope"])
	require.Equal(t, true, message.metadata["runtime_retained"])
	require.Equal(t, "execution-1", message.metadata["execution_id"])
	require.Equal(t, uint64(7), message.metadata["prompt_generation"])
	require.NotEqual(t, true, message.metadata["recovery_actions"])
	require.Equal(t, "refused", message.metadata["recovery_disposition"])
	require.Equal(t, 0, message.metadata["attempts_started"])

	svc.handleAgentTurnFailed(ctx, data)
	require.Len(t, mc.sessionMessages, 1, "duplicate completion must not create another turn error")
	agentMgr.mu.Lock()
	require.Empty(t, agentMgr.stopAgentWithReasonArgs, "a retained turn failure must keep the process alive")
	agentMgr.mu.Unlock()
}

func TestAgentTurnFailureStorageErrorKeepsSessionAdmissionClosed(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	mc := &mockMessageCreator{sessionMessageErr: errors.New("message storage unavailable")}
	svc.messageCreator = mc
	svc.beginPromptAttempt("s1", "execution-1", 7, false)

	svc.handleAgentTurnFailed(ctx, retainedTurnFailureData())

	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	_, ok := svc.promptAttemptForSession("s1")
	require.True(t, ok, "the failed settlement must retain its ownership fence")
}

func TestHandlePromptErrorDoesNotSettleRetainedTurnTwice(t *testing.T) {
	ctx := context.Background()
	svc, mc := newTransientTestService(t)
	err := svc.handlePromptError(ctx, "t1", "s1", models.TaskSessionStateWaitingForInput,
		&lifecycle.RetainedPromptFailureError{
			Message:     "Selected model is at capacity.",
			Disposition: streams.PromptFailureDispositionRetainRuntime,
		})

	var retained *lifecycle.RetainedPromptFailureError
	require.ErrorAs(t, err, &retained)
	session, loadErr := svc.repo.GetTaskSession(ctx, "s1")
	require.NoError(t, loadErr)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	require.Empty(t, mc.sessionMessages)
	task, taskErr := svc.repo.GetTask(ctx, "t1")
	require.NoError(t, taskErr)
	require.Equal(t, v1.TaskStateInProgress, task.State)
}

func TestAgentTurnFailedRoutesAutomationOriginsToTerminalFailureOwner(t *testing.T) {
	for _, origin := range []string{models.TaskOriginAutomationRun, models.TaskOriginAutomationTask} {
		t.Run(origin, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedAutomationTask(t, repo, "t-auto", origin, false)
			now := time.Now().UTC()
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "s-auto", TaskID: "t-auto", State: models.TaskSessionStateRunning,
				StartedAt: now, UpdatedAt: now,
			}))
			seedExecutorRunning(t, repo, "s-auto", "t-auto", "exec-auto")

			agentManager := &mockAgentManager{isAgentRunning: true}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
			svc.executor = agentexecutor.NewExecutor(agentManager, repo, testLogger(), agentexecutor.ExecutorConfig{})
			messageCreator := &mockMessageCreator{}
			svc.messageCreator = messageCreator
			svc.beginPromptAttempt("s-auto", "exec-auto", 9, false)
			svc.observePromptAttempt("s-auto", "exec-auto", 9, true, true)

			var runService interface{}
			if origin == models.TaskOriginAutomationRun {
				automationRuns := &mockAutomationRunService{}
				svc.SetAutomationService(automationRuns)
				runService = automationRuns
			} else {
				require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
					ID: "turn-auto", TaskSessionID: "s-auto", TaskID: "t-auto",
					StartedAt: now, CreatedAt: now, UpdatedAt: now,
				}))
				svc.activeTurns.Store("s-auto", "turn-auto")
				svc.turnService = &repoTurnService{repo: repo}
				binding := &continuityAutomationBindingRecorder{}
				svc.SetAutomationService(binding)
				runService = binding
			}

			data := retainedTurnFailureData()
			data.TaskID = "t-auto"
			data.SessionID = "s-auto"
			data.AgentExecutionID = "exec-auto"
			data.PromptGeneration = 9
			data.TurnID = "turn-auto"
			svc.handleAgentTurnFailed(ctx, data)

			switch automation := runService.(type) {
			case *mockAutomationRunService:
				require.Equal(t, []string{"t-auto"}, automation.failedTaskIDs,
					"automation_run completion must release the task-bound concurrency slot exactly once")
			case *continuityAutomationBindingRecorder:
				require.Equal(t, 1, automation.calls,
					"automation_task completion must settle the exact active run binding once")
				require.Equal(t, "t-auto", automation.taskID)
				require.Equal(t, "s-auto", automation.sessionID)
				require.Equal(t, "turn-auto", automation.turnID)
			default:
				t.Fatalf("unexpected automation failure owner %T", runService)
			}

			waitForStopCall(t, agentManager)
			svc.dynamicSuccessorWorkers.Wait()
			agentManager.mu.Lock()
			require.Len(t, agentManager.stopAgentWithReasonArgs, 1,
				"automation failure must use the existing execution cleanup owner exactly once")
			agentManager.mu.Unlock()
			_, retryOwned := svc.transientRetries.Load("s-auto")
			require.False(t, retryOwned, "automation failure must not create an interactive retained retry")
			for _, message := range messageCreator.sessionMessages {
				require.NotEqual(t, true, message.metadata["runtime_retained"],
					"automation failure must not persist the interactive turn-error presentation")
			}
		})
	}
}

type continuityAutomationBindingRecorder struct {
	mockAutomationRunService
	automationRunBinding
	calls     int
	taskID    string
	sessionID string
	turnID    string
}

func (s *continuityAutomationBindingRecorder) MarkRunTerminalByBinding(
	_ context.Context,
	taskID, sessionID, turnID string,
	status automation.RunStatus,
	errMsg string,
) error {
	s.calls++
	s.taskID, s.sessionID, s.turnID = taskID, sessionID, turnID
	if status != automation.RunStatusFailed {
		return errors.New("automation binding did not receive failed status")
	}
	return nil
}

func TestAgentTurnFailedUsesTerminalRecoveryForDynamicPrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.messageCreator = &mockMessageCreator{}
	svc.beginPromptAttempt("s1", "execution-1", 7, true)

	svc.handleAgentTurnFailed(ctx, retainedTurnFailureData())

	waitForStopCall(t, agentMgr)
	agentMgr.mu.Lock()
	require.Len(t, agentMgr.stopAgentWithReasonArgs, 1)
	agentMgr.mu.Unlock()
}

func retainedTurnFailureData() watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", AgentID: "codex-acp",
		OwnerKind: string(lifecycle.ExecutionOwnerTask), AgentProfileID: "profile-codex", TurnID: "turn-1", PromptGeneration: 7,
		ErrorMessage:             "Selected model is at capacity. Please try a different model.",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		ProviderError: &streams.ProviderError{
			Source: streams.ProviderErrorSourceCodexACP, ProviderID: "codex-acp",
			ErrorKind: "model_capacity", Message: "Selected model is at capacity. Please try a different model.",
		},
		EvidenceKnown: true, OutputObserved: true, EffectObserved: true,
	}
}
