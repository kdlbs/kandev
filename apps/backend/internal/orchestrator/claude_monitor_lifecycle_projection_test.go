package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// @covers AC-AGENTS-BACKGROUND-WORK-002.2
// @covers AC-AGENTS-BACKGROUND-WORK-010.4
func TestClaudeMonitorWorkloadProjection(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-claude-monitor", "session-claude-monitor"
	seedSession(t, repo, taskID, sessionID, "step1")
	taskService := taskservice.NewService(
		taskservice.Repos{BackgroundWork: repo}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{},
	)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.backgroundWorkObserver = taskService
	recorded := &recordingEventBus{}
	svc.eventBus = recorded

	startedAt := time.Now().UTC().Add(-time.Minute)
	first := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "monitor-task-first", RunID: "monitor-task-first",
		Kind: streams.WorkloadKindMonitor, Title: "gh pr checks --watch", State: streams.RunStateRunning,
		OriginTurnID: "turn-monitor-launch", SourceCallID: "toolu-monitor-first", StartedAt: &startedAt, Revision: 1,
	}
	sibling := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: "monitor-task-second", RunID: "monitor-task-second",
		Kind: streams.WorkloadKindMonitor, Title: "go test ./...", State: streams.RunStateRunning,
		OriginTurnID: "turn-monitor-launch", SourceCallID: "toolu-monitor-second", StartedAt: &startedAt, Revision: 1,
	}
	feedBackgroundWorkObservation(svc, taskID, sessionID, &first)
	feedBackgroundWorkObservation(svc, taskID, sessionID, &sibling)

	finishedAt := time.Now().UTC()
	completion := streams.WorkloadRunObservation{
		SessionID: sessionID, WorkID: first.WorkID, RunID: first.RunID,
		Kind: streams.WorkloadKindMonitor, Title: first.Title, State: streams.RunStateFailed,
		SourceCallID: first.SourceCallID, FinishedAt: &finishedAt,
	}
	require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""))
	const successorTurnID = "turn-monitor-successor"
	svc.activeTurns.Store(sessionID, successorTurnID)
	feedBackgroundWorkObservation(svc, taskID, sessionID, &completion)

	settled, err := repo.GetBackgroundWorkload(ctx, sessionID, first.WorkID)
	require.NoError(t, err)
	require.NotNil(t, settled)
	require.Equal(t, string(streams.WorkloadKindMonitor), settled.Kind)
	require.Equal(t, string(streams.RunStateFailed), settled.State)
	require.Equal(t, first.SourceCallID, settled.SourceCallID)
	require.Equal(t, first.OriginTurnID, settled.OriginTurnID)
	require.NotNil(t, settled.FinishedAt)
	require.Nil(t, settled.ExitCode, "provider status does not supply a shell exit code")

	liveSibling, err := repo.GetBackgroundWorkload(ctx, sessionID, sibling.WorkID)
	require.NoError(t, err)
	require.NotNil(t, liveSibling)
	require.Equal(t, string(streams.RunStateRunning), liveSibling.State)
	require.Equal(t, sibling.SourceCallID, liveSibling.SourceCallID)
	currentSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, currentSession.State,
		"a background completion during successor handoff must not wake the foreground session")
	activeTurn, ok := svc.activeTurns.Load(sessionID)
	require.True(t, ok)
	require.Equal(t, successorTurnID, activeTurn,
		"the original Monitor completion must not consume successor-turn ownership")

	publications := backgroundWorkUpdates(recorded)
	require.Len(t, publications, 3)
	require.Equal(t, first.WorkID, publications[2].WorkID)
	require.Equal(t, streams.RunStateFailed, publications[2].State)
	require.Equal(t, first.SourceCallID, publications[2].SourceCallID)
	require.Equal(t, first.OriginTurnID, publications[2].OriginTurnID)

	feedBackgroundWorkObservation(svc, taskID, sessionID, &first)
	settled, err = repo.GetBackgroundWorkload(ctx, sessionID, first.WorkID)
	require.NoError(t, err)
	require.Equal(t, string(streams.RunStateFailed), settled.State, "late Monitor progress cannot reopen a terminal run")

	_, err = taskService.SettleOpenBackgroundWorkloads(ctx, sessionID)
	require.NoError(t, err)
	liveSibling, err = repo.GetBackgroundWorkload(ctx, sessionID, sibling.WorkID)
	require.NoError(t, err)
	require.NotNil(t, liveSibling)
	require.Equal(t, string(streams.RunStateEnded), liveSibling.State,
		"owner exit may settle the remaining open Monitor without changing the provider outcome")
	settled, err = repo.GetBackgroundWorkload(ctx, sessionID, first.WorkID)
	require.NoError(t, err)
	require.Equal(t, string(streams.RunStateFailed), settled.State)
}

// @covers AC-AGENTS-BACKGROUND-WORK-010.8
func TestClaudeMonitorRegistrationObservationPopulatesTitle(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		output    any
		wantTitle string
	}{
		{
			name: "description takes precedence",
			input: map[string]any{"raw_input": map[string]any{
				"description": "watch required checks", "command": "gh pr checks --watch",
			}},
			output:    monitorObservationOutput("output command"),
			wantTitle: "watch required checks",
		},
		{
			name:      "command fallback",
			input:     map[string]any{"raw_input": map[string]any{"command": "gh pr checks --watch"}},
			output:    monitorObservationOutput("output command"),
			wantTitle: "gh pr checks --watch",
		},
		{
			name:      "monitor view fallback",
			input:     map[string]any{"raw_input": map[string]any{}},
			output:    monitorObservationOutput("output command"),
			wantTitle: "output command",
		},
		{
			name:      "generic fallback",
			input:     map[string]any{"raw_input": map[string]any{}},
			wantTitle: "Monitor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := streams.NewGeneric("other", test.input)
			payload.Generic().Output = test.output
			payload.SetMonitorIdentity("monitor-task-1", false)

			observation := buildObservationFromNormalized(&lifecycle.AgentStreamEventPayload{
				SessionID: "session-1",
				Data: &lifecycle.AgentStreamEventData{
					ToolCallID: "toolu_monitor_1",
					Normalized: payload,
				},
			}, "turn-1")

			require.NotNil(t, observation)
			require.Equal(t, test.wantTitle, observation.Title)
		})
	}
}

func monitorObservationOutput(command string) any {
	return map[string]any{"monitor": map[string]any{"command": command}}
}
