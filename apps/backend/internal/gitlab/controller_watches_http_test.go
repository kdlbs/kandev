package gitlab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newWatchHTTPFixture builds the full v1 HTTP surface over a store seeded with
// two workspaces, the dependency validators that accept the "ws-1" fixture
// identifiers, and a per-workspace mock client. Watch CRUD over HTTP is
// workspace-scoped through the ?workspace_id= query parameter, so every test
// here can also assert the cross-workspace rejection.
func newWatchHTTPFixture(t *testing.T) (*Service, *Store, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, store := newDependencyTestService(t)
	seedWorkspace(t, store, "ws-1")
	seedWorkspace(t, store, "ws-2")
	svc.workspaceClients = map[string]Client{
		"ws-1": NewMockClient(DefaultHost),
		"ws-2": NewMockClient(DefaultHost),
	}
	router := gin.New()
	NewController(svc, newTestLogger(t)).RegisterHTTPRoutes(router)
	return svc, store, router
}

// httpJSON issues a request against the in-process router and decodes the JSON
// body, returning the recorder so the caller can assert the status code too.
func httpJSON(t *testing.T, router *gin.Engine, method, target string, body interface{}, out interface{}) *httptest.ResponseRecorder {
	t.Helper()
	recorder := watchScopeRequest(t, router, method, target, body)
	if out != nil && recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s %s body %s: %v", method, target, recorder.Body.String(), err)
		}
	}
	return recorder
}

// httpErrorMessage decodes the {"error": "..."} envelope every failure path in
// this controller writes.
func httpErrorMessage(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %s: %v", recorder.Body.String(), err)
	}
	return body.Error
}

// --- Workspace guard ---

// TestWatchRoutesRequireWorkspaceQuery covers the requireWorkspaceQuery
// middleware: every route under it must abort with 400 before its handler runs,
// so an unscoped caller can never reach workspace data.
func TestWatchRoutesRequireWorkspaceQuery(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/gitlab/watches/review"},
		{http.MethodGet, "/api/v1/gitlab/watches/issue"},
		{http.MethodGet, "/api/v1/gitlab/watches/mr"},
		{http.MethodPost, "/api/v1/gitlab/watches/review/trigger-all"},
		{http.MethodPost, "/api/v1/gitlab/watches/issue/trigger-all"},
		{http.MethodGet, "/api/v1/gitlab/stats"},
		{http.MethodGet, "/api/v1/gitlab/action-presets"},
		{http.MethodGet, "/api/v1/gitlab/projects"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			recorder := watchScopeRequest(t, router, route.method, route.path, nil)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 without workspace_id (body %s)",
					recorder.Code, recorder.Body.String())
			}
			if got := httpErrorMessage(t, recorder); got != "workspace_id query parameter required" {
				t.Errorf("error = %q, want the workspace_id requirement", got)
			}
		})
	}
}

// --- Review watches over HTTP ---

func TestHTTPCreateReviewWatchPersistsUnderQueryWorkspace(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	// WorkspaceID is deliberately wrong in the body: the controller must
	// overwrite it with the query parameter, which is the scoped value.
	body := validReviewWatchRequest()
	body.WorkspaceID = "ws-2"

	var created ReviewWatch
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/review?workspace_id=ws-1", body, &created)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if created.WorkspaceID != "ws-1" {
		t.Errorf("workspace = %q, want ws-1 from the query parameter, not the body", created.WorkspaceID)
	}
	stored, err := store.GetReviewWatch(t.Context(), created.ID)
	if err != nil || stored == nil {
		t.Fatalf("watch was not persisted: row=%v err=%v", stored, err)
	}
	if stored.WorkspaceID != "ws-1" {
		t.Errorf("stored workspace = %q, want ws-1", stored.WorkspaceID)
	}
}

func TestHTTPCreateReviewWatchRejectsInvalidPayload(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/gitlab/watches/review?workspace_id=ws-1", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "invalid payload" {
		t.Errorf("error = %q, want \"invalid payload\"", got)
	}
}

// TestHTTPCreateReviewWatchMapsInvalidConfigTo400 pins the httpRespondError
// mapping: a dependency that belongs to another workspace is a client error,
// and the response must not disclose which dependency failed.
func TestHTTPCreateReviewWatchMapsInvalidConfigTo400(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)
	body := validReviewWatchRequest()
	body.RepositoryID = "repo-other" // belongs to ws-2

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/review?workspace_id=ws-1", body)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "invalid watch configuration" {
		t.Errorf("error = %q, want the generic configuration message", got)
	}
	if body := recorder.Body.String(); strings.Contains(body, "repo-other") {
		t.Errorf("response disclosed the rejected dependency id: %s", body)
	}
}

func TestHTTPListReviewWatchesScopesToQueryWorkspace(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	mine := validReviewWatchRow()
	foreign := validReviewWatchRow()
	foreign.WorkspaceID = "ws-2"
	for _, watch := range []*ReviewWatch{mine, foreign} {
		if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
			t.Fatalf("seed review watch: %v", err)
		}
	}

	var body struct {
		Watches []*ReviewWatch `json:"watches"`
	}
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/watches/review?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.Watches) != 1 || body.Watches[0].ID != mine.ID {
		t.Fatalf("watches = %+v, want only the ws-1 watch", body.Watches)
	}
}

func TestHTTPUpdateReviewWatchAppliesPatchAndReturnsRow(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	var updated ReviewWatch
	recorder := httpJSON(t, router, http.MethodPatch,
		"/api/v1/gitlab/watches/review/"+watch.ID+"?workspace_id=ws-1",
		map[string]interface{}{"enabled": false}, &updated)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if updated.ID != watch.ID || updated.Enabled {
		t.Errorf("returned watch = %+v, want the disabled row echoed back", updated)
	}
	stored, err := store.GetReviewWatch(t.Context(), watch.ID)
	if err != nil {
		t.Fatalf("reload watch: %v", err)
	}
	if stored.Enabled {
		t.Error("watch is still enabled; the patch did not reach the store")
	}
}

// TestHTTPUpdateReviewWatchRejectsForeignWatch is the cross-workspace guard:
// requireReviewWatchInWorkspace must 404 before the patch is applied, and the
// foreign row must be untouched.
func TestHTTPUpdateReviewWatchRejectsForeignWatch(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	foreign := validReviewWatchRow()
	foreign.WorkspaceID = "ws-2"
	if err := store.CreateReviewWatch(t.Context(), foreign); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodPatch,
		"/api/v1/gitlab/watches/review/"+foreign.ID+"?workspace_id=ws-1",
		map[string]interface{}{"enabled": false})

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "review watch not found" {
		t.Errorf("error = %q, want \"review watch not found\"", got)
	}
	stored, err := store.GetReviewWatch(t.Context(), foreign.ID)
	if err != nil {
		t.Fatalf("reload foreign watch: %v", err)
	}
	if !stored.Enabled {
		t.Error("the foreign watch was modified by a cross-workspace request")
	}
}

func TestHTTPDeleteReviewWatchRemovesRow(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	var body map[string]bool
	recorder := httpJSON(t, router, http.MethodDelete,
		"/api/v1/gitlab/watches/review/"+watch.ID+"?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if !body[respKeyDeleted] {
		t.Errorf("payload = %v, want {\"deleted\":true}", body)
	}
	remaining, err := store.ListReviewWatches(t.Context(), "ws-1")
	if err != nil {
		t.Fatalf("list review watches: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("watches remaining = %d, want 0", len(remaining))
	}
}

func TestHTTPDeleteReviewWatchRejectsForeignWatch(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	foreign := validReviewWatchRow()
	foreign.WorkspaceID = "ws-2"
	if err := store.CreateReviewWatch(t.Context(), foreign); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodDelete,
		"/api/v1/gitlab/watches/review/"+foreign.ID+"?workspace_id=ws-1", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
	remaining, err := store.ListReviewWatches(t.Context(), "ws-2")
	if err != nil || len(remaining) != 1 {
		t.Fatalf("foreign watch was deleted: rows=%d err=%v", len(remaining), err)
	}
}

func TestHTTPTriggerReviewWatchReturnsMRsAndCount(t *testing.T) {
	svc, store, router := newWatchHTTPFixture(t)
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}
	client := svc.workspaceClients["ws-1"].(*MockClient)
	client.SetUser("kandev-tester")
	client.SeedMR("group/p", &MR{IID: 1, Title: "needs review", WebURL: "https://x/1",
		Reviewers: []MRReviewer{{Username: "kandev-tester"}}})

	var body struct {
		MRs   []*MR `json:"mrs"`
		Count int   `json:"count"`
	}
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/review/"+watch.ID+"/trigger?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.MRs) != 1 {
		t.Fatalf("mrs = %+v, want the single seeded MR to reach the client", body.MRs)
	}
	if body.MRs[0].IID != 1 {
		t.Errorf("mr iid = %d, want 1", body.MRs[0].IID)
	}
	if body.Count != len(body.MRs) {
		t.Errorf("count = %d but mrs = %d; the two must agree", body.Count, len(body.MRs))
	}
}

func TestHTTPTriggerReviewWatchRejectsForeignWatch(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	foreign := validReviewWatchRow()
	foreign.WorkspaceID = "ws-2"
	if err := store.CreateReviewWatch(t.Context(), foreign); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/review/"+foreign.ID+"/trigger?workspace_id=ws-1", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
}

// --- Issue watches over HTTP ---

func TestHTTPCreateIssueWatchPersistsUnderQueryWorkspace(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	body := validIssueWatchRequest()
	body.WorkspaceID = "ws-2"

	var created IssueWatch
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/issue?workspace_id=ws-1", body, &created)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if created.WorkspaceID != "ws-1" {
		t.Errorf("workspace = %q, want ws-1 from the query parameter", created.WorkspaceID)
	}
	stored, err := store.GetIssueWatch(t.Context(), created.ID)
	if err != nil || stored == nil {
		t.Fatalf("watch was not persisted: row=%v err=%v", stored, err)
	}
}

func TestHTTPCreateIssueWatchMapsInvalidConfigTo400(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)
	body := validIssueWatchRequest()
	body.ExecutorProfileID = "executor-missing"

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/issue?workspace_id=ws-1", body)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "invalid watch configuration" {
		t.Errorf("error = %q, want the generic configuration message", got)
	}
}

func TestHTTPListIssueWatchesScopesToQueryWorkspace(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	mine := validIssueWatchRow()
	foreign := validIssueWatchRow()
	foreign.WorkspaceID = "ws-2"
	for _, watch := range []*IssueWatch{mine, foreign} {
		if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
			t.Fatalf("seed issue watch: %v", err)
		}
	}

	var body struct {
		Watches []*IssueWatch `json:"watches"`
	}
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/watches/issue?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.Watches) != 1 || body.Watches[0].ID != mine.ID {
		t.Fatalf("watches = %+v, want only the ws-1 watch", body.Watches)
	}
}

func TestHTTPUpdateIssueWatchAppliesPatchAndReturnsRow(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	var updated IssueWatch
	recorder := httpJSON(t, router, http.MethodPut,
		"/api/v1/gitlab/watches/issue/"+watch.ID+"?workspace_id=ws-1",
		map[string]interface{}{"enabled": false}, &updated)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if updated.ID != watch.ID || updated.Enabled {
		t.Errorf("returned watch = %+v, want the disabled row echoed back", updated)
	}
	stored, err := store.GetIssueWatch(t.Context(), watch.ID)
	if err != nil {
		t.Fatalf("reload watch: %v", err)
	}
	if stored.Enabled {
		t.Error("watch is still enabled; the patch did not reach the store")
	}
}

func TestHTTPUpdateIssueWatchRejectsForeignWatch(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	foreign := validIssueWatchRow()
	foreign.WorkspaceID = "ws-2"
	if err := store.CreateIssueWatch(t.Context(), foreign); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodPut,
		"/api/v1/gitlab/watches/issue/"+foreign.ID+"?workspace_id=ws-1",
		map[string]interface{}{"enabled": false})

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "issue watch not found" {
		t.Errorf("error = %q, want \"issue watch not found\"", got)
	}
	stored, err := store.GetIssueWatch(t.Context(), foreign.ID)
	if err != nil {
		t.Fatalf("reload foreign watch: %v", err)
	}
	if !stored.Enabled {
		t.Error("the foreign watch was modified by a cross-workspace request")
	}
}

func TestHTTPDeleteIssueWatchRemovesRow(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	var body map[string]bool
	recorder := httpJSON(t, router, http.MethodDelete,
		"/api/v1/gitlab/watches/issue/"+watch.ID+"?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if !body[respKeyDeleted] {
		t.Errorf("payload = %v, want {\"deleted\":true}", body)
	}
	remaining, err := store.ListIssueWatches(t.Context(), "ws-1")
	if err != nil {
		t.Fatalf("list issue watches: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("watches remaining = %d, want 0", len(remaining))
	}
}

func TestHTTPTriggerIssueWatchRejectsForeignWatch(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	foreign := validIssueWatchRow()
	foreign.WorkspaceID = "ws-2"
	if err := store.CreateIssueWatch(t.Context(), foreign); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/issue/"+foreign.ID+"/trigger?workspace_id=ws-1", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
}

func TestHTTPTriggerIssueWatchReturnsIssuesAndCount(t *testing.T) {
	svc, store, router := newWatchHTTPFixture(t)
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}
	svc.workspaceClients["ws-1"].(*MockClient).SeedIssue("group/p",
		&Issue{IID: 3, Title: "bug", WebURL: "https://x/3"})

	var body struct {
		Issues []*Issue `json:"issues"`
		Count  int      `json:"count"`
	}
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/issue/"+watch.ID+"/trigger?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.Issues) != 1 {
		t.Fatalf("issues = %+v, want the single seeded issue to reach the client", body.Issues)
	}
	if body.Issues[0].IID != 3 {
		t.Errorf("issue iid = %d, want 3", body.Issues[0].IID)
	}
	if body.Count != len(body.Issues) {
		t.Errorf("count = %d but issues = %d; the two must agree", body.Count, len(body.Issues))
	}
}

// --- Stats ---

func TestHTTPGetStatsReturnsWorkspaceCounts(t *testing.T) {
	svc, _, router := newWatchHTTPFixture(t)
	client := svc.workspaceClients["ws-1"].(*MockClient)
	client.SetUser("alice")
	client.SeedMR("group/p", &MR{IID: 1, Title: "one", State: "opened"})
	client.SeedIssue("group/p", &Issue{IID: 2, Title: "bug", State: "opened"})

	var stats Stats
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/stats?workspace_id=ws-1", nil, &stats)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if stats.OpenMRs != 1 {
		t.Errorf("open_mrs = %d, want 1", stats.OpenMRs)
	}
	if stats.OpenIssuesAssignedMe != 1 {
		t.Errorf("open_issues_assigned_me = %d, want 1", stats.OpenIssuesAssignedMe)
	}
}

// TestHTTPGetStatsIsScopedToItsWorkspace proves the stats route reads the
// workspace's own client rather than a shared one: ws-2 has nothing seeded, so
// its counts must be zero while ws-1's are not.
func TestHTTPGetStatsIsScopedToItsWorkspace(t *testing.T) {
	svc, _, router := newWatchHTTPFixture(t)
	client := svc.workspaceClients["ws-1"].(*MockClient)
	client.SetUser("alice")
	client.SeedMR("group/p", &MR{IID: 1, Title: "one", State: "opened"})
	svc.workspaceClients["ws-2"].(*MockClient).SetUser("bob")

	var stats Stats
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/stats?workspace_id=ws-2", nil, &stats)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if stats.OpenMRs != 0 {
		t.Errorf("open_mrs = %d for ws-2, want 0 — ws-1's MRs must not leak", stats.OpenMRs)
	}
}
