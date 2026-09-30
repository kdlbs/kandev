package handlers

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type selfPlacementReadFailureRepo struct {
	*sqliterepo.Repository
	parentID string
	err      error
}

func (r selfPlacementReadFailureRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if id == r.parentID {
		return nil, r.err
	}
	return r.Repository.GetTask(ctx, id)
}

// @covers AC-TASKS-SELF-SIBLING-001.3 AC-TASKS-SELF-SIBLING-002.3
func TestMCPCreateTaskSelfPlacementParentReadFailure(t *testing.T) {
	f := newSelfPlacementFixture(t)
	f.h.taskSvc = service.NewService(service.Repos{
		Tasks: selfPlacementReadFailureRepo{
			Repository: f.repo, parentID: f.parent.ID, err: errors.New("private database endpoint failed"),
		},
		Workspaces: f.repo, Workflows: f.repo, Sessions: f.repo,
		TaskRepos: f.repo, WorkspaceFolders: f.repo,
	}, nil, testLogger(t), service.RepositoryDiscoveryConfig{})
	resp, err := f.h.handleCreateTask(f.caller(), makeWSMessage(t, ws.ActionMCPCreateTask,
		selfPlacementCreatePayload(f, f.child.ID)))
	require.NoError(t, err)
	failure := errorPayload(t, resp)
	require.Equal(t, ws.ErrorCodeInternalError, failure.Code)
	require.NotContains(t, failure.Message, "private database endpoint")
	require.NotContains(t, string(resp.Payload), "parent_resolution")
	tasks, err := f.svc.ListTasks(f.ctx, f.workflow.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
}

// @covers AC-TASKS-SELF-SIBLING-001.3 AC-TASKS-SELF-SIBLING-002.4
func TestMCPCreateTaskSelfPlacementEphemeralCaller(t *testing.T) {
	f := newSelfPlacementFixture(t)
	_, err := f.repo.DB().ExecContext(f.ctx, "UPDATE tasks SET is_ephemeral = 1 WHERE id = ?", f.child.ID)
	require.NoError(t, err)
	resp, err := f.h.handleCreateTask(f.caller(), makeWSMessage(t, ws.ActionMCPCreateTask,
		selfPlacementCreatePayload(f, f.child.ID)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	require.NotContains(t, string(resp.Payload), "parent_resolution")
	require.Equal(t, 2, f.taskCount(t))
}

// Test-only contract coverage for existing rejection paths after redirection.
// @covers AC-TASKS-SELF-SIBLING-001.3 AC-TASKS-SELF-SIBLING-002.3 AC-TASKS-SELF-SIBLING-002.4
func TestMCPCreateTaskSelfPlacementAdmission(t *testing.T) {
	for _, tc := range []struct{ name, code string }{
		{"missing parent", ws.ErrorCodeNotFound},
		{"cross workspace parent", ws.ErrorCodeNotFound},
		{"inaccessible parent", ws.ErrorCodeNotFound},
		{"ephemeral parent", ws.ErrorCodeValidation},
		{"nested parent", ws.ErrorCodeValidation},
		{"office parent", ws.ErrorCodeForbidden},
		{"office caller", ws.ErrorCodeForbidden},
		{"foreign source task", ws.ErrorCodeForbidden},
		{"foreign source session", ws.ErrorCodeForbidden},
		{"session task mismatch", ws.ErrorCodeUnauthorized},
		{"workspace mismatch", ws.ErrorCodeValidation},
		{"workflow mismatch", ws.ErrorCodeValidation},
		{"read only destination", ws.ErrorCodeForbidden},
		{"untrusted caller", ws.ErrorCodeUnauthorized},
		{"conflicting identity", ws.ErrorCodeUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newSelfPlacementFixture(t)
				payload := selfPlacementCreatePayload(f, f.child.ID)
				payload["start_agent"], payload["description"] = true, "Must not launch"
				ctx := prepareSelfPlacementRejection(t, f, payload, tc.name)
				before := f.taskCount(t)
				launcher := newMockSessionLauncher()
				f.h.sessionLauncher = launcher
				resp := f.create(t, ctx, payload)
				failure := errorPayload(t, resp)
				require.Equal(t, tc.code, failure.Code)
				require.NotContains(t, string(resp.Payload), "parent_resolution")
				if tc.code == ws.ErrorCodeNotFound {
					require.Equal(t, "target parent task not found", failure.Message)
				}
				require.Equal(t, before, f.taskCount(t))
				synctest.Wait()
				require.Nil(t, launcher.getRequest())
			})
		})
	}
}

func prepareSelfPlacementRejection(t *testing.T, f selfPlacementFixture, payload map[string]interface{}, name string) context.Context {
	t.Helper()
	ctx := f.caller()
	switch name {
	case "missing parent":
		_, err := f.repo.DB().ExecContext(f.ctx, "UPDATE tasks SET parent_id = ? WHERE id = ?", "missing-parent", f.child.ID)
		require.NoError(t, err)
	case "cross workspace parent", "inaccessible parent":
		other := &models.Workspace{ID: "private-workspace", Name: "Private", OwnerID: "another-user"}
		require.NoError(t, f.repo.CreateWorkspace(f.ctx, other))
		_, err := f.repo.DB().ExecContext(f.ctx, "UPDATE tasks SET workspace_id = ? WHERE id = ?", other.ID, f.parent.ID)
		require.NoError(t, err)
		if name == "inaccessible parent" {
			ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "caller", Role: authn.RoleMember})
		}
	case "ephemeral parent":
		_, err := f.repo.DB().ExecContext(f.ctx, "UPDATE tasks SET is_ephemeral = 1 WHERE id = ?", f.parent.ID)
		require.NoError(t, err)
	case "nested parent":
		grandparent := &models.Task{WorkspaceID: f.workspace.ID, WorkflowID: f.workflow.ID, Title: "Grandparent"}
		require.NoError(t, f.repo.CreateTask(f.ctx, grandparent))
		_, err := f.repo.DB().ExecContext(f.ctx, "UPDATE tasks SET parent_id = ? WHERE id = ?", grandparent.ID, f.parent.ID)
		require.NoError(t, err)
	case "office parent":
		_, err := f.repo.DB().ExecContext(f.ctx, "UPDATE tasks SET project_id = ? WHERE id = ?", "office-project", f.parent.ID)
		require.NoError(t, err)
	case "office caller":
		ctx = mcpscope.WithPrincipal(f.ctx, mcpscope.Principal{Surface: mcpprofile.SurfaceOfficeTask,
			WorkspaceID: f.workspace.ID, CallerTaskID: f.child.ID, CallerSessionID: "self-placement-session"})
	case "foreign source task":
		payload["source_task_id"] = f.parent.ID
	case "foreign source session":
		payload["source_session_id"] = "another-session"
	case "session task mismatch":
		ctx = mcpTestKanbanContext(f.ctx, f.workspace.ID, f.parent.ID, "self-placement-session")
		payload["parent_id"] = f.parent.ID
	case "workspace mismatch":
		payload["workspace_id"] = "other-workspace"
	case "workflow mismatch":
		other := &models.Workspace{ID: "other-workspace", Name: "Other"}
		require.NoError(t, f.repo.CreateWorkspace(f.ctx, other))
		workflow := &models.Workflow{ID: "other-workflow", WorkspaceID: other.ID, Name: "Other"}
		require.NoError(t, f.repo.CreateWorkflow(f.ctx, workflow))
		payload["workflow_id"] = workflow.ID
	case "read only destination":
		_, err := f.repo.DB().ExecContext(f.ctx, "UPDATE workspaces SET owner_id = ? WHERE id = ?", "another-user", f.workspace.ID)
		require.NoError(t, err)
		require.NoError(t, f.repo.UpsertWorkspaceMember(f.ctx, &models.WorkspaceMember{
			WorkspaceID: f.workspace.ID, UserID: "viewer", Role: string(authz.WorkspaceRoleViewer),
		}))
		ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "viewer", Role: authn.RoleMember})
		require.True(t, service.IsForbidden(f.svc.AuthorizeWorkspaceScope(ctx, f.workspace.ID, authz.ScopeTaskWrite)))
	case "untrusted caller":
		ctx = f.ctx
	case "conflicting identity":
		ctx = mcpTestExternalContext(ctx)
	default:
		t.Fatalf("unknown rejection scenario %q", name)
	}
	return ctx
}
