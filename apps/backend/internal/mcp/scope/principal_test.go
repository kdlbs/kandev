package scope

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type principalLookup struct {
	task      *models.Task
	workspace *models.Workspace
	session   *models.TaskSession
}

func (l principalLookup) GetTask(context.Context, string) (*models.Task, error) {
	return l.task, nil
}

func (l principalLookup) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return l.workspace, nil
}

func (l principalLookup) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return l.session, nil
}

func TestScopePrincipalDerivesAutomationIdentityFromExecution(t *testing.T) {
	resolver := &Resolver{tasks: principalLookup{
		task: &models.Task{
			ID:          "automation-task",
			WorkspaceID: "workspace-1",
			Origin:      models.TaskOriginAutomationRun,
			Metadata:    map[string]interface{}{"automation_id": "automation-1"},
		},
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: "automation-task"},
	}}

	ctx, err := resolver.ScopePrincipal(context.Background(), "automation-task", "session-1")
	require.NoError(t, err)

	principal, ok := PrincipalFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, Principal{
		AutomationID:    "automation-1",
		WorkspaceID:     "workspace-1",
		CallerTaskID:    "automation-task",
		CallerSessionID: "session-1",
		Surface:         mcpprofile.SurfaceAutomation,
	}, principal)
	require.True(t, principal.IsAutomation())
}

func TestScopePrincipalRejectsSessionFromAnotherTask(t *testing.T) {
	resolver := &Resolver{tasks: principalLookup{
		task:      &models.Task{ID: "automation-task", WorkspaceID: "workspace-1"},
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: "other-task"},
	}}

	_, err := resolver.ScopePrincipal(context.Background(), "automation-task", "session-1")
	require.Error(t, err)
}

func TestScopePrincipalDerivesProjectMainTask(t *testing.T) {
	tests := []struct {
		name string
		task *models.Task
	}{
		{
			name: "coordinator",
			task: &models.Task{
				ID: "coordinator-task", WorkspaceID: "workspace-1",
				AgentProjectID: "project-1", AgentProjectTier: models.AgentProjectTierCoordinator,
			},
		},
		{
			name: "worker",
			task: &models.Task{
				ID: "worker-task", WorkspaceID: "workspace-1", ParentID: "coordinator-task",
				AgentProjectID: "project-1", AgentProjectTier: models.AgentProjectTierEconomy,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &Resolver{tasks: principalLookup{
				task:      test.task,
				workspace: &models.Workspace{ID: "workspace-1"},
				session:   &models.TaskSession{ID: "session-1", TaskID: test.task.ID},
			}}
			ctx, err := resolver.ScopePrincipal(context.Background(), test.task.ID, "session-1")
			require.NoError(t, err)
			principal, ok := PrincipalFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, "coordinator-task", principal.ProjectMainTaskID)
		})
	}
}
