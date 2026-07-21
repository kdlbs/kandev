package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newWatchScopeFixture(t *testing.T) (*Store, *Service, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace-a")
	seedWorkspace(t, store, "workspace-b")
	service := NewService(DefaultHost, NewMockClient(DefaultHost), AuthMethodPAT, nil, newTestLogger(t))
	service.SetStore(store)
	service.workspaceClients = map[string]Client{
		"workspace-a": NewMockClient("https://gitlab-a.example.com"),
		"workspace-b": NewMockClient("https://gitlab-b.example.com"),
	}
	router := gin.New()
	NewController(service, newTestLogger(t)).RegisterHTTPRoutes(router)
	return store, service, router
}

func watchScopeRequest(t *testing.T, router *gin.Engine, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, target, payload)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestMRWatchRoutesAreWorkspaceScoped(t *testing.T) {
	store, _, router := newWatchScopeFixture(t)
	seedTask(t, store, "task-a", "workspace-a")
	seedTask(t, store, "task-b", "workspace-b")
	watchA := &MRWatch{SessionID: "session-a", TaskID: "task-a", ProjectPath: "a/project", MRIID: 1}
	watchB := &MRWatch{SessionID: "session-b", TaskID: "task-b", ProjectPath: "b/project", MRIID: 2}
	for _, watch := range []*MRWatch{watchA, watchB} {
		if err := store.CreateMRWatch(t.Context(), watch); err != nil {
			t.Fatalf("create MR watch: %v", err)
		}
	}

	list := watchScopeRequest(t, router, http.MethodGet,
		"/api/v1/gitlab/watches/mr?workspace_id=workspace-a", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	if bytes.Contains(list.Body.Bytes(), []byte(watchB.ID)) {
		t.Fatalf("workspace-a list disclosed workspace-b watch: %s", list.Body.String())
	}
	if !bytes.Contains(list.Body.Bytes(), []byte(watchA.ID)) {
		t.Fatalf("workspace-a list omitted own watch: %s", list.Body.String())
	}

	deleted := watchScopeRequest(t, router, http.MethodDelete,
		"/api/v1/gitlab/watches/mr/"+watchB.ID+"?workspace_id=workspace-a", nil)
	if deleted.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	remaining, err := store.ListMRWatchesByTask(t.Context(), "task-b")
	if err != nil || len(remaining) != 1 {
		t.Fatalf("foreign MR watch was changed: rows=%d err=%v", len(remaining), err)
	}
}

func TestTriggerAllWatchRoutesOnlyDispatchRequestedWorkspace(t *testing.T) {
	store, service, router := newWatchScopeFixture(t)
	clientA := service.workspaceClients["workspace-a"].(*MockClient)
	clientB := service.workspaceClients["workspace-b"].(*MockClient)
	clientA.SeedMR("a/project", &MR{IID: 1, Title: "A", WebURL: "https://a/1", Reviewers: []MRReviewer{{Username: "kandev-tester"}}})
	clientB.SeedMR("b/project", &MR{IID: 2, Title: "B", WebURL: "https://b/2", Reviewers: []MRReviewer{{Username: "kandev-tester"}}})
	clientA.SeedIssue("a/project", &Issue{IID: 3, Title: "A", WebURL: "https://a/3"})
	clientB.SeedIssue("b/project", &Issue{IID: 4, Title: "B", WebURL: "https://b/4"})
	for _, watch := range []*ReviewWatch{
		{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true},
		{WorkspaceID: "workspace-b", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true},
	} {
		if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
			t.Fatalf("create review watch: %v", err)
		}
	}
	for _, watch := range []*IssueWatch{
		{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true},
		{WorkspaceID: "workspace-b", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true},
	} {
		if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
			t.Fatalf("create issue watch: %v", err)
		}
	}

	for _, path := range []string{"/watches/review/trigger-all", "/watches/issue/trigger-all"} {
		response := watchScopeRequest(t, router, http.MethodPost,
			"/api/v1/gitlab"+path+"?workspace_id=workspace-a", nil)
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"count":1`)) {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

type watchScopeTaskDeleter struct{ ids []string }

func (d *watchScopeTaskDeleter) DeleteTask(_ context.Context, taskID string) error {
	d.ids = append(d.ids, taskID)
	return nil
}

func TestCleanupWatchRoutesOnlyDeleteRequestedWorkspaceTasks(t *testing.T) {
	store, service, router := newWatchScopeFixture(t)
	deleter := &watchScopeTaskDeleter{}
	service.SetTaskDeleter(deleter)
	clientA := service.workspaceClients["workspace-a"].(*MockClient)
	clientB := service.workspaceClients["workspace-b"].(*MockClient)
	clientA.SeedMR("a/project", &MR{IID: 1, State: gitlabStateClosed})
	clientB.SeedMR("b/project", &MR{IID: 2, State: gitlabStateClosed})
	clientA.SeedIssue("a/project", &Issue{IID: 3, State: gitlabStateClosed})
	clientB.SeedIssue("b/project", &Issue{IID: 4, State: gitlabStateClosed})
	legacyClient := NewMockClient(DefaultHost)
	legacyClient.SeedMR("a/project", &MR{IID: 1, State: gitlabStateClosed})
	legacyClient.SeedMR("b/project", &MR{IID: 2, State: gitlabStateClosed})
	legacyClient.SeedIssue("a/project", &Issue{IID: 3, State: gitlabStateClosed})
	legacyClient.SeedIssue("b/project", &Issue{IID: 4, State: gitlabStateClosed})
	service.client = legacyClient

	reviewA := &ReviewWatch{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	reviewB := &ReviewWatch{WorkspaceID: "workspace-b", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	issueA := &IssueWatch{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	issueB := &IssueWatch{WorkspaceID: "workspace-b", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	for _, watch := range []*ReviewWatch{reviewA, reviewB} {
		if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
			t.Fatalf("create review watch: %v", err)
		}
	}
	for _, watch := range []*IssueWatch{issueA, issueB} {
		if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
			t.Fatalf("create issue watch: %v", err)
		}
	}
	for _, row := range []struct {
		watch *ReviewWatch
		path  string
		iid   int
		task  string
	}{{reviewA, "a/project", 1, "review-a"}, {reviewB, "b/project", 2, "review-b"}} {
		if _, err := store.ReserveReviewMRTask(t.Context(), row.watch.ID, row.path, row.iid, "url"); err != nil {
			t.Fatal(err)
		}
		if err := store.AssignReviewMRTaskID(t.Context(), row.watch.ID, row.path, row.iid, row.task); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		watch *IssueWatch
		path  string
		iid   int
		task  string
	}{{issueA, "a/project", 3, "issue-a"}, {issueB, "b/project", 4, "issue-b"}} {
		if _, err := store.ReserveIssueWatchTask(t.Context(), row.watch.ID, row.path, row.iid, "url"); err != nil {
			t.Fatal(err)
		}
		if err := store.AssignIssueWatchTaskID(t.Context(), row.watch.ID, row.path, row.iid, row.task); err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{"/cleanup/review-tasks", "/cleanup/issue-tasks"} {
		response := watchScopeRequest(t, router, http.MethodPost,
			"/api/v1/gitlab"+path+"?workspace_id=workspace-a", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	if len(deleter.ids) != 2 || deleter.ids[0] != "review-a" || deleter.ids[1] != "issue-a" {
		t.Fatalf("deleted tasks=%v, want only workspace-a tasks", deleter.ids)
	}
}

func TestActionPresetUpdateUsesAuthoritativeWorkspaceQuery(t *testing.T) {
	store, _, router := newWatchScopeFixture(t)
	mr := []ActionPreset{{ID: "foreign", Label: "foreign", PromptTemplate: "review {{url}}"}}
	response := watchScopeRequest(t, router, http.MethodPut,
		"/api/v1/gitlab/action-presets?workspace_id=workspace-a",
		UpdateActionPresetsRequest{WorkspaceID: "workspace-b", MR: &mr})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	a, err := store.GetActionPresets(t.Context(), "workspace-a")
	if err != nil || len(a.MR) != 1 {
		t.Fatalf("workspace-a presets=%+v err=%v", a, err)
	}
	b, err := store.GetActionPresets(t.Context(), "workspace-b")
	if err != nil || len(b.MR) != 0 {
		t.Fatalf("workspace-b was overwritten: presets=%+v err=%v", b, err)
	}
}
