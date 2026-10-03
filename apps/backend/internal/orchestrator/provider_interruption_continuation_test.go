package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func continuationFailureFixture(t *testing.T) (*Service, *mockMessageCreator, watcher.AgentEventData) {
	t.Helper()
	svc, mc := newTransientTestService(t)
	t.Cleanup(func() {
		// Mock turns have no runtime lifetime owner to release accepted contexts.
		svc.transientRetries.Range(func(_, value any) bool {
			if entry, ok := value.(*transientRetryEntry); ok && entry.cancel != nil {
				entry.cancel()
			}
			return true
		})
		svc.cancelAllTransientRetries()
	})
	svc.config.ProviderInterruptionContinuation = true
	armTransientPromptEvidence(svc)
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	session.DownstreamACPSessionID = "provider-session"
	session.TaskEnvironmentID = "existing-workspace"
	require.NoError(t, svc.repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID: "existing-workspace", TaskID: "t1", ExecutorType: "local", Status: models.TaskEnvironmentStatusReady,
	}))
	session.AgentProfileID = "profile-1"
	session.AgentProfileSnapshot = map[string]any{"model": "mock-fast", "auto_approve": false}
	require.NoError(t, svc.repo.UpdateTaskSession(context.Background(), session))
	return svc, mc, watcher.AgentEventData{TaskID: "t1", SessionID: "s1", OwnerKind: "task", AgentID: "cursor-acp", AgentExecutionID: "execution-1", PromptGeneration: 7, ErrorMessage: cursorRetriableConnectionStalled, EvidenceKnown: true, OutputObserved: true, EffectObserved: true,
		ContinuationSafety: &streams.ContinuationSafetySnapshot{Support: streams.ContinuationNativeSavedHistoryV1, Known: true, PromptGeneration: 7}}
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
func TestInterruptionContinuationAdmission(t *testing.T) {
	svc, mc, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	require.Len(t, mc.sessionMessages, 1)
	require.Equal(t, "continue", mc.sessionMessages[0].metadata["recovery_mode"])
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
func TestInterruptionContinuationDispatchUsesNativeIdentity(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := svc.agentManager.(*mockAgentManager)
	launches := make(chan *executor.LaunchAgentRequest, 1)
	mgr.isAgentRunning = true
	mgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	mgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches <- req
		now := time.Now().UTC()
		require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{ID: "runtime-replacement", TaskID: "t1", SessionID: "s1", AgentExecutionID: "replacement-1", ResumeToken: "provider-session", Resumable: true, CreatedAt: now, UpdatedAt: now}))
		_, _, err := svc.repo.UpdateTaskSessionStateIfCurrent(context.Background(), req.SessionID, models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput, "")
		require.NoError(t, err)
		return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-1"}, nil
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	select {
	case launch := <-launches:
		require.Equal(t, "provider-session", launch.RequiredNativeConversationID)
		require.Equal(t, "provider-session", launch.ACPSessionID)
		require.Empty(t, launch.TaskDescription)
	default:
		t.Fatal("continuation never reached native restore")
	}
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Len(t, mgr.capturedPrompts, 1)
	require.Contains(t, mgr.capturedPrompts[0], "Continue the unfinished request")
	require.NotContains(t, mgr.capturedPrompts[0], "test original request")
}

func TestTransientReplayPromotesLateSafeActivityToNativeContinuation(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	data.OutputObserved = false
	data.EffectObserved = false
	manager := svc.agentManager.(*mockAgentManager)
	manager.currentPromptExecutionID = "execution-1"
	manager.currentPromptGeneration.Store(7)
	manager.isAgentRunning = true
	manager.isAgentReadyFn = func(context.Context, string) bool { return true }
	reader := &transientRetryEvidenceAgentManager{
		mockAgentManager: manager,
		executionID:      "execution-1",
		generation:       7,
		found:            true,
		evidence:         lifecycle.PromptAttemptEvidence{EvidenceKnown: true},
	}
	svc.agentManager = reader

	svc.handleAgentFailed(context.Background(), data)
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok, "output-free failure snapshot should initially reserve replay")
	entry := value.(*transientRetryEntry)
	mode, _ := entry.recovery()
	require.Equal(t, recoveryModeReplay, mode)

	reader.evidence.OutputObserved = true
	reader.evidence.EffectObserved = true
	launches := make(chan *executor.LaunchAgentRequest, 1)
	manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches <- req
		now := time.Now().UTC()
		require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
			ID: "runtime-replacement", TaskID: "t1", SessionID: "s1", AgentExecutionID: "replacement-1",
			ResumeToken: "provider-session", Resumable: true, CreatedAt: now, UpdatedAt: now,
		}))
		_, _, err := svc.repo.UpdateTaskSessionStateIfCurrent(
			context.Background(), "s1", models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput, "",
		)
		require.NoError(t, err)
		return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-1"}, nil
	}
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	select {
	case launch := <-launches:
		require.Equal(t, "provider-session", launch.RequiredNativeConversationID)
		require.Equal(t, "provider-session", launch.ACPSessionID)
	case <-time.After(time.Second):
		t.Fatal("late safe lifecycle evidence did not promote replay to native restore")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	require.Len(t, manager.capturedPrompts, 1)
	require.Contains(t, manager.capturedPrompts[0], "Continue the unfinished request")
}

func TestInterruptionContinuationOwnershipContextCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	registry := newResumeAttemptRegistry()
	attempt, _ := registry.begin(context.WithValue(parent, continuationOwnedContextKey{}, true), "t1", "s1")
	cancel()
	require.ErrorIs(t, attempt.context().Err(), context.Canceled, "episode cancellation must reach the resume owner")
}

func TestInterruptionContinuationDisabledAndUnsafe(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Service, *watcher.AgentEventData)
	}{
		{"disabled", func(s *Service, _ *watcher.AgentEventData) { s.config.ProviderInterruptionContinuation = false }},
		{"unknown saved profile", func(s *Service, _ *watcher.AgentEventData) {
			session, err := s.repo.GetTaskSession(context.Background(), "s1")
			require.NoError(t, err)
			session.AgentProfileSnapshot = nil
			require.NoError(t, s.repo.UpdateTaskSession(context.Background(), session))
		}},
		{"old remote", func(_ *Service, d *watcher.AgentEventData) { d.ContinuationSafety = nil }},
		{"missing native identity", func(s *Service, _ *watcher.AgentEventData) {
			session, err := s.repo.GetTaskSession(context.Background(), "s1")
			require.NoError(t, err)
			session.DownstreamACPSessionID = ""
			delete(session.Metadata, "acp")
			require.NoError(t, s.repo.UpdateTaskSession(context.Background(), session))
		}},
		{"unsafe", func(_ *Service, d *watcher.AgentEventData) { d.ContinuationSafety.Unsafe = true }},
		{"pending", func(_ *Service, d *watcher.AgentEventData) { d.ContinuationSafety.Pending = true }},
		{"stale", func(_ *Service, d *watcher.AgentEventData) { d.PromptGeneration = 6 }},
		{"Office", func(_ *Service, d *watcher.AgentEventData) { d.OwnerKind = "office" }},
		{"dynamic", func(_ *Service, d *watcher.AgentEventData) { d.DynamicRouteAttempt = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, data := continuationFailureFixture(t)
			tc.mutate(svc, &data)
			require.False(t, svc.handleTransientFailure(context.Background(), data))
			_, owned := svc.transientRetries.Load("s1")
			require.False(t, owned)
		})
	}
}

func TestInterruptionContinuationBudgetDoesNotRevertToReplay(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	data.OutputObserved = false
	data.EffectObserved = false
	data.ContinuationSafety = nil
	require.False(t, svc.handleTransientFailure(context.Background(), data), "missing continuation evidence cannot authorize original prompt replay")
}

func TestInterruptionContinuationSettlementClosesTurnAsInterrupted(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	turns := &repoTurnService{repo: svc.repo.(*sqliterepo.Repository)}
	svc.turnService = turns
	turn, err := turns.StartTurn(context.Background(), "s1")
	require.NoError(t, err)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	settled, err := turns.GetTurn(context.Background(), turn.ID)
	require.NoError(t, err)
	require.NotNil(t, settled.CompletedAt)
	require.Equal(t, settled.StartedAt, *settled.CompletedAt)
	require.Equal(t, true, settled.Metadata["interrupted"])
}

func TestInterruptionContinuationManualAfterPreparationFailure(t *testing.T) {
	svc, mc, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	svc.finishContinuationManual(entry.retryCtx, "t1", "s1", "execution-1", entry)
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Len(t, mc.sessionMessages, 2)
}

func TestInterruptionContinuationOwnershipConfigurationChange(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	require.NoError(t, svc.repo.SetSessionMetadataKey(context.Background(), "s1", models.SessionMetaKeySessionMode, "acceptEdits"))
	require.Error(t, svc.validateContinuationOwner(context.Background(), "t1", "s1", value.(*transientRetryEntry)))
}
