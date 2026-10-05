package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCapacityContinuationAfterCompletedToolsUsesSameRuntime(t *testing.T) {
	svc, mgr, data := capacityContinuationFailureFixture(t)
	defer svc.cancelAllTransientRetries()
	mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return nil, errors.New("capacity continuation unexpectedly relaunched the runtime")
	}
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))

	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.Equal(t, recoveryModeContinue, entry.mode)
	require.NotNil(t, entry.continuation)
	require.NotNil(t, entry.retainedRuntime)
	require.Equal(t, 0, entry.started)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
	require.Len(t, mgr.capturedPromptCalls, 1)
	require.Equal(t, "execution-1", mgr.capturedPromptCalls[0].ExecutionID)
	require.True(t, mgr.capturedPromptCalls[0].DispatchOnly)
	require.Contains(t, mgr.capturedPrompts[0], "selected model was at capacity")
	require.Contains(t, mgr.capturedPrompts[0], "Preserve all completed actions")
	require.NotContains(t, mgr.capturedPrompts[0], "test original request")
	require.Equal(t, 1, entry.started)
}

func capacityContinuationFailureFixture(t *testing.T) (*Service, *mockAgentManager, watcher.AgentEventData) {
	t.Helper()
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	svc.config.ProviderInterruptionContinuation = false
	data.AgentID = "codex-acp"
	data.AgentProfileID = "profile-1"
	data.ErrorMessage = "Selected model is at capacity. Please try a different model."
	data.ProviderError = &streams.ProviderError{
		Source: streams.ProviderErrorSourceCodexACP, ProviderID: "codex-acp",
		ErrorKind: "model_capacity", Message: "Selected model is at capacity. Please try a different model.",
	}
	data.ContinuationSafety = nil
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	data.CapacityContinuation = &streams.CapacityContinuationSnapshot{
		Support: streams.CapacityContinuationCodexLiveSessionV1, PromptGeneration: data.PromptGeneration,
		EvidenceComplete: true, CompletedTools: 2,
	}
	return svc, mgr, data
}

func startCapacityContinuationEpisode(t *testing.T) (*Service, *mockAgentManager, *mockMessageCreator, watcher.AgentEventData, *transientRetryEntry) {
	t.Helper()
	svc, mgr, data := capacityContinuationFailureFixture(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
	}))
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	return svc, mgr, svc.messageCreator.(*mockMessageCreator), data, entry
}

func TestCapacityContinuationBudgetSurvivesProgress(t *testing.T) {
	svc, mgr, _, data, entry := startCapacityContinuationEpisode(t)
	for attempt := 1; attempt <= transientMaxAttempts; attempt++ {
		require.Equal(t, attempt, entry.attempt)
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		require.Equal(t, attempt, entry.started)

		if attempt == transientMaxAttempts {
			break
		}
		generation := mgr.currentPromptGeneration.Load()
		data.PromptGeneration = generation
		data.CapacityContinuation.PromptGeneration = generation
		data.CapacityContinuation.CompletedTools = uint16(attempt)
		svc.beginPromptAttempt("s1", "execution-1", generation, false)
		require.True(t, svc.handleTransientFailure(context.Background(), data))
		value, ok := svc.transientRetries.Load("s1")
		require.True(t, ok)
		entry = value.(*transientRetryEntry)
		t.Cleanup(entry.cancel)
		require.Equal(t, attempt+1, entry.attempt)
		require.Equal(t, attempt, entry.started, "accepted progress must keep earlier actual attempts")
	}
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Len(t, mgr.capturedPromptCalls, transientMaxAttempts)
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
}

func TestCapacityContinuationSuccessResetsEpisode(t *testing.T) {
	svc, mgr, _, data, entry := startCapacityContinuationEpisode(t)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Equal(t, 1, entry.started)
	svc.resetTransientRetry("s1")
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned)
	require.ErrorIs(t, entry.retryCtx.Err(), context.Canceled)

	generation := mgr.currentPromptGeneration.Load()
	data.PromptGeneration = generation
	data.CapacityContinuation.PromptGeneration = generation
	svc.beginPromptAttempt("s1", "execution-1", generation, false)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	next := value.(*transientRetryEntry)
	t.Cleanup(next.cancel)
	require.Equal(t, 1, next.attempt)
	require.Zero(t, next.started)
}

func TestCapacityContinuationSupersession(t *testing.T) {
	t.Run("queued work", func(t *testing.T) {
		svc, mgr, _, _, entry := startCapacityContinuationEpisode(t)
		_, err := svc.messageQueue.QueueMessage(context.Background(), "s1", "t1", "human queued prompt", "", "user", false, nil)
		require.NoError(t, err)
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		assertCapacityContinuationRefusedWithoutRuntimeMutation(t, svc, mgr)
	})

	t.Run("archive", func(t *testing.T) {
		svc, mgr, _, _, entry := startCapacityContinuationEpisode(t)
		archiver, ok := svc.repo.(interface {
			ArchiveTask(context.Context, string) error
		})
		require.True(t, ok)
		require.NoError(t, archiver.ArchiveTask(context.Background(), "t1"))
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		assertCapacityContinuationRefusedWithoutRuntimeMutation(t, svc, mgr)
	})

	t.Run("configuration change", func(t *testing.T) {
		svc, mgr, _, _, entry := startCapacityContinuationEpisode(t)
		require.NoError(t, svc.repo.SetSessionMetadataKey(context.Background(), "s1", models.SessionMetaKeySessionMode, "acceptEdits"))
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		assertCapacityContinuationRefusedWithoutRuntimeMutation(t, svc, mgr)
	})

	t.Run("automation ownership change", func(t *testing.T) {
		svc, mgr, messages, _, entry := startCapacityContinuationEpisode(t)
		task, err := svc.repo.GetTask(context.Background(), "t1")
		require.NoError(t, err)
		task.Origin = models.TaskOriginAutomationRun
		require.NoError(t, svc.repo.UpdateTask(context.Background(), task))
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

		require.Zero(t, entry.started)
		assertCapacityContinuationRefusedWithoutRuntimeMutation(t, svc, mgr)
		require.NotEmpty(t, messages.sessionMessages)
		last := messages.sessionMessages[len(messages.sessionMessages)-1]
		require.Equal(t, "refused", last.metadata["recovery_disposition"])
		require.Zero(t, last.metadata["attempts_started"])
	})

	t.Run("new human prompt", func(t *testing.T) {
		svc, mgr, _, _, entry := startCapacityContinuationEpisode(t)
		mgr.isAgentRunning = true
		mgr.isAgentReadyFn = func(context.Context, string) bool { return true }
		_, err := svc.PromptTask(context.Background(), "t1", "s1", "new human request", "", false, nil, false)
		require.NoError(t, err)
		_, owned := svc.transientRetries.Load("s1")
		require.False(t, owned)
		entry.cancel()
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		require.Len(t, mgr.capturedPrompts, 1)
		require.Equal(t, "new human request", mgr.capturedPrompts[0])
		require.Empty(t, mgr.stopAgentWithReasonArgs)
	})

	t.Run("reset", func(t *testing.T) {
		svc, _, _, _, entry := startCapacityContinuationEpisode(t)
		svc.resetTransientRetry("s1")
		_, owned := svc.transientRetries.Load("s1")
		require.False(t, owned)
		require.ErrorIs(t, entry.retryCtx.Err(), context.Canceled)
	})
}

func TestCapacityContinuationRuntimeLossRefusesRestore(t *testing.T) {
	for _, test := range []struct {
		name  string
		probe bool
	}{
		{name: "lost runtime"},
		{name: "inconclusive probe", probe: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, mgr, messages, _, entry := startCapacityContinuationEpisode(t)
			require.Equal(t, continuationPolicyCapacityLive, entry.continuationPolicy)
			launches := 0
			mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launches++
				return nil, errors.New("capacity continuation must not restore a runtime")
			}
			if test.probe {
				svc.agentManager = &retainedRuntimeProbeAgentManager{
					mockAgentManager: mgr, running: true, err: errors.New("runtime probe unavailable"),
				}
			} else {
				mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
					return "", agentruntime.ErrNoExecutionForSession
				}
			}
			require.True(t, entry.claim())
			svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
			require.Zero(t, entry.started)
			require.NotEmpty(t, messages.sessionMessages)
			last := messages.sessionMessages[len(messages.sessionMessages)-1]
			require.Equal(t, "refused", last.metadata["recovery_disposition"])
			require.Zero(t, last.metadata["attempts_started"])
			require.Zero(t, launches)
			mgr.mu.Lock()
			defer mgr.mu.Unlock()
			require.Empty(t, mgr.capturedPrompts)
			require.Empty(t, mgr.stopAgentWithReasonArgs)
			require.Empty(t, mgr.stopAgentArgs)
		})
	}
}

func TestCapacityContinuationRejectsRuntimeChangeBeforeResumePreparation(t *testing.T) {
	for _, test := range []struct {
		name string
		lost bool
	}{
		{name: "runtime removed", lost: true},
		{name: "runtime becomes unready"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, mgr, messages, _, entry := startCapacityContinuationEpisode(t)
			require.Equal(t, continuationPolicyCapacityLive, entry.continuationPolicy)
			var launches int
			mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launches++
				return nil, errors.New("capacity continuation must not restore a runtime")
			}

			entered := make(chan struct{})
			release := make(chan struct{})
			readinessChecks := 0
			mgr.isAgentReadyFn = func(context.Context, string) bool {
				readinessChecks++
				switch readinessChecks {
				case 1, 2:
					return true
				case 3:
					close(entered)
					<-release
					return false
				default:
					return test.lost
				}
			}

			oldReadyTimeout, oldReadyInterval := agentPromptReadyTimeout, agentPromptReadyInterval
			agentPromptReadyTimeout, agentPromptReadyInterval = 15*time.Millisecond, time.Millisecond
			t.Cleanup(func() {
				agentPromptReadyTimeout, agentPromptReadyInterval = oldReadyTimeout, oldReadyInterval
			})

			require.True(t, entry.claim())
			done := make(chan struct{})
			go func() {
				svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
				close(done)
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("continuation did not reach prompt resume preparation")
			}

			if test.lost {
				mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
					return "", agentruntime.ErrNoExecutionForSession
				}
				mgr.isAgentRunning = false
				require.NoError(t, svc.repo.DeleteExecutorRunningBySessionID(context.Background(), "s1"))
				require.NoError(t, svc.repo.UpdateTaskSessionState(context.Background(), "s1", models.TaskSessionStateWaitingForInput, ""))
			}
			close(release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("continuation did not retire after the runtime changed")
			}

			require.Zero(t, entry.started)
			_, owned := svc.transientRetries.Load("s1")
			require.False(t, owned, "refused runtime admission must retire retry ownership")
			require.NotEmpty(t, messages.sessionMessages)
			last := messages.sessionMessages[len(messages.sessionMessages)-1]
			require.Equal(t, "refused", last.metadata["recovery_disposition"])
			require.Zero(t, last.metadata["attempts_started"])
			require.Zero(t, launches)
			mgr.mu.Lock()
			require.Empty(t, mgr.capturedPrompts)
			require.Empty(t, mgr.stopAgentWithReasonArgs)
			require.Empty(t, mgr.stopAgentArgs)
			require.Empty(t, mgr.startAgentProcessCalls)
			require.Empty(t, mgr.restartProcessCalls)
			mgr.mu.Unlock()
			if test.lost {
				_, err := svc.repo.GetExecutorRunningBySessionID(context.Background(), "s1")
				require.ErrorIs(t, err, models.ErrExecutorRunningNotFound,
					"refusal must not restore the removed executor row")
			} else {
				running, err := svc.repo.GetExecutorRunningBySessionID(context.Background(), "s1")
				require.NoError(t, err)
				require.Equal(t, "execution-1", running.AgentExecutionID,
					"readiness refusal must not replace the retained executor identity")
			}
		})
	}
}

func TestCapacityContinuationFencesExecutionIdentityAtFinalAdmission(t *testing.T) {
	for _, test := range []struct {
		name           string
		readinessCheck int
		automation     bool
	}{
		{name: "before final dispatch admission", readinessCheck: 5},
		{name: "at provider admission", readinessCheck: 6},
		{name: "automation ownership changes at provider admission", readinessCheck: 6, automation: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, mgr, messages, _, entry := startCapacityContinuationEpisode(t)
			var launches int
			mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launches++
				return nil, errors.New("capacity continuation must not restore a runtime")
			}

			entered := make(chan struct{})
			release := make(chan struct{})
			readinessChecks := 0
			mgr.isAgentReadyFn = func(context.Context, string) bool {
				readinessChecks++
				if readinessChecks == test.readinessCheck {
					close(entered)
					<-release
				}
				return true
			}

			require.True(t, entry.claim())
			done := make(chan struct{})
			go func() {
				svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
				close(done)
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("continuation did not reach final live-execution admission")
			}

			if test.automation {
				task, err := svc.repo.GetTask(context.Background(), "t1")
				require.NoError(t, err)
				task.Origin = models.TaskOriginAutomationRun
				require.NoError(t, svc.repo.UpdateTask(context.Background(), task))
				updatedTask, err := svc.repo.GetTask(context.Background(), "t1")
				require.NoError(t, err)
				require.Equal(t, models.TaskOriginAutomationRun, updatedTask.Origin)
			} else {
				mgr.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
					return "execution-successor", nil
				}
				require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
					ID: "runtime-successor", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-successor", Status: "ready",
				}))
			}
			close(release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("continuation did not retire after execution identity changed")
			}

			require.Zero(t, entry.started, "attempt count advances only after the retained identity passes final admission")
			_, owned := svc.transientRetries.Load("s1")
			require.False(t, owned)
			require.NotEmpty(t, messages.sessionMessages)
			last := messages.sessionMessages[len(messages.sessionMessages)-1]
			require.Equal(t, "refused", last.metadata["recovery_disposition"])
			require.Zero(t, last.metadata["attempts_started"])
			require.Zero(t, launches)
			mgr.mu.Lock()
			require.Empty(t, mgr.capturedPrompts)
			require.Empty(t, mgr.stopAgentWithReasonArgs)
			require.Empty(t, mgr.stopAgentArgs)
			require.Empty(t, mgr.startAgentProcessCalls)
			require.Empty(t, mgr.restartProcessCalls)
			mgr.mu.Unlock()
			running, err := svc.repo.GetExecutorRunningBySessionID(context.Background(), "s1")
			require.NoError(t, err)
			wantExecution := "execution-successor"
			if test.automation {
				wantExecution = "execution-1"
			}
			require.Equal(t, wantExecution, running.AgentExecutionID,
				"refusal leaves the independently admitted execution untouched")
		})
	}
}

func TestCapacityContinuationFinalDisposition(t *testing.T) {
	t.Run("uncertain completed work is refused", func(t *testing.T) {
		svc, _, data := capacityContinuationFailureFixture(t)
		data.CapacityContinuation.PendingTools = true
		require.False(t, svc.handleTransientFailure(context.Background(), data))
		_, owned := svc.transientRetries.Load("s1")
		require.False(t, owned)
		require.Equal(t, "unsafe_work", svc.continuationRefusalReason(context.Background(), data))
	})

	t.Run("unresolved background work is refused", func(t *testing.T) {
		svc, _, data := capacityContinuationFailureFixture(t)
		data.CapacityContinuation.UnaccountedBackground = true
		require.False(t, data.CapacityContinuation.SafeFor(data.PromptGeneration))
		require.False(t, svc.handleTransientFailure(context.Background(), data))
		_, owned := svc.transientRetries.Load("s1")
		require.False(t, owned)
		require.Equal(t, "unsafe_work", svc.continuationRefusalReason(context.Background(), data))
	})

	t.Run("cancellation persists zero actual attempts", func(t *testing.T) {
		svc, _, messages, _, _ := startCapacityContinuationEpisode(t)
		require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
		require.NotEmpty(t, messages.sessionMessages)
		last := messages.sessionMessages[len(messages.sessionMessages)-1]
		require.Equal(t, "cancelled", last.metadata["recovery_disposition"])
		require.Zero(t, last.metadata["attempts_started"])
	})

	t.Run("five accepted dispatches exhaust without a sixth", func(t *testing.T) {
		svc, mgr, messages, data, entry := startCapacityContinuationEpisode(t)
		for attempt := 1; attempt <= transientMaxAttempts; attempt++ {
			require.True(t, entry.claim())
			svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
			require.Equal(t, attempt, entry.started)
			if attempt < transientMaxAttempts {
				generation := mgr.currentPromptGeneration.Load()
				data.PromptGeneration = generation
				data.CapacityContinuation.PromptGeneration = generation
				svc.beginPromptAttempt("s1", "execution-1", generation, false)
				require.True(t, svc.handleTransientFailure(context.Background(), data))
				value, ok := svc.transientRetries.Load("s1")
				require.True(t, ok)
				entry = value.(*transientRetryEntry)
				t.Cleanup(entry.cancel)
			}
		}
		entry.mu.Lock()
		entry.attempt = transientMaxAttempts
		entry.mu.Unlock()
		svc.finishRetainedRetryWithoutDispatch(context.Background(), "t1", "s1", entry, recoveryDispositionExhausted)
		_, owned := svc.transientRetries.Load("s1")
		require.False(t, owned)
		require.NotEmpty(t, messages.sessionMessages)
		last := messages.sessionMessages[len(messages.sessionMessages)-1]
		require.Equal(t, "exhausted", last.metadata["recovery_disposition"])
		require.Equal(t, transientMaxAttempts, last.metadata["attempts_started"])
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		require.Len(t, mgr.capturedPromptCalls, transientMaxAttempts)
	})
}

func assertCapacityContinuationRefusedWithoutRuntimeMutation(t *testing.T, svc *Service, mgr *mockAgentManager) {
	t.Helper()
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned)
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.capturedPrompts)
	require.Empty(t, mgr.stopAgentWithReasonArgs)
	require.Empty(t, mgr.stopAgentArgs)
}
