package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestClaudeMonitorTerminalDispatchPreservesNativeOutcomeDuringSuccessorHandoff(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-native-monitor", "session-native-monitor"
	seedSession(t, repo, taskID, sessionID, "step1")
	taskService := taskservice.NewService(taskservice.Repos{BackgroundWork: repo}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{})
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.backgroundWorkObserver = taskService
	messageCreator := &mockMessageCreator{}
	svc.messageCreator = messageCreator
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	startedAt := time.Now().UTC().Add(-time.Minute)
	first := nativeMonitorObservation(sessionID, "monitor-first", "toolu-first", streams.RunStateRunning, startedAt, "turn-original")
	sibling := nativeMonitorObservation(sessionID, "monitor-sibling", "toolu-sibling", streams.RunStateRunning, startedAt, "turn-original")
	feedBackgroundWorkObservation(svc, taskID, sessionID, &first)
	feedBackgroundWorkObservation(svc, taskID, sessionID, &sibling)

	finishedAt := time.Now().UTC()
	interrupted := nativeMonitorObservation(sessionID, first.WorkID, first.SourceCallID, streams.RunStateInterrupted, finishedAt, "")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""))
	const successorTurnID = "turn-successor"
	svc.activeTurns.Store(sessionID, successorTurnID)
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundWorkUpdated, BackgroundWork: &interrupted},
	})

	normalized := streams.NewGeneric("other", nil)
	normalized.SetMonitorIdentity(first.WorkID, true)
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventToolUpdate, ToolCallID: first.SourceCallID, ToolStatus: "cancelled", Normalized: normalized,
		},
	})

	assertNativeMonitorDispatchState(t, ctx, repo, svc, recorded, sessionID, first, sibling, finishedAt, successorTurnID)
	require.Equal(t, first.SourceCallID, messageCreator.lastToolUpdateID)
	require.Equal(t, 1, messageCreator.toolUpdateWrites)
}

type nativeMonitorDispatchRepository interface {
	GetBackgroundWorkload(context.Context, string, string) (*models.BackgroundWorkload, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}

func nativeMonitorObservation(
	sessionID, workID, callID string,
	state streams.RunState,
	when time.Time,
	turnID string,
) streams.WorkloadRunObservation {
	observation := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: workID, RunID: workID, Kind: streams.WorkloadKindMonitor,
		Title: "watch checks", State: state, SourceCallID: callID, OriginTurnID: turnID,
	}
	if state == streams.RunStateRunning {
		observation.StartedAt = &when
	} else {
		observation.FinishedAt = &when
	}
	return observation
}

func assertNativeMonitorDispatchState(
	t *testing.T,
	ctx context.Context,
	repo nativeMonitorDispatchRepository,
	svc *Service,
	recorded *recordingEventBus,
	sessionID string,
	first, sibling streams.WorkloadRunObservation,
	finishedAt time.Time,
	successorTurnID string,
) {
	t.Helper()
	settled, err := repo.GetBackgroundWorkload(ctx, sessionID, first.WorkID)
	require.NoError(t, err)
	require.NotNil(t, settled)
	require.Equal(t, string(streams.RunStateInterrupted), settled.State)
	require.Equal(t, finishedAt.UnixNano(), settled.FinishedAt.UnixNano())
	require.Equal(t, first.OriginTurnID, settled.OriginTurnID)
	liveSibling, err := repo.GetBackgroundWorkload(ctx, sessionID, sibling.WorkID)
	require.NoError(t, err)
	require.NotNil(t, liveSibling)
	require.Equal(t, string(streams.RunStateRunning), liveSibling.State)
	currentSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, currentSession.State)
	activeTurn, ok := svc.activeTurns.Load(sessionID)
	require.True(t, ok)
	require.Equal(t, successorTurnID, activeTurn)
	updates := backgroundWorkUpdates(recorded)
	require.Len(t, updates, 3, "the terminal Monitor must be recorded once")
	require.Equal(t, streams.RunStateInterrupted, updates[2].State)
}
