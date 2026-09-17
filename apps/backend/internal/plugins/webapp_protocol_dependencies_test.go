package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestWebAppTaskFromSDK_MapsDependencyFields(t *testing.T) {
	task := pluginsdk.Task{
		ID: "task-1", Blocked: true, BlockedReason: taskservice.BlockedReasonPending,
		DependsOn:          []pluginsdk.TaskDependencyRef{{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending, WorkspaceID: "ws-1"}},
		Blocks:             []pluginsdk.TaskDependencyRef{{ID: "task-9", Title: "Dependent", State: "TODO", WorkspaceID: "ws-1"}},
		DependsOnTruncated: true,
		StartWhenUnblocked: true,
	}

	wire := webAppTaskFromSDK(task, "ws-1")

	require.True(t, wire.Blocked)
	require.Equal(t, taskservice.BlockedReasonPending, wire.BlockedReason)
	require.Equal(t, []webAppTaskDependencyRef{{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending}}, wire.DependsOn)
	require.Equal(t, []webAppTaskDependencyRef{{ID: "task-9", Title: "Dependent", State: "TODO"}}, wire.Blocks)
	require.True(t, wire.DependsOnTruncated)
	require.False(t, wire.BlocksTruncated)
	require.True(t, wire.StartWhenUnblocked)
}

func TestWebAppTaskDependencyRefFromSDK_RedactsOutOfScopeEdge(t *testing.T) {
	ref := pluginsdk.TaskDependencyRef{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending, WorkspaceID: "ws-other"}

	out := webAppTaskDependencyRefFromSDK(ref, "ws-1")

	require.Equal(t, "task-0", out.ID)
	require.Empty(t, out.Title, "title is redacted for an edge end outside the caller's scoped workspace")
	require.Empty(t, out.State, "state is redacted for an edge end outside the caller's scoped workspace")
	require.Equal(t, taskservice.DependencyPending, out.Status, "status is not scope-sensitive and is never redacted")
}

func TestWebAppTaskDependencyRefFromSDK_InstanceScopeCallerNeverRedacts(t *testing.T) {
	ref := pluginsdk.TaskDependencyRef{ID: "task-0", Title: "Predecessor", State: "TODO", WorkspaceID: "ws-other"}

	out := webAppTaskDependencyRefFromSDK(ref, "")

	require.Equal(t, "Predecessor", out.Title)
	require.Equal(t, "TODO", out.State)
}

func TestWebAppTaskDependencyRefsFromSDK_EmptyIsNeverNil(t *testing.T) {
	out := webAppTaskDependencyRefsFromSDK(nil, "ws-1")

	require.NotNil(t, out)
	require.Empty(t, out)
}

func TestListWebAppTasks_DerivesDependenciesAfterScopeNarrowing(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	inScope := &taskmodels.Task{
		ID: "task-in", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		Repositories: []*taskmodels.TaskRepository{{ID: "tr-1", RepositoryID: "repo-1"}},
	}
	outOfScope := &taskmodels.Task{
		ID: "task-out", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Repositories: []*taskmodels.TaskRepository{{ID: "tr-2", RepositoryID: "repo-2"}},
	}
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {inScope, outOfScope}}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-in": {Blocked: true, BlockedReason: taskservice.BlockedReasonPending},
	}

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeRepository, WorkspaceID: "ws-1", RepositoryID: "repo-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, []string{"task-in"}, d.tasks.dependencyViewsTasks, "dependency derivation runs only over tasks surviving the repository scope filter")
	require.Contains(t, recorder.Body.String(), `"blocked":true`)
}

func TestListWebAppTasks_FanOutRefusalBecomesResponseTooLarge(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	task := &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1"}
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {task}}
	d.tasks.dependencyViewsErr = taskservice.ErrDependencyFanOutExceeded

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks")

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "response_too_large")
}

func TestGetWebAppTask_PassesCallerWorkspaceForRedaction(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	task := &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1"}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": task}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {
			Blocked: true, BlockedReason: taskservice.BlockedReasonPending,
			DependsOn: []taskservice.DependencyRef{{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending}},
		},
	}

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeTask, WorkspaceID: "ws-1", TaskID: "task-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks/task-1")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"title":"Predecessor"`, "an in-scope edge end keeps its title")
}
