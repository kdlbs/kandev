package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type quickChatLaunchReloadFailureStore struct {
	sessionExecutorStore
	reads  int
	failAt int
}

func (r *quickChatLaunchReloadFailureStore) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.reads++
	// Passthrough resolution precedes the mandatory launch reload (no event bus is installed).
	if r.reads == r.failAt {
		return nil, errors.New("session reload unavailable")
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, id)
}

func TestQuickChatLaunchPersistsPostAllocationFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failAt    int
		wantError string
		launches  int
	}{
		{"session reload", 2, "failed to reload launch session", 0},
		{"runtime launch", 0, "runtime launch unavailable", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			now := time.Now().UTC()
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Workspace", CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Workflow", CreatedAt: now, UpdatedAt: now}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Chat", State: v1.TaskStateCreated, IsEphemeral: true, CreatedAt: now, UpdatedAt: now}))
			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Chat", State: v1.TaskStateCreated, IsEphemeral: true}
			launches := 0
			manager := &mockAgentManager{launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				launches++
				return nil, errors.New("runtime launch unavailable")
			}}
			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
			svc.repo = &quickChatLaunchReloadFailureStore{sessionExecutorStore: svc.repo, failAt: tc.failAt}
			_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{TaskID: "task1", Intent: IntentStart, AgentProfileID: "profile1", ProfileExplicit: true})
			require.ErrorContains(t, err, tc.wantError)
			require.Equal(t, tc.launches, launches)
			sessions, err := repo.ListTaskSessions(ctx, "task1")
			require.NoError(t, err)
			require.Len(t, sessions, 1)
			require.Equal(t, models.TaskSessionStateFailed, sessions[0].State)
			require.Contains(t, sessions[0].ErrorMessage, "unavailable")
			failure, ok := models.LoadLastAgentError(sessions[0].Metadata)
			require.True(t, ok)
			require.Equal(t, models.ErrorScopeSession, failure.Scope)
			require.Equal(t, models.LaunchErrorPhaseBootstrap, failure.Phase)
			require.NotEmpty(t, failure.Stamp())
			require.Contains(t, failure.RecoveryActions, models.RecoveryActionRetryLaunch)
		})
	}
}

type quickChatFailingMCPSelectionWriter struct{ err error }

func (w quickChatFailingMCPSelectionWriter) Replace(
	context.Context,
	mcpconfig.SelectionScope,
	string,
	string,
	[]string,
) error {
	return w.err
}

func TestQuickChatPreparedStartPersistsEarlyFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Workspace", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Workflow", CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Chat", State: v1.TaskStateCreated, IsEphemeral: true, CreatedAt: now, UpdatedAt: now}))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", WorkspaceID: "ws1", Title: "Chat", State: v1.TaskStateCreated, IsEphemeral: true}
	manager := &mockAgentManager{}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)

	prepared, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:         "task1",
		Intent:         IntentPrepare,
		AgentProfileID: "profile1",
		ExecutorID:     models.ExecutorIDLocal,
		DeferredStart:  true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, prepared.SessionID)

	selectionErr := errors.New("selection persistence unavailable")
	svc.SetMCPSelectionWriter(quickChatFailingMCPSelectionWriter{err: selectionErr})
	_, err = svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:                "task1",
		Intent:                IntentStartCreated,
		SessionID:             prepared.SessionID,
		AgentProfileID:        "profile1",
		NoInitialPrompt:       true,
		MCPServerIDs:          []string{},
		QuickChatInitialStart: true,
	})
	require.ErrorIs(t, err, selectionErr)

	sessions, err := repo.ListTaskSessions(ctx, "task1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, models.TaskSessionStateFailed, sessions[0].State)
	failure, ok := models.LoadLastAgentError(sessions[0].Metadata)
	require.True(t, ok)
	require.Equal(t, models.ErrorScopeSession, failure.Scope)
	require.Equal(t, models.LaunchErrorPhaseBootstrap, failure.Phase)
	require.Contains(t, failure.RecoveryActions, models.RecoveryActionRetryLaunch)
}

func TestRecordEarlyLaunchFailureDoesNotFailRunningSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-running", "session-running", models.TaskSessionStateRunning)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})

	launchErr := errors.New("stale startup failure")
	err := svc.executor.RecordEarlyLaunchFailure(ctx, "task-running", "session-running", launchErr)
	require.ErrorIs(t, err, launchErr)

	session, err := repo.GetTaskSession(ctx, "session-running")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	_, hasFailure := models.LoadLastAgentError(session.Metadata)
	require.False(t, hasFailure, "a stale failure must not overwrite a live session")
}

func TestQuickChatStartFailureIgnoresCompetingStartingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-quick-chat-competing", "session-quick-chat-competing", models.TaskSessionStateStarting)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-quick-chat-competing"] = &v1.Task{ID: "task-quick-chat-competing", IsEphemeral: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	entry := &transientRetryEntry{attempt: 1}
	svc.transientRetries.Store("session-quick-chat-competing", entry)
	svc.lastTurnPrompt.Store("session-quick-chat-competing", capturedPrompt{text: "keep retry prompt"})

	_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:                "task-quick-chat-competing",
		SessionID:             "session-quick-chat-competing",
		Intent:                IntentStartCreated,
		QuickChatInitialStart: true,
	})
	require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning)

	session, err := repo.GetTaskSession(ctx, "session-quick-chat-competing")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, session.State)
	_, hasFailure := models.LoadLastAgentError(session.Metadata)
	require.False(t, hasFailure)
	storedEntry, retryStillOwned := svc.transientRetries.Load("session-quick-chat-competing")
	require.True(t, retryStillOwned, "a competing startup must preserve the current retry owner")
	require.Same(t, entry, storedEntry)
	prompt, promptRetained := svc.lastTurnPrompt.Load("session-quick-chat-competing")
	require.True(t, promptRetained, "a competing startup must preserve the current retry prompt")
	require.Equal(t, "keep retry prompt", prompt.(capturedPrompt).text)
}

func TestQuickChatStartFailureDoesNotOverwriteReplacementWaitingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-quick-chat-replaced", "session-quick-chat-replaced", models.TaskSessionStateWaitingForInput)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-quick-chat-replaced", models.SessionMetaKeyQuickChatStartAttemptID, "replacement-attempt"))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-quick-chat-replaced", models.SessionMetaKeyAgentStartAttemptID, "replacement-execution-attempt"))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-quick-chat-replaced"] = &v1.Task{ID: "task-quick-chat-replaced", IsEphemeral: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	launchErr := errors.New("stale startup failure")
	entry := &transientRetryEntry{attempt: 1}
	svc.transientRetries.Store("session-quick-chat-replaced", entry)
	svc.lastTurnPrompt.Store("session-quick-chat-replaced", capturedPrompt{text: "keep replacement prompt"})

	err := svc.handleQuickChatStartCreatedFailure(
		ctx,
		"task-quick-chat-replaced",
		"session-quick-chat-replaced",
		"original-attempt",
		"original-execution-attempt",
		launchErr,
	)
	require.ErrorIs(t, err, launchErr)

	session, err := repo.GetTaskSession(ctx, "session-quick-chat-replaced")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Equal(t, "replacement-attempt", models.StringFromAny(session.Metadata[models.SessionMetaKeyQuickChatStartAttemptID]))
	require.Equal(t, "replacement-execution-attempt", models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]))
	_, hasFailure := models.LoadLastAgentError(session.Metadata)
	require.False(t, hasFailure)
	storedEntry, retryStillOwned := svc.transientRetries.Load("session-quick-chat-replaced")
	require.True(t, retryStillOwned, "stale failures must not clean up replacement retry state")
	require.Same(t, entry, storedEntry)
	prompt, promptRetained := svc.lastTurnPrompt.Load("session-quick-chat-replaced")
	require.True(t, promptRetained, "stale failures must not clear the replacement prompt")
	require.Equal(t, "keep replacement prompt", prompt.(capturedPrompt).text)
}

func TestQuickChatStartFailurePreservesCapacityDeferredSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-quick-chat-capacity", "session-quick-chat-capacity", models.TaskSessionStateCreated)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-quick-chat-capacity"] = &v1.Task{ID: "task-quick-chat-capacity", IsEphemeral: true}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})

	err := svc.handleQuickChatStartCreatedFailure(
		ctx,
		"task-quick-chat-capacity",
		"session-quick-chat-capacity",
		"attempt-capacity",
		"",
		ErrCeilingLaunchDeferred,
	)
	require.ErrorIs(t, err, ErrCeilingLaunchDeferred)

	session, err := repo.GetTaskSession(ctx, "session-quick-chat-capacity")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCreated, session.State)
	_, hasFailure := models.LoadLastAgentError(session.Metadata)
	require.False(t, hasFailure)
}
