package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type initialCreatePromptQueueFailureRepository struct {
	messagequeue.Repository
	err error
}

func (r *initialCreatePromptQueueFailureRepository) Insert(
	ctx context.Context,
	msg *messagequeue.QueuedMessage,
	maxPerSession int,
) error {
	return r.err
}

func (r *initialCreatePromptQueueFailureRepository) InsertForSession(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	msg *messagequeue.QueuedMessage,
	maxPerSession int,
) error {
	return r.err
}

func TestInitialCreatePrompt_TransitionsBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-prompt", "session-create-prompt", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-prompt", models.TaskSessionStateCreated, ""))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1,
	}

	seedExecutorRunning(t, repo, "session-create-prompt", "task-create-prompt", "execution-create-prompt")
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-create-prompt"] = &v1.Task{
		ID: "task-create-prompt", Title: "Create prompt", Description: "initial prompt",
		State: v1.TaskStateInProgress,
	}
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	var providerDispatches atomic.Int32
	dispatchObserved := make(chan string, 1)
	agentMgr.launchAgentFunc = func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		return &executor.LaunchAgentResponse{AgentExecutionID: "execution-create-prompt-dispatch"}, nil
	}
	agentMgr.startAgentProcessFunc = func(_ context.Context, executionID string) error {
		providerDispatches.Add(1)
		dispatchObserved <- executionID
		return nil
	}
	svc := createEngineServiceWithScheduler(t, repo, stepGetter, taskRepo, agentMgr)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:              "task-create-prompt",
		SessionID:           "session-create-prompt",
		Intent:              IntentStartCreated,
		AgentProfileID:      "profile-create-prompt",
		Prompt:              "initial prompt",
		InitialCreatePrompt: true,
	})
	require.NoError(t, err)
	select {
	case executionID := <-dispatchObserved:
		require.Equal(t, "execution-create-prompt", executionID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the initial provider dispatch")
	}

	task, err := repo.GetTask(ctx, "task-create-prompt")
	require.NoError(t, err)
	require.Equal(t, "step-spec", task.WorkflowStepID)
	require.Len(t, messages.userMessages, 1)
	require.Contains(t, messages.userMessages[0].content, "initial prompt")
	require.Equal(t, int32(1), providerDispatches.Load(),
		"the initial prompt must cross one provider dispatch boundary")
}

func TestInitialCreatePrompt_TransitionFailurePreventsLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-failure", "session-create-failure", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-failure", models.TaskSessionStateCreated, ""))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-failure", v1.TaskStateInProgress)
	svc := createEngineServiceWithScheduler(t, repo, stepGetter, taskRepo, &mockAgentManager{})

	_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:              "task-create-failure",
		SessionID:           "session-create-failure",
		Intent:              IntentStartCreated,
		AgentProfileID:      "profile-create-failure",
		Prompt:              "initial prompt",
		InitialCreatePrompt: true,
	})
	require.Error(t, err)

	task, err := repo.GetTask(ctx, "task-create-failure")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID)
	session, err := repo.GetTaskSession(ctx, "session-create-failure")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, session.State)
	require.Contains(t, session.ErrorMessage, "on_turn_start")
}

func TestInitialCreatePrompt_AdmissionFailureDoesNotFailSupersededSuccessor(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-superseded", "session-create-superseded", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx, "session-create-superseded", models.TaskSessionStateCancelled, "operator stopped",
	))
	source, err := repo.GetTaskSession(ctx, "session-create-superseded")
	require.NoError(t, err)
	source.IsPrimary = false
	require.NoError(t, repo.UpdateTaskSession(ctx, source))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        "session-create-successor",
		TaskID:    "task-create-superseded",
		State:     models.TaskSessionStateCreated,
		IsPrimary: true,
		StartedAt: time.Now().UTC(),
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-superseded", v1.TaskStateInProgress)
	launchCalls := 0
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			return nil, errors.New("unexpected launch")
		},
	}
	svc := createEngineServiceWithScheduler(t, repo, stepGetter, taskRepo, agentMgr)

	_, err = svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: "task-create-superseded", SessionID: "session-create-superseded",
		Intent: IntentStartCreated, AgentProfileID: "profile-create-superseded",
		Prompt: "initial prompt", InitialCreatePrompt: true,
	})
	require.Error(t, err)
	require.Zero(t, launchCalls, "a superseded admission must not dispatch a provider launch")

	failedSource, err := repo.GetTaskSession(ctx, "session-create-superseded")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, failedSource.State)
	successor, err := repo.GetTaskSession(ctx, "session-create-successor")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCreated, successor.State,
		"a successor that did not own this creation attempt must remain launchable")
}

func TestInitialCreatePrompt_LaunchFailureUsesReplacementSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-replacement-failure", "session-create-replacement-source", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx, "session-create-replacement-source", models.TaskSessionStateCreated, "",
	))
	source, err := repo.GetTaskSession(ctx, "session-create-replacement-source")
	require.NoError(t, err)
	source.AgentProfileID = "profile-source"
	source.ExecutorID = models.ExecutorIDLocal
	source.TaskEnvironmentID = "environment-create-replacement"
	require.NoError(t, repo.UpdateTaskSession(ctx, source))
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:           source.TaskEnvironmentID,
		TaskID:       source.TaskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1,
		AgentProfileID: "profile-destination",
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-create-replacement-failure"] = &v1.Task{
		ID: "task-create-replacement-failure", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Replacement failure", Description: "initial prompt",
		State: v1.TaskStateInProgress,
	}
	launchCalls := 0
	agentMgr := &mockAgentManager{
		getExecutionIDForSessionFunc: func(_ context.Context, sessionID string) (string, error) {
			if sessionID == source.ID {
				return "", lifecycle.ErrNoExecutionForSession
			}
			return "", errors.New("execution intentionally unavailable")
		},
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			if launchCalls == 1 {
				return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-workspace"}, nil
			}
			return nil, errors.New("destination launch failed")
		},
	}
	svc := createEngineServiceWithScheduler(t, repo, stepGetter, taskRepo, agentMgr)

	_, err = svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: "task-create-replacement-failure", SessionID: source.ID,
		Intent: IntentStartCreated, AgentProfileID: "profile-source",
		Prompt: "initial prompt", InitialCreatePrompt: true,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "destination launch failed")
	require.Equal(t, 2, launchCalls, "workspace preparation and the actual prompt launch must both be attempted")

	original, err := repo.GetTaskSession(ctx, source.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, original.State,
		"the parked source must remain available without owning the new prompt")
	sessions, err := repo.ListTaskSessions(ctx, "task-create-replacement-failure")
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	var replacement *models.TaskSession
	for _, candidate := range sessions {
		if candidate.ID != source.ID {
			replacement = candidate
			break
		}
	}
	require.NotNil(t, replacement)
	require.Equal(t, "profile-destination", replacement.AgentProfileID)
	require.Equal(t, models.TaskSessionStateFailed, replacement.State,
		"the launch error belongs to the replacement that received the prompt")
	require.Contains(t, replacement.ErrorMessage, "destination launch failed")
}

func TestInitialCreatePrompt_PassthroughRunningDoesNotRepeatTurnStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-passthrough", "session-create-passthrough", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-passthrough", models.TaskSessionStateWaitingForInput, ""))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1,
	}
	seedExecutorRunning(t, repo, "session-create-passthrough", "task-create-passthrough", "execution-create-passthrough")
	agentMgr := &mockAgentManager{isPassthrough: true, repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-passthrough", v1.TaskStateInProgress)
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
	session, err := repo.GetTaskSession(ctx, "session-create-passthrough")
	require.NoError(t, err)
	svc.activeTurns.Store(session.ID, "turn-create-initial")
	svc.armInitialCreatePromptPassthrough(ctx, session, "turn-create-initial")

	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-passthrough", SessionID: "session-create-passthrough",
		AgentExecutionID: "execution-create-passthrough",
	})
	task, err := repo.GetTask(ctx, "task-create-passthrough")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID,
		"the passthrough running event must not repeat the already-admitted trigger")

	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-passthrough", SessionID: "session-create-passthrough",
		AgentExecutionID: "execution-create-passthrough",
	})
	task, err = repo.GetTask(ctx, "task-create-passthrough")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID,
		"a duplicate running event must not imply a later user turn")

	// A later user turn clears the old admission evidence when its turn is
	// accepted. The following running event can then evaluate on_turn_start.
	svc.activeTurns.Store(session.ID, "turn-create-later")
	svc.clearInitialCreatePromptPassthroughForNewTurn(session.ID, "turn-create-later")
	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-passthrough", SessionID: "session-create-passthrough",
		AgentExecutionID: "execution-create-passthrough",
	})
	task, err = repo.GetTask(ctx, "task-create-passthrough")
	require.NoError(t, err)
	require.Equal(t, "step-spec", task.WorkflowStepID,
		"a real later passthrough turn must still evaluate on_turn_start")
}

func TestInitialCreatePrompt_PassthroughEvidenceSurvivesProcessRestart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-passthrough-restart", "session-create-passthrough-restart", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx, "session-create-passthrough-restart", models.TaskSessionStateWaitingForInput, "",
	))
	seedExecutorRunning(t, repo, "session-create-passthrough-restart", "task-create-passthrough-restart", "execution-create-passthrough-restart")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-passthrough-restart", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{isPassthrough: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
	session, err := repo.GetTaskSession(ctx, "session-create-passthrough-restart")
	require.NoError(t, err)
	svc.activeTurns.Store(session.ID, "turn-create-restart")
	svc.armInitialCreatePromptPassthrough(ctx, session, "turn-create-restart")

	persisted, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Contains(t, persisted.Metadata, models.SessionMetaKeyInitialCreatePromptPassthrough,
		"the admission evidence must survive a process restart")

	// A restarted service has no in-memory evidence map. Hydration from the
	// session row must still suppress the exact initial running event.
	svc.initialCreatePromptMu.Lock()
	svc.initialCreatePromptPassthrough = nil
	svc.initialCreatePromptMu.Unlock()
	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-passthrough-restart", SessionID: session.ID,
		AgentExecutionID: "execution-create-passthrough-restart",
	})
	task, err := repo.GetTask(ctx, "task-create-passthrough-restart")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID,
		"restart recovery must suppress the already-admitted turn")

	// Duplicate running events remain suppressed, while an explicitly accepted
	// later turn clears the durable evidence and restores ordinary behavior.
	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-passthrough-restart", SessionID: session.ID,
		AgentExecutionID: "execution-create-passthrough-restart",
	})
	task, err = repo.GetTask(ctx, "task-create-passthrough-restart")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID)
	svc.activeTurns.Store(session.ID, "turn-create-restart-later")
	svc.clearInitialCreatePromptPassthroughForNewTurn(session.ID, "turn-create-restart-later")
	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-passthrough-restart", SessionID: session.ID,
		AgentExecutionID: "execution-create-passthrough-restart",
	})
	task, err = repo.GetTask(ctx, "task-create-passthrough-restart")
	require.NoError(t, err)
	require.Equal(t, "step-spec", task.WorkflowStepID,
		"a later user turn must evaluate on_turn_start after restart recovery")
}

func TestInitialCreatePrompt_PassthroughEvidenceSurvivesPredecessorTerminalEvents(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-successor", "session-create-successor", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-successor", models.TaskSessionStateWaitingForInput, ""))
	seedExecutorRunning(t, repo, "session-create-successor", "task-create-successor", "execution-successor")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-successor", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{isPassthrough: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	session, err := repo.GetTaskSession(ctx, "session-create-successor")
	require.NoError(t, err)
	svc.activeTurns.Store(session.ID, "turn-successor")
	svc.armInitialCreatePromptPassthroughForLaunch(ctx, session, "turn-successor")

	predecessor := watcher.AgentEventData{
		TaskID: "task-create-successor", SessionID: session.ID, AgentExecutionID: "execution-predecessor",
		PromptGeneration: 1,
	}
	svc.handleAgentRunning(ctx, predecessor)
	svc.retireInitialCreatePromptPassthroughForEvent(ctx, predecessor)
	svc.handleAgentCompleted(ctx, predecessor)
	svc.handleAgentFailed(ctx, predecessor)
	svc.handleAgentStopped(ctx, predecessor)
	svc.bindInitialCreatePromptPassthroughExecution(ctx, session.ID, "turn-successor", "execution-successor")
	svc.retireInitialCreatePromptPassthroughForEvent(ctx, predecessor)

	task, err := repo.GetTask(ctx, "task-create-successor")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID)

	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-successor", SessionID: session.ID, AgentExecutionID: "execution-successor",
	})
	task, err = repo.GetTask(ctx, "task-create-successor")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID,
		"the successor running event must still consume the initial admission evidence")
}

func TestInitialCreatePrompt_QueueReplayTransfersPassthroughEvidence(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-queue-passthrough", "session-create-queue-passthrough", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-queue-passthrough", models.TaskSessionStateWaitingForInput, ""))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-queue-passthrough", v1.TaskStateInProgress)
	seedExecutorRunning(t, repo, "session-create-queue-passthrough", "task-create-queue-passthrough", "execution-create-queue-passthrough")
	agentMgr := &mockAgentManager{isPassthrough: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)

	session, err := repo.GetTaskSession(ctx, "session-create-queue-passthrough")
	require.NoError(t, err)
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task-create-queue-passthrough", session.ID)
	require.NoError(t, err)
	queued := &messagequeue.QueuedMessage{
		ID:        "queue-create-passthrough",
		TaskID:    "task-create-queue-passthrough",
		SessionID: session.ID,
		Content:   "initial prompt",
		Metadata: map[string]interface{}{
			MetaKeyTurnStartAlreadyProcessed:      true,
			metaKeyInitialCreatePromptPassthrough: true,
		},
	}
	svc.activeTurns.Store(session.ID, "turn-create-queue-initial")

	afterClaim := svc.queuedMessageAfterClaim(ctx, identity, queued, nil, false, nil)
	require.NoError(t, afterClaim())
	// The production passthrough worker binds this evidence immediately after
	// PreparePassthroughRunning claims the successor execution. Model that
	// boundary here so the successor event proves suppression, rather than only
	// proving that an unbound marker rejects every event.
	svc.bindInitialCreatePromptPassthroughExecution(
		ctx, session.ID, "turn-create-queue-initial", "execution-create-queue-passthrough",
	)

	// The predecessor event arrives after the successor's admission evidence
	// is armed. It must be side-effect free.
	svc.retireInitialCreatePromptPassthroughForQueueEvent(
		ctx,
		session.ID,
		identity.SessionIncarnationID,
		"execution-predecessor",
		"turn-create-queue-initial",
		0,
	)
	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-queue-passthrough", SessionID: session.ID,
		AgentExecutionID: "execution-predecessor",
	})
	task, err := repo.GetTask(ctx, "task-create-queue-passthrough")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID,
		"a predecessor running event must not consume queued successor evidence")

	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-queue-passthrough", SessionID: session.ID,
		AgentExecutionID: "execution-create-queue-passthrough",
	})
	task, err = repo.GetTask(ctx, "task-create-queue-passthrough")
	require.NoError(t, err)
	require.Equal(t, "step-backlog", task.WorkflowStepID,
		"a duplicate queued running event must not imply a later user turn")
	svc.initialCreatePromptMu.Lock()
	evidence := svc.initialCreatePromptPassthrough[session.ID]
	svc.initialCreatePromptMu.Unlock()
	require.True(t, evidence.Consumed, "the admitted successor event must consume the marker")

	svc.activeTurns.Store(session.ID, "turn-create-queue-later")
	svc.clearInitialCreatePromptPassthroughForNewTurn(session.ID, "turn-create-queue-later")
	svc.handleAgentRunning(ctx, watcher.AgentEventData{
		TaskID: "task-create-queue-passthrough", SessionID: session.ID,
		AgentExecutionID: "execution-create-queue-passthrough",
	})
	task, err = repo.GetTask(ctx, "task-create-queue-passthrough")
	require.NoError(t, err)
	require.Equal(t, "step-spec", task.WorkflowStepID,
		"a real later queued turn must still evaluate on_turn_start")
}

func TestInitialCreatePrompt_QueuesAfterTurnStartAdmission(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-queued", "session-create-queued", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-queued", models.TaskSessionStateCreated, ""))
	queuedSession, err := repo.GetTaskSession(ctx, "session-create-queued")
	require.NoError(t, err)
	queuedSession.IsPassthrough = true
	require.NoError(t, repo.UpdateTaskSession(ctx, queuedSession))
	task, err := repo.GetTask(ctx, "task-create-queued")
	require.NoError(t, err)
	task.WIPAdmitted = true
	require.NoError(t, repo.UpdateTask(ctx, task))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-create-occupant", WorkspaceID: "ws1", WorkflowID: "wf1",
		WorkflowStepID: "step-spec", Title: "Occupant", State: v1.TaskStateInProgress,
		WIPAdmitted: true,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1, WIPLimit: 1,
		Prompt: "destination automatic prompt",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-queued", v1.TaskStateInProgress)
	seedExecutorRunning(t, repo, "session-create-queued", "task-create-queued", "execution-create-queued")
	agentMgr := &mockAgentManager{isAgentRunning: true, isPassthrough: true, repoForExecutionLookup: repo}
	svc := createEngineServiceWithScheduler(t, repo, stepGetter, taskRepo, agentMgr)

	response, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: "task-create-queued", SessionID: "session-create-queued",
		Intent: IntentStartCreated, Prompt: "initial queued prompt",
		InitialCreatePrompt: true,
	})
	require.NoError(t, err)
	require.Equal(t, "session-create-queued", response.SessionID)

	status := svc.messageQueue.GetStatus(ctx, "session-create-queued")
	require.Len(t, status.Entries, 1)
	require.True(t, turnStartAlreadyProcessed(status.Entries[0].Metadata))
	updatedTask, err := repo.GetTask(ctx, "task-create-queued")
	require.NoError(t, err)
	require.Equal(t, "step-spec", updatedTask.WorkflowStepID)
	require.False(t, updatedTask.WIPAdmitted)

	// Release the WIP slot and drain the durable entry through the production
	// admission-gated queue path. The initial admission already moved the task
	// to Spec, so delivery must not evaluate another on_turn_start transition.
	occupant, err := repo.GetTask(ctx, "task-create-occupant")
	require.NoError(t, err)
	occupant.WIPAdmitted = false
	require.NoError(t, repo.UpdateTask(ctx, occupant))
	updatedTask.WIPAdmitted = true
	updatedTask.QueuedForStepID = ""
	updatedTask.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{"from_step_id": "step-backlog"}
	require.NoError(t, repo.UpdateTask(ctx, updatedTask))
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-queued", models.TaskSessionStateWaitingForInput, ""))
	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: "task-create-queued"})
	require.Eventually(t, func() bool {
		agentMgr.mu.Lock()
		defer agentMgr.mu.Unlock()
		return len(agentMgr.passthroughStdinCalls) == 1 && svc.messageQueue.GetStatus(ctx, "session-create-queued").Count == 0
	}, time.Second, 10*time.Millisecond)
	agentMgr.mu.Lock()
	passthroughPrompt := agentMgr.passthroughStdinCalls[0].Data
	agentMgr.mu.Unlock()
	require.True(t, strings.Contains(passthroughPrompt, "initial queued prompt"))
	require.NotContains(t, passthroughPrompt, "destination automatic prompt")
	finalTask, err := repo.GetTask(ctx, "task-create-queued")
	require.NoError(t, err)
	require.Equal(t, "step-spec", finalTask.WorkflowStepID)
}

func TestInitialCreatePrompt_QueueAdmissionFailurePersistsLaunchError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-create-queue-failure", "session-create-queue-failure", "step-backlog")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-create-queue-failure", models.TaskSessionStateCreated, ""))
	task, err := repo.GetTask(ctx, "task-create-queue-failure")
	require.NoError(t, err)
	task.WIPAdmitted = true
	require.NoError(t, repo.UpdateTask(ctx, task))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-create-queue-failure-occupant", WorkspaceID: "ws1", WorkflowID: "wf1",
		WorkflowStepID: "step-spec", Title: "Occupant", State: v1.TaskStateInProgress,
		WIPAdmitted: true,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step-backlog"] = &wfmodels.WorkflowStep{
		ID: "step-backlog", WorkflowID: "wf1", Name: "Backlog", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}},
		},
	}
	stepGetter.steps["step-spec"] = &wfmodels.WorkflowStep{
		ID: "step-spec", WorkflowID: "wf1", Name: "Spec", Position: 1, WIPLimit: 1,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-create-queue-failure", v1.TaskStateInProgress)
	launchCalls := 0
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls++
			return nil, errors.New("unexpected launch")
		},
	}
	svc := createEngineServiceWithScheduler(t, repo, stepGetter, taskRepo, agentMgr)
	baseQueue := messagequeue.NewMemoryRepositoryWithAuthority(func(context.Context, string, string) (messagequeue.QueueSessionIdentity, error) {
		return messagequeue.QueueSessionIdentity{
			TaskID:               "task-create-queue-failure",
			SessionID:            "session-create-queue-failure",
			SessionIncarnationID: "memory:session-create-queue-failure",
		}, nil
	})
	svc.messageQueue = messagequeue.NewService(
		&initialCreatePromptQueueFailureRepository{Repository: baseQueue, err: errors.New("queue backend unavailable")},
		messagequeue.DefaultMaxPerSession,
		testLogger(),
	)

	_, err = svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:              "task-create-queue-failure",
		SessionID:           "session-create-queue-failure",
		Intent:              IntentStartCreated,
		AgentProfileID:      "profile-create-queue-failure",
		Prompt:              "initial queued prompt",
		InitialCreatePrompt: true,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "queue initial creation prompt")
	require.Zero(t, launchCalls)

	failedSession, err := repo.GetTaskSession(ctx, "session-create-queue-failure")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateFailed, failedSession.State)
	require.Contains(t, failedSession.ErrorMessage, "queue initial creation prompt")
}
