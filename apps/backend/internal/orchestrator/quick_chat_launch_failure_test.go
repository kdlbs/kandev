package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

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
