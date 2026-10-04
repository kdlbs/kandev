package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func dispatchContinuationFixture(t *testing.T) (*Service, *mockAgentManager, *transientRetryEntry) {
	t.Helper()
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.Len(t, mgr.capturedPrompts, 1)
	return svc, mgr, entry
}

func installContinuationRestoreFixture(t *testing.T, svc *Service) *mockAgentManager {
	t.Helper()
	mgr := svc.agentManager.(*mockAgentManager)
	if svc.turnService == nil {
		repo := svc.repo.(*sqliterepo.Repository)
		svc.turnService = taskservice.NewService(taskservice.Repos{
			Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo,
			GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo,
		}, &recordingEventBus{}, testLogger(), taskservice.RepositoryDiscoveryConfig{})
	}
	mgr.isAgentRunning = true
	mgr.currentPromptExecutionID = "replacement-1"
	mgr.currentPromptGeneration.Store(7)
	mgr.currentPromptActivityEpoch.Store(1)
	mgr.advancePromptGenerationOnAdmission = true
	mgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	mgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		now := time.Now().UTC()
		if err := svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{ID: "replacement-runtime", TaskID: "t1", SessionID: "s1", AgentExecutionID: "replacement-1", ResumeToken: "provider-session", Resumable: true, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Errorf("persist restore fixture: %v", err)
			return nil, err
		}
		_, _, err := svc.repo.UpdateTaskSessionStateIfCurrent(context.Background(), req.SessionID, models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput, "")
		if err != nil {
			t.Errorf("park restore fixture: %v", err)
			return nil, err
		}
		return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-1"}, nil
	}
	return mgr
}

// @covers AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
func TestInterruptionContinuationOwnershipCancelAcceptedTurn(t *testing.T) {
	svc, mgr, _ := dispatchContinuationFixture(t)
	require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	require.Equal(t, int32(1), mgr.cancelAgentCalls.Load(), "Cancel must stop the accepted continuation at the provider")
}

type continuationCancellationOrderMessageCreator struct {
	*mockMessageCreator
	cancellationPending              func() bool
	pendingAtCancellationNoticeWrite bool
}

func (m *continuationCancellationOrderMessageCreator) CreateSessionMessageIdempotent(
	ctx context.Context,
	messageID, taskID, content, sessionID, messageType, turnID string,
	metadata map[string]interface{},
	requestsInput bool,
) error {
	if metadata["recovery_disposition"] == "cancelled" {
		m.pendingAtCancellationNoticeWrite = m.cancellationPending()
	}
	return m.mockMessageCreator.CreateSessionMessageIdempotent(
		ctx, messageID, taskID, content, sessionID, messageType, turnID, metadata, requestsInput,
	)
}

func TestInterruptionContinuationCancellationSettlesBeforeReleasingOwnership(t *testing.T) {
	svc, _, _ := dispatchContinuationFixture(t)
	creator := &continuationCancellationOrderMessageCreator{
		mockMessageCreator: svc.messageCreator.(*mockMessageCreator),
		cancellationPending: func() bool {
			return svc.currentCancellation("s1") != nil
		},
	}
	svc.messageCreator = creator

	require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	latest := creator.sessionMessages[len(creator.sessionMessages)-1]
	require.Equal(t, "cancelled", latest.metadata["recovery_disposition"])
	require.True(t, creator.pendingAtCancellationNoticeWrite,
		"continuation recovery must be persisted before cancellation releases session ownership")
	session, err := svc.repo.GetTaskSession(context.Background(), "s1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
}

func TestInterruptionContinuationOwnershipOrdinaryStopRetiresEpisode(t *testing.T) {
	svc, mgr, _ := dispatchContinuationFixture(t)
	require.NoError(t, svc.CancelAgent(context.Background(), "s1"))
	require.Equal(t, int32(1), mgr.cancelAgentCalls.Load())
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "an ordinary Stop must retire automatic continuation")
}

func TestInterruptionContinuationOwnershipAcceptedContextStaysLive(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	var acceptedCtx context.Context
	mgr.promptAgentFunc = func(ctx context.Context, _ string, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		acceptedCtx = ctx
		return &executor.PromptResult{}, nil
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.NotNil(t, acceptedCtx)
	require.NoError(t, acceptedCtx.Err(), "successful dispatch transfers lifetime to the episode, rather than cancelling the accepted turn")
	svc.resetTransientRetry("s1")
	require.ErrorIs(t, acceptedCtx.Err(), context.Canceled)
}

func TestInterruptionContinuationShutdownPreservesAcceptedTurn(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	var acceptedCtx context.Context
	mgr.promptAgentFunc = func(ctx context.Context, _ string, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		acceptedCtx = ctx
		return &executor.PromptResult{}, nil
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	require.NotNil(t, acceptedCtx)
	svc.cancelAllTransientRetries()
	require.NoError(t, acceptedCtx.Err(), "shutdown abandons retry scheduling without cancelling work eligible for live adoption")
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned)
	entry.cancel()
}

func TestInterruptionContinuationOwnershipFinishesStartupAtAcceptance(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	entered := make(chan context.Context, 1)
	mgr.promptAgentFunc = func(ctx context.Context, _ string, _ string, _ []v1.MessageAttachment, dispatchOnly bool) (*executor.PromptResult, error) {
		entered <- ctx
		if !dispatchOnly {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &executor.PromptResult{}, nil
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	done := make(chan struct{})
	go func() { svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1"); close(done) }()
	t.Cleanup(func() { entry.cancel(); <-done })
	var acceptedCtx context.Context
	select {
	case acceptedCtx = <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("continuation did not reach acceptance")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("startup must finish at acceptance rather than own the entire provider turn")
	}
	svc.cancelResumeAttempts()
	require.NoError(t, acceptedCtx.Err(), "startup shutdown cannot cancel an accepted turn eligible for adoption")
}

func TestInterruptionContinuationOwnershipHumanPromptSupersedesEpisode(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := svc.agentManager.(*mockAgentManager)
	mgr.isAgentRunning = true
	mgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	now := time.Now().UTC()
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{ID: "human-runtime", TaskID: "t1", SessionID: "s1", AgentExecutionID: "execution-1", ResumeToken: "provider-session", Resumable: true, CreatedAt: now, UpdatedAt: now}))
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	_, err := svc.PromptTask(context.Background(), "t1", "s1", "new human request", "", false, nil, false)
	require.NoError(t, err)
	require.Equal(t, "new human request", mgr.capturedPrompts[len(mgr.capturedPrompts)-1])
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned, "a human prompt must retire the old continuation episode")
	svc.finishContinuationManual(context.Background(), "t1", "s1", "execution-1", entry)
	require.Zero(t, mgr.cancelAgentCalls.Load(), "late cleanup must leave the human successor alone")
}

// Contract coverage: cancellation must reach the existing native-restore owner.
func TestInterruptionContinuationOwnershipCancelDuringRestore(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	entered := make(chan struct{})
	mgr.launchAgentFunc = func(ctx context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, _ := svc.transientRetries.Load("s1")
	entry := value.(*transientRetryEntry)
	require.True(t, entry.claim())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("native restore did not start")
	}
	require.True(t, svc.CancelTransientRetry(context.Background(), "t1", "s1"))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not release the native restore owner")
	}
	require.Empty(t, mgr.capturedPrompts)
	_, owned := svc.transientRetries.Load("s1")
	require.False(t, owned)
}
