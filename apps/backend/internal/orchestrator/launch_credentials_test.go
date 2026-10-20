package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type launchCredentialReminterFunc func(context.Context, string, string, map[string]string) (map[string]string, error)

func (f launchCredentialReminterFunc) RemintLaunchCredentials(ctx context.Context, taskID, sessionID string, env map[string]string) (map[string]string, error) {
	return f(ctx, taskID, sessionID, env)
}

// @covers AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.1
func TestStartTaskWithEnv_RemintsConcreteOfficeSessionBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "completed-session", models.TaskSessionStateCompleted)
	task, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, repo.UpdateTask(ctx, task))
	tasks := newMockTaskRepo()
	tasks.tasks["task1"] = &v1.Task{ID: "task1", Title: "Office task", Description: "Do the work", State: v1.TaskStateInProgress}
	var dispatched *executor.LaunchAgentRequest
	manager := &mockAgentManager{launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		dispatched = req
		return &executor.LaunchAgentResponse{AgentExecutionID: "exec-" + req.SessionID}, nil
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), tasks, manager)
	var mintedSession string
	env := validOfficeRuntimeEnv()
	svc.SetLaunchCredentialReminter(launchCredentialReminterFunc(func(_ context.Context, taskID, sessionID string, input map[string]string) (map[string]string, error) {
		require.Equal(t, "task1", taskID)
		session, readErr := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, readErr)
		require.Equal(t, taskID, session.TaskID)
		require.NotEqual(t, "completed-session", sessionID)
		mintedSession = sessionID
		refreshed := make(map[string]string, len(input))
		for key, value := range input {
			refreshed[key] = value
		}
		refreshed["KANDEV_API_KEY"], refreshed["KANDEV_RUN_TOKEN"] = "fresh-session-token", "fresh-session-token"
		return refreshed, nil
	}))
	execution, err := svc.StartTaskWithEnv(ctx, "task1", "office-runner", "", "", "", "Do the work", "", false, false, nil, env)
	require.NoError(t, err)
	require.NotNil(t, dispatched)
	require.Equal(t, execution.SessionID, mintedSession)
	require.Equal(t, mintedSession, dispatched.SessionID)
	require.Equal(t, "fresh-session-token", dispatched.Env["KANDEV_API_KEY"])
	require.Equal(t, "fresh-session-token", dispatched.Env["KANDEV_RUN_TOKEN"])
	require.Equal(t, "signed-run-token", env["KANDEV_API_KEY"])
}

func TestCeilingReplay_RefreshFailureRetainsRedactedLaunch(t *testing.T) {
	for _, failAt := range []int{0, 1, 2} {
		t.Run(map[int]string{0: "dispatch control", 1: "before session", 2: "after session"}[failAt], func(t *testing.T) {
			ctx := context.Background()
			fixture := newLaunchFailureReplayFixture(t, "refresh-failure-task", errors.New("executor reached"))
			original := deferredLaunchOf(t, fixture.svc, fixture.taskID)
			deferral, err := models.ReadCeilingDeferral(original)
			require.NoError(t, err)
			deferral.Payload["env"] = redactedCeilingLaunchEnv(map[string]string{"KANDEV_RUN_ID": "run-1", "KANDEV_API_KEY": "captured-token", "KANDEV_RUN_TOKEN": "captured-token"})
			_, prior, readErr := fixture.repo.GetTaskDeferredLaunch(ctx, fixture.taskID)
			require.NoError(t, readErr)
			stored, lostCompare, writeErr := fixture.repo.SetTaskDeferredLaunchIfUnchanged(ctx, fixture.taskID, prior, models.CeilingRecordKeys(deferral))
			require.NoError(t, writeErr)
			require.True(t, stored)
			require.False(t, lostCompare)
			fixture.svc.SetTurnService(&repoBackedTurnService{repo: fixture.repo})
			calls := 0
			cause := errors.New("credential issuer unavailable")
			fixture.svc.SetLaunchCredentialReminter(launchCredentialReminterFunc(func(_ context.Context, taskID, sessionID string, input map[string]string) (map[string]string, error) {
				calls++
				require.Equal(t, fixture.taskID, taskID)
				if calls == 1 {
					require.Empty(t, sessionID)
					require.NotContains(t, input, "KANDEV_API_KEY")
				} else {
					require.NotEmpty(t, sessionID)
				}
				if calls == failAt {
					return nil, cause
				}
				return map[string]string{"KANDEV_RUN_ID": "run-1", "KANDEV_API_KEY": "early-token", "KANDEV_RUN_TOKEN": "early-token"}, nil
			}))
			fixture.svc.drainDeferredCeilingLaunches(ctx)
			wantCalls, wantLaunches, wantSessions := failAt, 0, failAt-1
			if failAt == 0 {
				wantCalls, wantLaunches, wantSessions = 2, 1, 1
			}
			require.Equal(t, wantCalls, calls)
			require.Equal(t, wantLaunches, fixture.launchCount())
			require.True(t, fixture.ceilingDeferred(t))
			retained := deferredLaunchOf(t, fixture.svc, fixture.taskID)
			serialized, err := json.Marshal(retained)
			require.NoError(t, err)
			require.NotContains(t, string(serialized), "captured-token")
			require.NotContains(t, string(serialized), "early-token")
			sessions, err := fixture.repo.ListTaskSessions(ctx, fixture.taskID)
			require.NoError(t, err)
			require.Len(t, sessions, wantSessions)
			if wantSessions == 1 {
				require.Equal(t, models.TaskSessionStateFailed, sessions[0].State)
				turn, turnErr := fixture.repo.GetTurn(ctx, "turn-"+sessions[0].ID)
				require.NoError(t, turnErr)
				require.NotNil(t, turn.CompletedAt)
			}
			entries := fixture.logs.FilterMessage(ceilingReplayRetainedFailureLog).All()
			require.Len(t, entries, 1)
			wantCause := cause.Error()
			if failAt == 0 {
				wantCause = "executor reached"
			}
			require.Contains(t, entries[0].ContextMap()["error"], wantCause)
		})
	}
}

func TestStartTaskWithEnv_RefreshFailureSettlesOwnedSession(t *testing.T) {
	ctx := context.Background()
	repo, svc := seedStartClaimTask(t)
	svc.SetTurnService(&repoBackedTurnService{repo: repo})
	launches := 0
	manager := svc.agentManager.(*mockAgentManager)
	manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		return &executor.LaunchAgentResponse{AgentExecutionID: "exec-" + req.SessionID}, nil
	}
	cause := errors.New("runtime credential mint refused")
	var sessionID string
	svc.SetLaunchCredentialReminter(launchCredentialReminterFunc(func(_ context.Context, taskID, session string, env map[string]string) (map[string]string, error) {
		require.Equal(t, "task1", taskID)
		require.NotEmpty(t, session)
		sessionID = session
		return nil, cause
	}))
	execution, err := svc.StartTaskWithEnv(ctx, "task1", "profile1", "", "", "", "Do the work", "", false, false, nil, map[string]string{"KANDEV_RUN_ID": "run-1", "KANDEV_API_KEY": "captured-token"})
	require.Nil(t, execution)
	require.ErrorContains(t, err, cause.Error())
	require.Zero(t, launches)
	session, readErr := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, readErr)
	require.Equal(t, models.TaskSessionStateFailed, session.State)
	turn, turnErr := repo.GetTurn(ctx, "turn-"+sessionID)
	require.NoError(t, turnErr)
	require.NotNil(t, turn.CompletedAt)
	environment, readErr := repo.GetTaskEnvironmentByTaskID(ctx, "task1")
	require.NoError(t, readErr)
	require.Equal(t, models.TaskEnvironmentStatusFailed, environment.Status)
	require.Empty(t, environment.MaterializationSessionID)
}
