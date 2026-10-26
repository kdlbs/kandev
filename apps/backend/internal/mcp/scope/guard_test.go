package scope

import (
	"context"
	"errors"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type guardStub struct {
	guarded bool
	err     error
	gotWS   string
	gotTask string
}

func (g *guardStub) TaskGuarded(_ context.Context, workspaceID, taskID string) (bool, error) {
	g.gotWS, g.gotTask = workspaceID, taskID
	return g.guarded, g.err
}

func guardResolver(task *models.Task, src GuardSource) *Resolver {
	r := &Resolver{tasks: principalLookup{
		task:      task,
		workspace: &models.Workspace{ID: "workspace-1"},
		session:   &models.TaskSession{ID: "session-1", TaskID: task.ID},
	}}
	r.SetGuardSource(src)
	return r
}

func TestScopePrincipalResolvesTaskGuard(t *testing.T) {
	task := &models.Task{ID: "task-1", WorkspaceID: "workspace-1"}
	stub := &guardStub{guarded: true}
	ctx, err := guardResolver(task, stub).ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	principal, ok := PrincipalFromContext(ctx)
	require.True(t, ok)
	require.True(t, principal.IsGuarded())
	require.Equal(t, "workspace-1", stub.gotWS)
	require.Equal(t, "task-1", stub.gotTask)

	ctx, err = guardResolver(task, &guardStub{}).ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	principal, _ = PrincipalFromContext(ctx)
	require.False(t, principal.IsGuarded())
}

func TestScopePrincipalGuardReadFailureDeniesDispatch(t *testing.T) {
	task := &models.Task{ID: "task-1", WorkspaceID: "workspace-1"}
	ctx, err := guardResolver(task, &guardStub{err: errors.New("db down")}).ScopePrincipal(context.Background(), "task-1", "session-1")
	require.ErrorIs(t, err, ErrGuardUnavailable)
	require.Nil(t, ctx)
}

func TestScopePrincipalWithoutGuardSourceIsUnguarded(t *testing.T) {
	task := &models.Task{ID: "task-1", WorkspaceID: "workspace-1"}
	ctx, err := guardResolver(task, nil).ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	principal, _ := PrincipalFromContext(ctx)
	require.False(t, principal.IsGuarded())
}

func TestGuardIsNotConsultedForAutomationTasks(t *testing.T) {
	task := &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1", Origin: models.TaskOriginAutomationRun,
		Metadata: map[string]interface{}{"automation_id": "a-1"},
	}
	stub := &guardStub{guarded: true}
	ctx, err := guardResolver(task, stub).ScopePrincipal(context.Background(), "task-1", "session-1")
	require.NoError(t, err)
	principal, _ := PrincipalFromContext(ctx)
	require.Equal(t, mcpprofile.SurfaceAutomation, principal.Surface)
	require.False(t, principal.IsGuarded())
	require.Empty(t, stub.gotTask)
}
