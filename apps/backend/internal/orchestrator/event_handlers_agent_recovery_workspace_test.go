package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005.3
func TestSessionFailureMetadataWorkspaceRestorable(t *testing.T) {
	trueValue := true
	falseValue := false
	tests := []struct {
		name      string
		status    models.TaskEnvironmentStatus
		path      string
		readErr   error
		emptyPath bool
		eventTask string
		envTask   string
		want      *bool
	}{
		{name: "ready workspace", status: models.TaskEnvironmentStatusReady, path: "/work", want: &trueValue},
		{name: "stopped workspace", status: models.TaskEnvironmentStatusStopped, path: "/work", want: &trueValue},
		{name: "empty status workspace", path: "/work", want: &trueValue},
		{name: "creating workspace", status: models.TaskEnvironmentStatusCreating, path: "/work", want: &falseValue},
		{name: "failed workspace", status: models.TaskEnvironmentStatusFailed, path: "/work", want: &falseValue},
		{name: "missing workspace path", status: models.TaskEnvironmentStatusReady, emptyPath: true, want: &falseValue},
		{name: "session belongs to another task", status: models.TaskEnvironmentStatusReady, path: "/work", eventTask: "task-other"},
		{name: "environment belongs to another task", status: models.TaskEnvironmentStatusReady, path: "/work", envTask: "task-other"},
		{name: "environment read error", status: models.TaskEnvironmentStatusReady, path: "/work", readErr: errors.New("environment read failed")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task-workspace", "session-workspace", "step1")
			workspacePath := tc.path
			if workspacePath == "" {
				workspacePath = "/work"
			}
			environmentTaskID := tc.envTask
			if environmentTaskID == "" {
				environmentTaskID = "task-workspace"
			}
			if tc.eventTask == "task-other" || environmentTaskID == "task-other" {
				now := time.Now()
				require.NoError(t, repo.CreateTask(ctx, &models.Task{
					ID: "task-other", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Other task",
					State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
				}))
			}
			env := &models.TaskEnvironment{
				ID: "environment-workspace", TaskID: environmentTaskID, ExecutorType: "worktree",
				Status: tc.status, WorkspacePath: workspacePath, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}
			require.NoError(t, repo.CreateTaskEnvironment(ctx, env))
			session, err := repo.GetTaskSession(ctx, "session-workspace")
			require.NoError(t, err)
			session.TaskEnvironmentID = env.ID
			require.NoError(t, repo.UpdateTaskSession(ctx, session))

			svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{repoForExecutionLookup: repo})
			creator := &mockMessageCreator{}
			svc.messageCreator = creator
			if tc.readErr != nil || tc.emptyPath {
				svc.repo = &recoveryWorkspaceTestRepository{
					Repository: repo, err: tc.readErr, emptyPath: tc.emptyPath,
				}
			}

			eventTaskID := tc.eventTask
			if eventTaskID == "" {
				eventTaskID = "task-workspace"
			}
			require.NoError(t, svc.createRecoveryStatusMessage(ctx, watcherDataForRecoveryWorkspace(eventTaskID), ""))
			require.Len(t, creator.sessionMessages, 1)
			value, exists := creator.sessionMessages[0].metadata["workspace_restorable"]
			if tc.want == nil {
				require.False(t, exists)
				return
			}
			require.True(t, exists)
			require.Equal(t, *tc.want, value)
		})
	}
}

type recoveryWorkspaceTestRepository struct {
	*sqliterepo.Repository
	err       error
	emptyPath bool
}

func (r *recoveryWorkspaceTestRepository) GetTaskEnvironment(
	ctx context.Context,
	id string,
) (*models.TaskEnvironment, error) {
	if r.err != nil {
		return nil, r.err
	}
	environment, err := r.Repository.GetTaskEnvironment(ctx, id)
	if err != nil || environment == nil || !r.emptyPath {
		return environment, err
	}
	environment.WorkspacePath = ""
	return environment, nil
}

func watcherDataForRecoveryWorkspace(taskID string) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: taskID, SessionID: "session-workspace", ErrorMessage: "worktree setup failed",
	}
}
