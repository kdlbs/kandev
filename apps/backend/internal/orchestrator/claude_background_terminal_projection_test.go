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

func feedBackgroundWorkObservation(
	svc *Service,
	taskID, sessionID string,
	observation *streams.WorkloadRunObservation,
) {
	svc.recordBackgroundWorkObservationFromToolPayload(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			BackgroundWork: observation,
		},
	})
}

// @covers AC-AGENTS-BACKGROUND-WORK-002.2
// @covers AC-AGENTS-BACKGROUND-WORK-010.6
func TestClaudeBackgroundTerminalProjection(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-claude-terminal", "session-claude-terminal"
	seedSession(t, repo, taskID, sessionID, "step1")

	taskService := taskservice.NewService(
		taskservice.Repos{BackgroundWork: repo},
		nil,
		testLogger(),
		taskservice.RepositoryDiscoveryConfig{},
	)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.backgroundWorkObserver = taskService
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	startedAt := time.Now().UTC().Add(-time.Minute)
	firstLaunch := streams.WorkloadRunObservation{
		SessionID:    sessionID,
		WorkID:       "toolu_first",
		Kind:         streams.WorkloadKindShell,
		Title:        "Run the test suite",
		State:        streams.RunStateRunning,
		OriginTurnID: "turn-1",
		SourceCallID: "toolu_first",
		StartedAt:    &startedAt,
		Revision:     1,
	}
	siblingLaunch := streams.WorkloadRunObservation{
		SessionID:    sessionID,
		WorkID:       "toolu_second",
		Kind:         streams.WorkloadKindShell,
		Title:        "Watch checks",
		State:        streams.RunStateRunning,
		OriginTurnID: "turn-1",
		SourceCallID: "toolu_second",
		StartedAt:    &startedAt,
		Revision:     1,
	}
	feedBackgroundWorkObservation(svc, taskID, sessionID, &firstLaunch)
	feedBackgroundWorkObservation(svc, taskID, sessionID, &siblingLaunch)

	finishedAt := time.Now().UTC()
	terminal := streams.WorkloadRunObservation{
		SessionID:    sessionID,
		WorkID:       "toolu_first",
		Kind:         streams.WorkloadKindShell,
		Title:        "Run the test suite",
		State:        streams.RunStateEnded,
		OriginTurnID: "late-turn-must-not-replace-the-launch-turn",
		SourceCallID: "toolu_first",
		FinishedAt:   &finishedAt,
		Revision:     2,
	}
	feedBackgroundWorkObservation(svc, taskID, sessionID, &terminal)

	first, err := repo.GetBackgroundWorkload(ctx, sessionID, "toolu_first")
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, string(streams.RunStateEnded), first.State)
	require.Nil(t, first.ExitCode, "an unavailable outcome must stay unavailable")
	require.Equal(t, "turn-1", first.OriginTurnID, "the launch turn association is immutable")
	require.Equal(t, "toolu_first", first.SourceCallID)
	require.Equal(t, "Run the test suite", first.Title)
	require.NotNil(t, first.FinishedAt)

	sibling, err := repo.GetBackgroundWorkload(ctx, sessionID, "toolu_second")
	require.NoError(t, err)
	require.NotNil(t, sibling)
	require.Equal(t, string(streams.RunStateRunning), sibling.State,
		"one job's exit must not settle its live sibling")

	published := backgroundWorkUpdates(recorded)
	require.Len(t, published, 3)
	terminalPublication := published[2]
	require.Equal(t, "toolu_first", terminalPublication.WorkID)
	require.Equal(t, streams.RunStateEnded, terminalPublication.State)
	require.Nil(t, terminalPublication.ExitCode)
	require.Equal(t, "turn-1", terminalPublication.OriginTurnID)
	require.Equal(t, "toolu_first", terminalPublication.SourceCallID)
	require.Equal(t, int64(2), terminalPublication.Revision)

	// A delayed non-terminal observation and a duplicate terminal frame are
	// both rejected by the committed terminal state.
	replay := firstLaunch
	replay.Revision = 1
	feedBackgroundWorkObservation(svc, taskID, sessionID, &replay)
	feedBackgroundWorkObservation(svc, taskID, sessionID, &terminal)

	first, err = repo.GetBackgroundWorkload(ctx, sessionID, "toolu_first")
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, string(streams.RunStateEnded), first.State)
	require.Nil(t, first.ExitCode)
	require.Equal(t, int64(2), first.Revision)
}
