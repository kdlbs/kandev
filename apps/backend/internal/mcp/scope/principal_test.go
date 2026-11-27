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

// fakeCoordinatorLookup is a test double for CoordinatorLookup.
type fakeCoordinatorLookup struct {
	coordinatorID string
	ok            bool
	err           error
}

func (f fakeCoordinatorLookup) CoordinatorForConversationTask(context.Context, string) (string, bool, error) {
	return f.coordinatorID, f.ok, f.err
}

func coordinatorConversationLookup() principalLookup {
	return principalLookup{
		task: &models.Task{
			ID:          "conversation-task",
			WorkspaceID: "workspace-1",
			Origin:      models.TaskOriginCoordinator,
		},
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: "conversation-task"},
	}
}

func TestScopePrincipalDerivesCoordinatorIdentityFromExecution(t *testing.T) {
	resolver := &Resolver{
		tasks:        coordinatorConversationLookup(),
		coordinators: fakeCoordinatorLookup{coordinatorID: "coordinator-1", ok: true},
	}

	ctx, err := resolver.ScopePrincipal(context.Background(), "conversation-task", "session-1")
	require.NoError(t, err)

	principal, ok := PrincipalFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, Principal{
		WorkspaceID:     "workspace-1",
		CallerTaskID:    "conversation-task",
		CallerSessionID: "session-1",
		Surface:         mcpprofile.SurfaceCoordinator,
		CoordinatorID:   "coordinator-1",
	}, principal)
}

func TestScopePrincipalRefusesCoordinatorTaskWithNoLookupWired(t *testing.T) {
	resolver := &Resolver{tasks: coordinatorConversationLookup()}

	_, err := resolver.ScopePrincipal(context.Background(), "conversation-task", "session-1")
	require.Error(t, err)
}

func TestScopePrincipalRefusesOrphanedCoordinatorConversationTask(t *testing.T) {
	resolver := &Resolver{
		tasks:        coordinatorConversationLookup(),
		coordinators: fakeCoordinatorLookup{ok: false},
	}

	_, err := resolver.ScopePrincipal(context.Background(), "conversation-task", "session-1")
	require.Error(t, err)
}

func TestScopePrincipalRefusesOnCoordinatorLookupError(t *testing.T) {
	resolver := &Resolver{
		tasks:        coordinatorConversationLookup(),
		coordinators: fakeCoordinatorLookup{err: context.DeadlineExceeded},
	}

	_, err := resolver.ScopePrincipal(context.Background(), "conversation-task", "session-1")
	require.Error(t, err)
}
