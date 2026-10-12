package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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

	wire := webAppTaskFromSDK(task, dependencyRedactionScope{kind: instances.ScopeWorkspace, workspaceID: "ws-1"})

	require.True(t, wire.Blocked)
	require.Equal(t, taskservice.BlockedReasonPending, wire.BlockedReason)
	require.Equal(t, []webAppTaskDependencyRef{{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending}}, wire.DependsOn)
	require.Equal(t, []webAppTaskDependencyRef{{ID: "task-9", Title: "Dependent", State: "TODO"}}, wire.Blocks)
	require.True(t, wire.DependsOnTruncated)
	require.False(t, wire.BlocksTruncated)
	require.True(t, wire.StartWhenUnblocked)
}

// TestWebAppTaskFromSDK_BlockedReasonKeyAlwaysPresentEvenWhenEmpty guards the
// presence rule for the not-blocked case, which none of the other dependency
// fixtures exercises (they all set Blocked: true). blocked_reason is "" on an
// unblocked task, so an omitempty tag would silently drop the key from the
// wire response for every unblocked task -- the majority case.
func TestWebAppTaskFromSDK_BlockedReasonKeyAlwaysPresentEvenWhenEmpty(t *testing.T) {
	task := pluginsdk.Task{ID: "task-1", Blocked: false, BlockedReason: ""}

	wire := webAppTaskFromSDK(task, dependencyRedactionScope{kind: instances.ScopeInstance})

	raw, err := json.Marshal(wire)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	value, present := decoded["blocked_reason"]
	require.True(t, present, "blocked_reason key must always be present, never omitted, even when empty")
	require.Equal(t, "", value)
}

// TestWebAppTaskDependencyRefFromSDK_RedactedTitleAndStateKeysAlwaysPresent
// guards Review round 2's F-B: a redacted entry blanks title/state to "",
// which an omitempty tag would silently drop instead of serializing.
func TestWebAppTaskDependencyRefFromSDK_RedactedTitleAndStateKeysAlwaysPresent(t *testing.T) {
	ref := pluginsdk.TaskDependencyRef{ID: "task-0", Title: "Predecessor", State: "TODO", WorkspaceID: "ws-other"}

	out := webAppTaskDependencyRefFromSDK(ref, dependencyRedactionScope{kind: instances.ScopeWorkspace, workspaceID: "ws-1"})

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	title, titlePresent := decoded["title"]
	require.True(t, titlePresent, "title key must always be present, never omitted, even when redacted to empty")
	require.Equal(t, "", title)
	state, statePresent := decoded["state"]
	require.True(t, statePresent, "state key must always be present, never omitted, even when redacted to empty")
	require.Equal(t, "", state)
}

func TestWebAppTaskDependencyRefFromSDK_RedactsOutOfScopeEdge(t *testing.T) {
	ref := pluginsdk.TaskDependencyRef{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending, WorkspaceID: "ws-other"}

	out := webAppTaskDependencyRefFromSDK(ref, dependencyRedactionScope{kind: instances.ScopeWorkspace, workspaceID: "ws-1"})

	require.Equal(t, "task-0", out.ID)
	require.Empty(t, out.Title, "title is redacted for an edge end outside the caller's scoped workspace")
	require.Empty(t, out.State, "state is redacted for an edge end outside the caller's scoped workspace")
	require.Equal(t, taskservice.DependencyPending, out.Status, "status is not scope-sensitive and is never redacted")
}

func TestWebAppTaskDependencyRefFromSDK_InstanceScopeCallerNeverRedacts(t *testing.T) {
	ref := pluginsdk.TaskDependencyRef{ID: "task-0", Title: "Predecessor", State: "TODO", WorkspaceID: "ws-other"}

	out := webAppTaskDependencyRefFromSDK(ref, dependencyRedactionScope{kind: instances.ScopeInstance})

	require.Equal(t, "Predecessor", out.Title)
	require.Equal(t, "TODO", out.State)
}

func TestWebAppTaskDependencyRefsFromSDK_EmptyIsNeverNil(t *testing.T) {
	out := webAppTaskDependencyRefsFromSDK(nil, dependencyRedactionScope{kind: instances.ScopeWorkspace, workspaceID: "ws-1"})

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

func TestListWebAppTasks_TaskPlacementUsesWorkspaceDataScopeAndPagination(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}, {ID: "ws-2"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{
		"ws-1": {
			{ID: "task-a", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)},
			{ID: "task-b", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		},
		"ws-2": {{ID: "foreign", WorkspaceID: "ws-2", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}
	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeTask, DataScopeKind: instances.ScopeWorkspace,
		WorkspaceID: "ws-1", TaskID: "task-a", Permissions: []string{"api_read:tasks"},
	}

	first := httptest.NewRecorder()
	svc.handleWebAppProtocol(first, httptest.NewRequest(http.MethodGet, "/?limit=1", nil), "", binding, "v1/data/tasks")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var firstPage webAppPage[webAppTask]
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstPage))
	require.Len(t, firstPage.Items, 1)
	require.Equal(t, "task-a", firstPage.Items[0].ID)
	require.True(t, firstPage.PageInfo.HasMore)
	require.NotEmpty(t, firstPage.PageInfo.NextCursor)

	second := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodGet, "/?limit=1&cursor="+firstPage.PageInfo.NextCursor, nil)
	svc.handleWebAppProtocol(second, secondRequest, "", binding, "v1/data/tasks")
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	var secondPage webAppPage[webAppTask]
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondPage))
	require.Len(t, secondPage.Items, 1)
	require.Equal(t, "task-b", secondPage.Items[0].ID)

	foreign := httptest.NewRecorder()
	foreignRequest := httptest.NewRequest(http.MethodGet, "/?workspace_id=ws-2", nil)
	svc.handleWebAppProtocol(foreign, foreignRequest, "", binding, "v1/data/tasks")
	require.Equal(t, http.StatusForbidden, foreign.Code)
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

// TestGetWebAppTask_TaskScopeRedactsEveryEdgeEnd covers
// AC-PLUGINS-TASK-DEPS-003.8's task-scope rule: task scope admits no end,
// because an end is never the bound task itself. A task-scoped canvas may
// see that its task is blocked, but never the title of what blocks it --
// even when the end's workspace equals the bound one.
func TestGetWebAppTask_TaskScopeRedactsEveryEdgeEnd(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	task := &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1"}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": task}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {
			Blocked: true, BlockedReason: taskservice.BlockedReasonPending,
			DependsOn: []taskservice.DependencyRef{{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending, WorkspaceID: "ws-1"}},
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
	require.NotContains(t, recorder.Body.String(), `"title":"Predecessor"`, "task scope admits no edge end, even one sharing the bound workspace")
	require.Contains(t, recorder.Body.String(), `"id":"task-0"`, "the entry is kept, only its title and state are redacted")
}

// TestGetWebAppTask_WorkspaceScopeAdmitsEdgeEndInSameWorkspace covers
// AC-PLUGINS-TASK-DEPS-003.8's workspace-scope rule: workspace scope admits
// an end whose workspace is known equal to the bound workspace.
func TestGetWebAppTask_WorkspaceScopeAdmitsEdgeEndInSameWorkspace(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	task := &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1"}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": task}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-1": {
			Blocked: true, BlockedReason: taskservice.BlockedReasonPending,
			DependsOn: []taskservice.DependencyRef{{ID: "task-0", Title: "Predecessor", State: "TODO", Status: taskservice.DependencyPending, WorkspaceID: "ws-1"}},
		},
	}

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks/task-1")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"title":"Predecessor"`, "an edge end sharing the bound workspace keeps its title")
}

// TestGetWebAppTask_WorkspaceScopeFailsClosedOnUnknownEdgeWorkspace covers
// AC-PLUGINS-TASK-DEPS-003.3: an end whose predicate inputs are unavailable
// (here, an edge end with no recorded workspace) is treated as not admitted.
func TestGetWebAppTask_WorkspaceScopeFailsClosedOnUnknownEdgeWorkspace(t *testing.T) {
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
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks/task-1")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), `"title":"Predecessor"`, "an edge end with no recorded workspace fails closed, not open")
}

// TestListWebAppTasks_RepositoryScopeRedactsEdgeEndNotReturnedInSameResponse
// covers AC-PLUGINS-TASK-DEPS-003.8's repository/session rule: such scopes do
// not admit by workspace equality alone -- an end filtered out of this
// response by the repository scope stays redacted even though it shares the
// bound workspace.
func TestListWebAppTasks_RepositoryScopeRedactsEdgeEndNotReturnedInSameResponse(t *testing.T) {
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
		"task-in": {
			Blocked: true, BlockedReason: taskservice.BlockedReasonPending,
			DependsOn: []taskservice.DependencyRef{
				{ID: "task-out", Title: "Same workspace, different repository", State: "TODO", Status: taskservice.DependencyPending, WorkspaceID: "ws-1"},
			},
		},
	}

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeRepository, WorkspaceID: "ws-1", RepositoryID: "repo-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), `"title":"Same workspace, different repository"`,
		"repository scope does not admit by workspace equality; task-out was filtered out of this response by the repository scope, so it is not a directly readable task here")
}

// TestListWebAppTasks_RepositoryScopeAdmitsEdgeEndReturnedInSameResponse
// covers the positive side of the same rule: when the edge's far end DOES
// survive the repository scope filter and is returned in the same response,
// it is a directly readable task and keeps its title/state.
func TestListWebAppTasks_RepositoryScopeAdmitsEdgeEndReturnedInSameResponse(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	dependent := &taskmodels.Task{
		ID: "task-dependent", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		Repositories: []*taskmodels.TaskRepository{{ID: "tr-1", RepositoryID: "repo-1"}},
	}
	predecessor := &taskmodels.Task{
		ID: "task-predecessor", WorkspaceID: "ws-1", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Repositories: []*taskmodels.TaskRepository{{ID: "tr-1", RepositoryID: "repo-1"}},
	}
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {dependent, predecessor}}
	d.tasks.dependencyViews = map[string]taskservice.DependencyView{
		"task-dependent": {
			Blocked: true, BlockedReason: taskservice.BlockedReasonPending,
			DependsOn: []taskservice.DependencyRef{
				{ID: "task-predecessor", Title: "Same repository, in this response", State: "TODO", Status: taskservice.DependencyPending, WorkspaceID: "ws-1"},
			},
		},
	}

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeRepository, WorkspaceID: "ws-1", RepositoryID: "repo-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"title":"Same repository, in this response"`,
		"task-predecessor survived the repository scope filter and is a directly readable task in this response, so its title is admitted")
}

// ── F1: a scope-check preflight that discards the task must never pay for
// dependency derivation, because it never serializes the task it fetched ──

func TestUpdateWebAppTask_ScopeCheckPreflightDoesNotDeriveDependencies(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}, APIWrite: []string{"tasks"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1"}}
	d.taskWriter.updated = &taskmodels.Task{ID: "task-1", Title: "new title"}

	svc := &Service{}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks", "api_write:tasks"},
	}
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"title":"new title"}`))
	recorder := httptest.NewRecorder()

	svc.updateWebAppTask(context.Background(), recorder, req, d.host, binding, "task-1")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, d.tasks.dependencyViewsCalls,
		"derivation runs once, for the updated task actually serialized -- not a second time for the discarded scope-check preflight")
}

// TestUpdateWebAppTask_CombinedTitleAndStepPatchDerivesOnce covers the design's
// Placement section ("the PATCH route derives once ... one response, one
// derivation, on the object serialized") for a body that takes both the
// Update and the Move write branch: only Move's result is ever serialized, so
// the Update branch's write must not derive on the intermediate result nobody
// sees.
func TestUpdateWebAppTask_CombinedTitleAndStepPatchDerivesOnce(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}, APIWrite: []string{"tasks"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1"}}
	d.taskWriter.updated = &taskmodels.Task{ID: "task-1", Title: "new title", WorkspaceID: "ws-1"}
	d.taskWriter.moveResult = &TaskMoveResult{
		Task: &taskmodels.Task{ID: "task-1", WorkflowStepID: "step-2", WorkspaceID: "ws-1"}, Transitioned: true,
	}

	svc := &Service{}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks", "api_write:tasks"},
	}
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"title":"new title","workflow_step_id":"step-2"}`))
	recorder := httptest.NewRecorder()

	svc.updateWebAppTask(context.Background(), recorder, req, d.host, binding, "task-1")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 1, d.taskWriter.updateCalls, "the update branch still writes the task fields")
	require.Equal(t, 1, d.taskWriter.moveCalls, "the move branch still writes the step transition")
	require.Equal(t, 1, d.tasks.dependencyViewsCalls,
		"a body naming both a title and a workflow step takes both write branches, but only Move's result is serialized -- derivation must run exactly once, not once per write")
}

func TestSendWebAppMessage_ScopeCheckPreflightDoesNotDeriveDependencies(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}, APIWrite: []string{"messages"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1"}}

	svc := &Service{}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks", "api_write:messages"},
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"text":"hello"}`))
	recorder := httptest.NewRecorder()

	svc.sendWebAppMessage(context.Background(), recorder, req, d.host, binding, "task-1")

	require.Equal(t, http.StatusAccepted, recorder.Code, recorder.Body.String())
	require.Equal(t, 0, d.tasks.dependencyViewsCalls)
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

func TestListWebAppWorkflows_TaskScopeCheckPreflightDoesNotDeriveDependencies(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks", "workflows"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1"}}
	d.workflows.workflows = map[string][]*taskmodels.Workflow{"ws-1": {{ID: "wf-1", WorkspaceID: "ws-1"}}}

	svc := &Service{}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeTask, WorkspaceID: "ws-1", TaskID: "task-1",
		Permissions: []string{"api_read:tasks", "api_read:workflows"},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	svc.listWebAppWorkflows(context.Background(), recorder, req, d.host, binding)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 0, d.tasks.dependencyViewsCalls)
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

func TestListWebAppWorkflowSteps_TaskScopeCheckPreflightDoesNotDeriveDependencies(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks", "workflows"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1"}}
	d.workflows.workflows = map[string][]*taskmodels.Workflow{"ws-1": {{ID: "wf-1", WorkspaceID: "ws-1"}}}
	d.steps.steps = map[string][]*wfmodels.WorkflowStep{"wf-1": {{ID: "step-1", WorkflowID: "wf-1"}}}

	svc := &Service{}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeTask, WorkspaceID: "ws-1", TaskID: "task-1",
		Permissions: []string{"api_read:tasks", "api_read:workflows"},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	svc.listWebAppWorkflowSteps(context.Background(), recorder, req, d.host, binding, "wf-1")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, 0, d.tasks.dependencyViewsCalls)
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

// ── Review round 2, Finding #1/#2: a scope check that can discard the task
// must run before dependency derivation on the canvas single-task read and
// the list route's task-scope branch, matching the rule the non-task-scope
// branch already documents for itself ───────────────────────────────────

func TestGetWebAppTask_ScopeCheckDiscardDoesNotDeriveDependencies(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-other"}}

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks/task-1")

	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	require.Equal(t, 0, d.tasks.dependencyViewsCalls,
		"a task the scope check discards is never serialized, so it must not pay for dependency derivation")
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

func TestListWebAppTasks_TaskScopeDiscardDoesNotDeriveDependencies(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-other"}}

	svc := &Service{taskData: d.tasks}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeTask, WorkspaceID: "ws-1", TaskID: "task-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks")

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), `"items":[]`)
	require.Equal(t, 0, d.tasks.dependencyViewsCalls,
		"a task the scope check discards is never serialized, so the list route's task-scope branch must not pay for dependency derivation")
	require.Equal(t, 0, d.tasks.dependencyViewsBoundedCalls)
}

// ── listWebAppTasks's non-task-scope branch must guard a nil task data
// source the same way the gRPC taskReader.List path already does, rather
// than panicking inside resolveWorkspaceIDs ─────────────────────────────

func TestListWebAppTasks_NonTaskScopeReturnsUnimplementedWhenTaskDataSourceIsNil(t *testing.T) {
	svc := &Service{}
	binding := webapp.CapabilityBinding{
		ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1",
		Permissions: []string{"api_read:tasks"},
	}

	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks")

	require.Equal(t, http.StatusNotImplemented, recorder.Code, recorder.Body.String())
}
