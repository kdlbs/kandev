package scope

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type principalLookup struct {
	task      *models.Task
	workspace *models.Workspace
	session   *models.TaskSession
}

type lifecyclePrincipalLookup struct {
	principalLookup
	principal *models.WorkspaceAgentPrincipal
}

func (l *lifecyclePrincipalLookup) GetActiveWorkspaceAgentPrincipalForTask(context.Context, string, string) (*models.WorkspaceAgentPrincipal, error) {
	if l.principal == nil || l.principal.RevokedAt != nil {
		return nil, nil
	}
	return l.principal, nil
}

func (l *lifecyclePrincipalLookup) GetWorkspaceAgentPrincipalByContext(context.Context, string, string, string) (*models.WorkspaceAgentPrincipal, error) {
	if l.principal == nil {
		return nil, repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	return l.principal, nil
}

func (l *lifecyclePrincipalLookup) CreateWorkspaceAgentPrincipal(_ context.Context, principal *models.WorkspaceAgentPrincipal) error {
	principal.ID = "principal-1"
	l.principal = principal
	return nil
}

func (l *lifecyclePrincipalLookup) RebindWorkspaceAgentPrincipal(_ context.Context, id, taskID, sessionID string, _ time.Time) error {
	if l.principal == nil || l.principal.ID != id {
		return repoerrors.ErrWorkspaceAgentPrincipalNotFound
	}
	l.principal.BackingTaskID = taskID
	l.principal.BackingSessionID = sessionID
	return nil
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

func TestScopePrincipalRegistersAndBindsNormalTask(t *testing.T) {
	lookup := &lifecyclePrincipalLookup{principalLookup: principalLookup{
		task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: "task-1"},
	}}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	require.NotNil(t, lookup.principal)
	require.Equal(t, coordinator.TaskPrincipalInstallationID, lookup.principal.PluginInstallationID)
	require.Equal(t, coordinator.TaskPrincipalLogicalKey("task-1"), lookup.principal.LogicalKey)
	require.Equal(t, "task-1", lookup.principal.BackingTaskID)
	require.Equal(t, "session-1", lookup.principal.BackingSessionID)
}

func TestScopePrincipalDoesNotResurrectRevokedTaskPrincipal(t *testing.T) {
	revokedAt := time.Now().UTC()
	lookup := &lifecyclePrincipalLookup{
		principalLookup: principalLookup{
			task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspace: &models.Workspace{ID: "workspace-1"},
			session:   &models.TaskSession{ID: "session-1", TaskID: "task-1"},
		},
		principal: &models.WorkspaceAgentPrincipal{
			ID: "principal-1", WorkspaceID: "workspace-1",
			PluginInstallationID: coordinator.TaskPrincipalInstallationID,
			LogicalKey:           coordinator.TaskPrincipalLogicalKey("task-1"),
			BackingTaskID:        "task-1", RevokedAt: &revokedAt,
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
	require.Error(t, err)
	require.True(t, lookup.principal.RevokedAt.Equal(revokedAt))
}

func TestScopePrincipalUsesExistingCustomTaskPrincipal(t *testing.T) {
	lookup := &lifecyclePrincipalLookup{
		principalLookup: principalLookup{
			task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspace: &models.Workspace{ID: "workspace-1"},
			session:   &models.TaskSession{ID: "session-1", TaskID: "task-1"},
		},
		principal: &models.WorkspaceAgentPrincipal{
			ID:                   "custom-principal",
			WorkspaceID:          "workspace-1",
			PluginInstallationID: "plugin-1",
			LogicalKey:           "custom-key",
			BackingTaskID:        "task-1",
			BackingSessionID:     "session-1",
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	require.Equal(t, "custom-principal", lookup.principal.ID)
}

func TestScopePrincipalRejectsExistingCustomTaskPrincipalOnAnotherSession(t *testing.T) {
	lookup := &lifecyclePrincipalLookup{
		principalLookup: principalLookup{
			task:      &models.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			workspace: &models.Workspace{ID: "workspace-1"},
			session:   &models.TaskSession{ID: "session-2", TaskID: "task-1"},
		},
		principal: &models.WorkspaceAgentPrincipal{
			ID:                   "custom-principal",
			WorkspaceID:          "workspace-1",
			PluginInstallationID: "plugin-1",
			LogicalKey:           "custom-key",
			BackingTaskID:        "task-1",
			BackingSessionID:     "session-1",
		},
	}
	resolver := &Resolver{tasks: lookup}

	_, err := resolver.ScopePrincipal(context.Background(), "task-1", "session-2")
	require.Error(t, err)
	require.Equal(t, "session-1", lookup.principal.BackingSessionID)
}
