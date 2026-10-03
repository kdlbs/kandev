package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestTransientReplayUsesRetainedRuntime(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := configureRetainedReplayRuntime(t, svc)
	armTransientPromptEvidence(svc)
	svc.rememberTurnPrompt("s1", "retry the request", "", false, nil)

	data := retainedTurnFailureDataForReplay()
	svc.handleAgentTurnFailed(context.Background(), data)

	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok, "an eligible retained failure should use the existing retry owner")
	entry := value.(*transientRetryEntry)
	require.Equal(t, recoveryModeReplay, entry.mode)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Equal(t, 1, entry.started, "an admitted replay counts as one actual attempt")

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Empty(t, mgr.stopAgentWithReasonArgs, "a live retained runtime must not be torn down before replay")
	require.Empty(t, mgr.stopAgentArgs, "a live retained runtime must not be stopped before replay")
	require.Len(t, mgr.capturedPromptCalls, 1)
	require.Equal(t, "execution-1", mgr.capturedPromptCalls[0].ExecutionID)
}

func TestRetainedRetryCancelAndExhaustionKeepRuntime(t *testing.T) {
	t.Run("idle cancellation", func(t *testing.T) {
		svc, mc := newTransientTestService(t)
		t.Cleanup(svc.cancelAllTransientRetries)
		mgr := configureRetainedReplayRuntime(t, svc)
		launches := 0
		mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches++
			return nil, errors.New("replay unexpectedly restored")
		}
		armTransientPromptEvidence(svc)
		svc.rememberTurnPrompt("s1", "retry the request", "", false, nil)

		svc.handleAgentTurnFailed(context.Background(), retainedTurnFailureDataForReplay())
		require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))

		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		require.Empty(t, mgr.stopAgentWithReasonArgs)
		require.Empty(t, mgr.stopAgentArgs)
		require.Empty(t, mgr.capturedPromptCalls)
		require.True(t, mgr.isAgentRunning, "cancelling idle automatic work must keep the process available")
		var retainedError bool
		for _, message := range mc.sessionMessages {
			if message.metadata["runtime_retained"] == true {
				retainedError = true
				require.Equal(t, "cancelled", message.metadata["recovery_disposition"])
				require.NotEqual(t, true, message.metadata["recovery_actions"])
			}
		}
		require.True(t, retainedError, "idle cancellation should keep a durable provider error without startup controls")
	})

	t.Run("budget exhausted before another dispatch", func(t *testing.T) {
		svc, mc := newTransientTestService(t)
		mgr := configureRetainedReplayRuntime(t, svc)
		armTransientPromptEvidence(svc)
		data := retainedTurnFailureDataForReplay()
		svc.transientRetries.Store("s1", &transientRetryEntry{
			attempt: transientMaxAttempts, cancel: func() {}, retainedRuntime: svc.retainedRuntimeRetryForFailure(context.Background(), data),
		})

		svc.handleAgentTurnFailed(context.Background(), data)

		_, retryOwned := svc.transientRetries.Load("s1")
		require.False(t, retryOwned)
		require.Len(t, mc.sessionMessages, 1)
		require.Equal(t, true, mc.sessionMessages[0].metadata["runtime_retained"])
		require.Equal(t, "refused", mc.sessionMessages[0].metadata["recovery_disposition"])
		require.Equal(t, 0, mc.sessionMessages[0].metadata["attempts_started"])
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		require.Empty(t, mgr.stopAgentWithReasonArgs)
		require.Empty(t, mgr.stopAgentArgs)
		require.True(t, mgr.isAgentRunning)
	})
}

func TestRetainedRuntimeStatusProbeErrorBlocksReplayAndContinuation(t *testing.T) {
	t.Run("replay", func(t *testing.T) {
		svc, _ := newTransientTestService(t)
		t.Cleanup(svc.cancelAllTransientRetries)
		mgr := configureRetainedReplayRuntime(t, svc)
		launches := 0
		mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches++
			return nil, errors.New("replay unexpectedly restored")
		}
		armTransientPromptEvidence(svc)
		svc.rememberTurnPrompt("s1", "retry the request", "", false, nil)
		probe := &retainedRuntimeProbeAgentManager{mockAgentManager: mgr, err: errors.New("status endpoint unavailable")}
		svc.agentManager = probe

		svc.handleAgentTurnFailed(context.Background(), retainedTurnFailureDataForReplay())
		value, ok := svc.transientRetries.Load("s1")
		require.True(t, ok)
		entry := value.(*transientRetryEntry)
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

		require.Equal(t, 1, probe.probes)
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		require.Empty(t, mgr.stopAgentWithReasonArgs, "an inconclusive status probe must not stop the retained execution")
		require.Empty(t, mgr.stopAgentArgs)
		require.Empty(t, mgr.capturedPromptCalls, "an inconclusive status probe must refuse dispatch")
		require.Zero(t, launches, "an inconclusive status probe must not restore the runtime")
	})

	t.Run("continuation", func(t *testing.T) {
		svc, _, data := continuationFailureFixture(t)
		t.Cleanup(svc.cancelAllTransientRetries)
		mgr := installContinuationRestoreFixture(t, svc)
		launches := 0
		mgr.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches++
			return nil, errors.New("continuation unexpectedly restored")
		}
		data.AgentProfileID = "profile-1"
		data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
		require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
			ID: "runtime-original", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", Status: "ready",
		}))
		require.True(t, svc.handleTransientFailure(context.Background(), data))
		value, ok := svc.transientRetries.Load("s1")
		require.True(t, ok)
		entry := value.(*transientRetryEntry)
		probe := &retainedRuntimeProbeAgentManager{mockAgentManager: mgr, err: errors.New("status endpoint unavailable")}
		svc.agentManager = probe
		require.True(t, entry.claim())
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

		require.Equal(t, 1, probe.probes)
		require.Zero(t, launches, "an inconclusive status probe must not restore the native session")
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		require.Empty(t, mgr.stopAgentWithReasonArgs)
		require.Empty(t, mgr.stopAgentArgs)
		require.Empty(t, mgr.capturedPromptCalls)
	})
}

func TestRetainedRuntimeConfirmedAbsenceUsesExistingReplayFallback(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	mgr := configureRetainedReplayRuntime(t, svc)
	armTransientPromptEvidence(svc)
	svc.rememberTurnPrompt("s1", "retry the request", "", false, nil)
	probe := &retainedRuntimeProbeAgentManager{mockAgentManager: mgr, running: false}
	svc.agentManager = probe

	svc.handleAgentTurnFailed(context.Background(), retainedTurnFailureDataForReplay())
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	require.Equal(t, 1, probe.probes)
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Len(t, mgr.stopAgentWithReasonArgs, 1,
		"a successful false status probe confirms runtime loss and keeps the existing teardown fallback")
}

func TestRetainedRetryFinalizerCannotRetireSuccessorOwner(t *testing.T) {
	for _, disposition := range []string{"cancelled", "refused"} {
		t.Run(disposition, func(t *testing.T) {
			svc, messageCreator := newTransientTestService(t)
			t.Cleanup(svc.cancelAllTransientRetries)
			blockingCreator := &blockingRetainedFailureMessageCreator{
				mockMessageCreator: messageCreator,
				entered:            make(chan struct{}),
				release:            make(chan struct{}),
			}
			svc.messageCreator = blockingCreator
			notices := &transientRetryMessageServiceStub{
				messages: []*models.Message{{
					ID:       "successor-notice",
					Metadata: map[string]interface{}{"retrying": true},
				}},
			}
			svc.SetTransientRetryMessageService(notices)

			oldFailure := retainedTurnFailureDataForReplay()
			oldEntry := &transientRetryEntry{
				cancel: func() {},
				retainedRuntime: &retainedRuntimeRetry{
					failure: oldFailure,
				},
			}
			svc.transientRetries.Store("s1", oldEntry)
			svc.lastTurnPrompt.Store("s1", capturedPrompt{text: "old prompt"})

			finalizerDone := make(chan struct{})
			go func() {
				svc.finishRetainedRetryWithoutDispatch(context.Background(), "t1", "s1", oldEntry, disposition)
				close(finalizerDone)
			}()
			select {
			case <-blockingCreator.entered:
			case <-time.After(time.Second):
				t.Fatal("old retained failure did not enter durable message persistence")
			}

			state, releaseState := svc.acquireTransientRetryNoticeState("s1")
			state.mu.Lock()
			successor := svc.reserveTransientRetryWithMetadataLocked(state, "s1", 2, func(entry *transientRetryEntry) {
				entry.retainedRuntime = &retainedRuntimeRetry{failure: retainedTurnFailureDataForReplay()}
			})
			require.NotNil(t, successor)
			svc.armTransientRetryEntryLocked("t1", "s1", "execution-successor", successor, time.Hour)
			svc.lastTurnPrompt.Store("s1", capturedPrompt{text: "successor prompt"})
			state.mu.Unlock()
			releaseState()

			close(blockingCreator.release)
			select {
			case <-finalizerDone:
			case <-time.After(time.Second):
				t.Fatal("old retained failure finalizer did not finish")
			}

			current, ok := svc.transientRetries.Load("s1")
			require.True(t, ok, "the successor retry entry must remain installed")
			require.Same(t, successor, current)
			require.True(t, successor.armed, "the successor timer must remain armed")
			require.NoError(t, successor.retryCtx.Err(), "the successor timer context must remain live")
			cached, ok := svc.lastTurnPrompt.Load("s1")
			require.True(t, ok, "the successor prompt must remain cached")
			require.Equal(t, "successor prompt", cached.(capturedPrompt).text)
			require.Empty(t, notices.deleted, "the old finalizer must not resolve a successor notice")
			require.Equal(t, 0, notices.listCalls, "a stale finalizer must not scan session-wide notices")
		})
	}
}

type blockingRetainedFailureMessageCreator struct {
	*mockMessageCreator
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (m *blockingRetainedFailureMessageCreator) CreateSessionMessageIdempotent(
	ctx context.Context,
	messageID, taskID, content, sessionID, messageType, turnID string,
	metadata map[string]interface{},
	requestsInput bool,
) error {
	m.once.Do(func() {
		close(m.entered)
		<-m.release
	})
	return m.mockMessageCreator.CreateSessionMessageIdempotent(
		ctx, messageID, taskID, content, sessionID, messageType, turnID, metadata, requestsInput,
	)
}

type retainedRuntimeProbeAgentManager struct {
	*mockAgentManager
	running bool
	err     error
	probes  int
}

func (m *retainedRuntimeProbeAgentManager) IsAgentRunningForSession(context.Context, string) bool {
	return m.running
}

func (m *retainedRuntimeProbeAgentManager) ProbeAgentRunningForSession(context.Context, string) (bool, error) {
	m.probes++
	return m.running, m.err
}

func configureRetainedReplayRuntime(t *testing.T, svc *Service) *mockAgentManager {
	t.Helper()
	mgr := svc.agentManager.(*mockAgentManager)
	mgr.isAgentRunning = true
	mgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	mgr.currentPromptExecutionID = "execution-1"
	mgr.currentPromptGeneration.Store(7)
	mgr.advancePromptGenerationOnAdmission = true
	seedExecutorRunning(t, svc.repo.(*sqliterepo.Repository), "s1", "t1", "execution-1")
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	session.AgentProfileID = "profile-codex"
	session.AgentProfileSnapshot = map[string]any{"model": "gpt-5-codex"}
	session.DownstreamACPSessionID = "provider-session"
	require.NoError(t, svc.repo.UpdateTaskSession(context.Background(), session))
	return mgr
}

func retainedTurnFailureDataForReplay() watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", AgentID: "codex-acp",
		OwnerKind: "task", AgentProfileID: "profile-codex", TurnID: "turn-1", PromptGeneration: 7,
		ErrorMessage:             "Selected model is at capacity. Please try a different model.",
		PromptFailureDisposition: streams.PromptFailureDispositionRetainRuntime,
		ProviderError: &streams.ProviderError{
			Source: streams.ProviderErrorSourceCodexACP, ProviderID: "codex-acp",
			ErrorKind: "model_capacity", Message: "Selected model is at capacity. Please try a different model.",
		},
		EvidenceKnown: true,
	}
}
