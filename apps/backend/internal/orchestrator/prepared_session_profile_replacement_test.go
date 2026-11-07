package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A prepared session whose runtime is bound to another execution profile is
// replaced in place: the lifecycle stop synchronously removes the runtime row
// and delivers agent.stopped, and the launch must still register its new
// execution on the same session.
func TestLaunchPreparedSession_ProfileReplacementKeepsSessionLaunchable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-replace", "session-replace", "step-replace")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-replace", models.TaskSessionStateCreated, ""))
	session, err := repo.GetTaskSession(ctx, "session-replace")
	require.NoError(t, err)
	session.AgentProfileID = "agent-prepared"
	session.ExecutionProfileID = "agent-prepared"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-replace", TaskID: session.TaskID, SessionID: session.ID,
		Status:             models.ExecutorRunningStatusPrepared,
		AgentExecutionID:   "execution-prepared",
		ExecutionProfileID: "agent-prepared",
	}))

	manager := &mockAgentManager{repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	task := &v1.Task{ID: session.TaskID, WorkspaceID: "ws1", Title: "Replace", State: v1.TaskStateInProgress}
	taskRepo.tasks[task.ID] = task
	svc := NewService(DefaultServiceConfig(), nil, manager, taskRepo, repo, nil, nil, nil, testLogger())
	manager.stopAgentWithReasonFunc = func(stopCtx context.Context, executionID, _ string, _ bool) error {
		// Mirrors the lifecycle manager: the runtime row is removed and the
		// stopped event is handled before StopAgentWithReason returns.
		if err := repo.DeleteExecutorRunningBySessionID(stopCtx, session.ID); err != nil {
			return err
		}
		svc.handleAgentStopped(stopCtx, watcher.AgentEventData{
			TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: executionID,
		})
		return nil
	}
	var launchedProfile string
	manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchedProfile = req.AgentProfileID
		return &executor.LaunchAgentResponse{AgentExecutionID: "execution-replacement"}, nil
	}

	_, err = svc.executor.LaunchPreparedSession(ctx, task, session.ID, executor.LaunchOptions{
		AgentProfileID: "profile-routed", Prompt: "run", StartAgent: true,
	})

	require.NoError(t, err)
	assert.Equal(t, "profile-routed", launchedProfile)
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.NotEqual(t, models.TaskSessionStateCancelled, current.State,
		"the profile-replacement stop must not end the session being relaunched")
}

func TestLaunchPreparedSession_ProfileReplacementPreservesIncomingTurn(t *testing.T) {
	for _, stopDelivery := range []struct {
		name    string
		delayed bool
	}{
		{name: "stop before replacement starts"},
		{name: "delayed stop after replacement starts", delayed: true},
	} {
		t.Run(stopDelivery.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-replace-turn", "session-replace-turn", "step-replace-turn")
			require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-replace-turn", models.TaskSessionStateCreated, ""))
			session, err := repo.GetTaskSession(ctx, "session-replace-turn")
			require.NoError(t, err)
			session.AgentProfileID = "agent-prepared"
			session.ExecutionProfileID = "agent-prepared"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "runtime-replace-turn", TaskID: session.TaskID, SessionID: session.ID,
				Status: models.ExecutorRunningStatusPrepared, AgentExecutionID: "execution-prepared-turn",
				ExecutionProfileID: "agent-prepared",
			}))
			turn := &models.Turn{
				ID: "turn-incoming", TaskID: session.TaskID, TaskSessionID: session.ID,
				StartedAt: time.Now().UTC(),
			}
			require.NoError(t, repo.CreateTurn(ctx, turn))

			manager := &mockAgentManager{repoForExecutionLookup: repo}
			taskRepo := newMockTaskRepo()
			task := &v1.Task{ID: session.TaskID, WorkspaceID: "ws1", Title: "Replace", State: v1.TaskStateInProgress}
			taskRepo.tasks[task.ID] = task
			svc := NewService(DefaultServiceConfig(), nil, manager, taskRepo, repo, nil, nil, nil, testLogger())
			svc.SetTurnService(&repoTurnService{repo: repo})
			manager.stopAgentWithReasonFunc = func(stopCtx context.Context, executionID, _ string, _ bool) error {
				if err := repo.DeleteExecutorRunningBySessionID(stopCtx, session.ID); err != nil {
					return err
				}
				if !stopDelivery.delayed {
					svc.handleAgentStopped(stopCtx, watcher.AgentEventData{
						TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: executionID,
					})
				}
				return nil
			}
			manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				assert.Equal(t, turn.ID, req.TurnID)
				return &executor.LaunchAgentResponse{AgentExecutionID: "execution-replacement-turn"}, nil
			}

			_, err = svc.executor.LaunchPreparedSession(ctx, task, session.ID, executor.LaunchOptions{
				AgentProfileID: "profile-routed", Prompt: "run", StartAgent: true, TurnID: turn.ID,
			})
			require.NoError(t, err)
			if stopDelivery.delayed {
				svc.handleAgentStopped(ctx, watcher.AgentEventData{
					TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: "execution-prepared-turn",
				})
			}

			activeTurn, err := svc.turnService.GetActiveTurn(ctx, session.ID)
			require.NoError(t, err)
			require.NotNil(t, activeTurn, "the old stop must leave the replacement's incoming turn open")
			assert.Equal(t, turn.ID, activeTurn.ID)

			svc.handleAgentCompleted(ctx, watcher.AgentEventData{
				TaskID: session.TaskID, SessionID: session.ID,
				AgentExecutionID: "execution-replacement-turn", TurnID: turn.ID,
			})
			completedTurn, err := repo.GetTurn(ctx, turn.ID)
			require.NoError(t, err)
			require.NotNil(t, completedTurn.CompletedAt, "the replacement's own completion must close its turn")
		})
	}
}

func TestLaunchPreparedSession_ProfileReplacementRequiresStopOwnership(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-replace-owner", "session-replace-owner", "step-replace-owner")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-replace-owner", models.TaskSessionStateCreated, ""))
	session, err := repo.GetTaskSession(ctx, "session-replace-owner")
	require.NoError(t, err)
	session.AgentProfileID = "agent-prepared"
	session.ExecutionProfileID = "agent-prepared"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-replace-owner", TaskID: session.TaskID, SessionID: session.ID,
		Status: models.ExecutorRunningStatusPrepared, AgentExecutionID: "execution-prepared-owner",
		ExecutionProfileID: "agent-prepared",
	}))

	var stopCalls, launchCalls int
	manager := &mockAgentManager{repoForExecutionLookup: repo}
	manager.stopAgentWithReasonFunc = func(context.Context, string, string, bool) error {
		stopCalls++
		return nil
	}
	manager.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchCalls++
		return &executor.LaunchAgentResponse{AgentExecutionID: "execution-replacement-owner"}, nil
	}
	taskRepo := newMockTaskRepo()
	task := &v1.Task{ID: session.TaskID, WorkspaceID: "ws1", Title: "Replace", State: v1.TaskStateInProgress}
	taskRepo.tasks[task.ID] = task
	svc := NewService(DefaultServiceConfig(), nil, manager, taskRepo, repo, nil, nil, nil, testLogger())

	guard, release := svc.acquireCancelInFlightGuard(session.ID)
	guard.Lock()
	_, err = svc.executor.LaunchPreparedSession(ctx, task, session.ID, executor.LaunchOptions{
		AgentProfileID: "profile-routed", Prompt: "run", StartAgent: true,
	})
	guard.Unlock()
	release()

	require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning,
		"profile replacement must wait when cancellation owns the session guard")
	assert.Zero(t, stopCalls, "replacement must not stop an execution without an ownership claim")
	assert.Zero(t, launchCalls, "replacement must not launch after ownership registration fails")
	running, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "execution-prepared-owner", running.AgentExecutionID)
}
