package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// @covers AC-AGENTS-BACKGROUND-WORK-011.1
// @covers AC-AGENTS-BACKGROUND-WORK-011.2
// @covers AC-AGENTS-BACKGROUND-WORK-011.4
func TestCursorShellWatchProjection(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-cursor-watch", "session-cursor-watch"
	seedSession(t, repo, taskID, sessionID, "step1")
	taskService := taskservice.NewService(
		taskservice.Repos{BackgroundWork: repo}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{},
	)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.backgroundWorkObserver = taskService
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	startedAt := time.Now().UTC().Add(-time.Minute)
	capabilities := streams.InspectionOnlyWorkloadCapabilities()
	first := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "cursor-work-first", RunID: "cursor-work-first",
		Kind: streams.WorkloadKindMonitor, Title: "Wait for build — /tmp/build.sh",
		State: streams.RunStateRunning, OriginTurnID: "turn-launch", SourceCallID: "watch-first",
		Capabilities: capabilities, StartedAt: &startedAt,
	}
	sibling := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "cursor-work-second", RunID: "cursor-work-second",
		Kind: streams.WorkloadKindMonitor, Title: "Wait for tests — /tmp/test.sh",
		State: streams.RunStateRunning, OriginTurnID: "turn-launch", SourceCallID: "watch-second",
		Capabilities: capabilities, StartedAt: &startedAt,
	}
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &first)
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &sibling)

	finishedAt := time.Now().UTC()
	completed := first
	completed.State = streams.RunStateCompleted
	completed.FinishedAt = &finishedAt
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &completed)
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &completed)
	delayedProgress := first
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &delayedProgress)

	svc.markExecutionCompleted(sessionID, "execution-replaced")
	wrongOwner := completed
	wrongOwner.State = streams.RunStateFailed
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-replaced", &wrongOwner)

	stored, err := repo.GetBackgroundWorkload(ctx, sessionID, first.WorkID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, string(streams.RunStateCompleted), stored.State)
	require.Equal(t, first.SourceCallID, stored.SourceCallID)
	require.Equal(t, first.OriginTurnID, stored.OriginTurnID)
	require.Equal(t, first.Title, stored.Title)
	require.Nil(t, stored.ExitCode, "watch completion does not report the shell exit code")
	require.NotNil(t, stored.FinishedAt)
	require.Equal(t, int64(2), stored.Revision)

	siblingStored, err := repo.GetBackgroundWorkload(ctx, sessionID, sibling.WorkID)
	require.NoError(t, err)
	require.NotNil(t, siblingStored)
	require.Equal(t, string(streams.RunStateRunning), siblingStored.State)

	runs, err := repo.ListBackgroundRunsByWorkload(ctx, sessionID, first.WorkID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, first.RunID, runs[0].ID)
	require.Equal(t, string(streams.RunStateCompleted), runs[0].State)

	published := backgroundWorkUpdates(recorded)
	require.NotEmpty(t, published)
	last := published[len(published)-1]
	require.Equal(t, stored.ID, last.WorkID)
	require.Equal(t, streams.RunStateCompleted, last.State)
	require.Equal(t, stored.Revision, last.Revision)
	require.Equal(t, stored.SourceCallID, last.SourceCallID)
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.6
func TestCursorShellWatchFailureProjectionRetainsHistoryAndSibling(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-cursor-watch-failure", "session-cursor-watch-failure"
	seedSession(t, repo, taskID, sessionID, "step1")
	taskService := taskservice.NewService(
		taskservice.Repos{BackgroundWork: repo}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{},
	)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.backgroundWorkObserver = taskService
	startedAt := time.Now().UTC().Add(-time.Minute)
	failedWatch := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "cursor-watch-failed", RunID: "cursor-watch-failed",
		Kind: streams.WorkloadKindMonitor, Title: "Wait for build", State: streams.RunStateRunning,
		SourceCallID: "watch-failed", Capabilities: streams.InspectionOnlyWorkloadCapabilities(), StartedAt: &startedAt,
	}
	sibling := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "cursor-watch-sibling", RunID: "cursor-watch-sibling",
		Kind: streams.WorkloadKindMonitor, Title: "Wait for tests", State: streams.RunStateRunning,
		SourceCallID: "watch-sibling", Capabilities: streams.InspectionOnlyWorkloadCapabilities(), StartedAt: &startedAt,
	}
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &failedWatch)
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &sibling)

	finishedAt := time.Now().UTC()
	failedWatch.State = streams.RunStateFailed
	failedWatch.FinishedAt = &finishedAt
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &failedWatch)
	lateProgress := failedWatch
	lateProgress.State = streams.RunStateRunning
	lateProgress.FinishedAt = nil
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &lateProgress)

	stored, err := repo.GetBackgroundWorkload(ctx, sessionID, failedWatch.WorkID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, string(streams.RunStateFailed), stored.State)
	require.NotNil(t, stored.FinishedAt)
	failedRuns, err := repo.ListBackgroundRunsByWorkload(ctx, sessionID, failedWatch.WorkID)
	require.NoError(t, err)
	require.Len(t, failedRuns, 1)
	require.Equal(t, string(streams.RunStateFailed), failedRuns[0].State)

	siblingStored, err := repo.GetBackgroundWorkload(ctx, sessionID, sibling.WorkID)
	require.NoError(t, err)
	require.NotNil(t, siblingStored)
	require.Equal(t, string(streams.RunStateRunning), siblingStored.State)
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.8
func TestCursorShellWatchResetProjectionSeparatesReusedCallID(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-cursor-watch-reset", "session-cursor-watch-reset"
	seedSession(t, repo, taskID, sessionID, "step1")
	taskService := taskservice.NewService(
		taskservice.Repos{BackgroundWork: repo}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{},
	)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.backgroundWorkObserver = taskService

	retired := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "cursor-watch-retired", RunID: "cursor-watch-retired",
		Kind: streams.WorkloadKindMonitor, Title: "Retired watch", State: streams.RunStateRunning,
		SourceCallID: "watch-reused", Capabilities: streams.InspectionOnlyWorkloadCapabilities(),
	}
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &retired)
	retired.State = streams.RunStateUnknown
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &retired)

	successor := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "cursor-watch-successor", RunID: "cursor-watch-successor",
		Kind: streams.WorkloadKindMonitor, Title: "Successor watch", State: streams.RunStateRunning,
		SourceCallID: retired.SourceCallID, Capabilities: streams.InspectionOnlyWorkloadCapabilities(),
	}
	feedCursorShellWatchEvent(svc, taskID, sessionID, "execution-current", &successor)

	retiredStored, err := repo.GetBackgroundWorkload(ctx, sessionID, retired.WorkID)
	require.NoError(t, err)
	require.NotNil(t, retiredStored)
	require.Equal(t, string(streams.RunStateUnknown), retiredStored.State)
	require.Equal(t, successor.SourceCallID, retiredStored.SourceCallID)
	successorStored, err := repo.GetBackgroundWorkload(ctx, sessionID, successor.WorkID)
	require.NoError(t, err)
	require.NotNil(t, successorStored)
	require.Equal(t, string(streams.RunStateRunning), successorStored.State)
	require.Equal(t, retiredStored.SourceCallID, successorStored.SourceCallID)
	require.NotEqual(t, retiredStored.ID, successorStored.ID)
}

func feedCursorShellWatchEvent(
	svc *Service,
	taskID, sessionID, executionID string,
	observation *streams.WorkloadRunObservation,
) {
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: streams.EventTypeBackgroundWorkUpdated, BackgroundWork: observation,
		},
	})
}
