package plugins

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-WORKFLOW-HISTORY-001.4 AC-PLUGINS-WORKFLOW-HISTORY-001.5
func TestWebAppTransitionTrailChecksTaskScopeBeforeReading(t *testing.T) {
	task := &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1"}
	source := &fakeTaskDataSource{
		tasksByID:      map[string]*taskmodels.Task{"task-1": task, "task-2": {ID: "task-2", WorkspaceID: "ws-1"}},
		transitionRows: map[string][]taskmodels.StepTransition{"task-1": {{ID: 7, ToWorkflowStepID: stringPointer("review"), Trigger: "task_moved", OccurredAt: time.Now().UTC()}}},
	}
	svc := &Service{taskData: source}
	binding := webapp.CapabilityBinding{ScopeKind: instances.ScopeTask, WorkspaceID: "ws-1", TaskID: "task-1", Permissions: []string{"api_read:tasks"}}

	denied := httptest.NewRecorder()
	svc.handleWebAppProtocol(denied, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks/task-2/step-transitions")
	require.Equal(t, http.StatusNotFound, denied.Code)
	require.Zero(t, source.transitionCalls)

	allowed := httptest.NewRecorder()
	svc.handleWebAppProtocol(allowed, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/tasks/task-1/step-transitions")
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
	require.Equal(t, 1, source.transitionCalls)
	require.Contains(t, allowed.Body.String(), `"id":"7"`)
	require.NotContains(t, allowed.Body.String(), "actor_id")
	require.NotContains(t, allowed.Body.String(), "session_id")
}

// @covers AC-PLUGINS-WORKFLOW-HISTORY-002.1
func TestWebAppTransitionGroupsFindWorkflowBeyondFirstListPage(t *testing.T) {
	workflows := make([]*taskmodels.Workflow, 201)
	for i := range workflows {
		workflows[i] = &taskmodels.Workflow{ID: "wf-" + strconv.Itoa(i), WorkspaceID: "ws-1"}
	}
	source := &fakeTaskDataSource{transitionGroups: map[string][]taskmodels.TransitionGroup{"wf-200": {{Kind: "entry", Count: 1}}}}
	svc := &Service{taskData: source, workflows: &fakeWorkflowLister{workflows: map[string][]*taskmodels.Workflow{"ws-1": workflows}}, workflowSteps: &fakeWorkflowStepLister{}}
	binding := webapp.CapabilityBinding{ScopeKind: instances.ScopeWorkspace, WorkspaceID: "ws-1", Permissions: []string{"api_read:tasks", "api_read:workflows"}}
	recorder := httptest.NewRecorder()
	svc.handleWebAppProtocol(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "", binding, "v1/data/workflows/wf-200/transition-groups")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func stringPointer(value string) *string { return &value }

// @covers AC-PLUGINS-WORKFLOW-HISTORY-002.1 AC-PLUGINS-WORKFLOW-HISTORY-002.3
func TestWebAppTransitionGroupsRequireWorkspaceAndBothGrants(t *testing.T) {
	source := &fakeTaskDataSource{transitionGroups: map[string][]taskmodels.TransitionGroup{
		"wf-1": {{Kind: "within", FromStepID: stringPointer("draft"), ToStepID: stringPointer("review"), Count: 2}},
	}}
	svc := &Service{
		taskData:      source,
		workflows:     &fakeWorkflowLister{workflows: map[string][]*taskmodels.Workflow{"ws-1": {{ID: "wf-1", WorkspaceID: "ws-1"}}}},
		workflowSteps: &fakeWorkflowStepLister{},
	}
	binding := webapp.CapabilityBinding{ScopeKind: instances.ScopeTask, WorkspaceID: "ws-1", TaskID: "task-1", Permissions: []string{"api_read:tasks", "api_read:workflows"}}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	call := func(workflowID string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		svc.handleWebAppProtocol(recorder, request, "", binding, "v1/data/workflows/"+workflowID+"/transition-groups")
		return recorder
	}
	require.Equal(t, http.StatusForbidden, call("wf-1").Code)
	binding.ScopeKind = instances.ScopeWorkspace
	binding.Permissions = []string{"api_read:workflows"}
	require.Equal(t, http.StatusForbidden, call("wf-1").Code)
	binding.Permissions = []string{"api_read:tasks", "api_read:workflows"}
	require.Equal(t, http.StatusNotFound, call("foreign-wf").Code)
	require.Zero(t, source.groupCalls)
	allowed := call("wf-1")
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
	require.Equal(t, 1, source.groupCalls)
	require.Contains(t, allowed.Body.String(), `"count":2`)
}
