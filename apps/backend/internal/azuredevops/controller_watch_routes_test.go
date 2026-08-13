package azuredevops

import (
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func pullRequestWatchPayload() map[string]any {
	return map[string]any{
		"workflowId": "wf-1", "workflowStepId": "step-1", "projectId": "project-1",
		"azureRepositoryId": "azure-repo-1", "status": "active",
		"repositoryId": "repo-1", "baseBranch": "main",
		"agentProfileId": "agent-1", "executorProfileId": "executor-1",
		"prompt": "Review {{title}}", "pollIntervalSeconds": 1,
	}
}

func newWatchRouteFixture(t *testing.T) (*gin.Engine, *Service, *Store) {
	t.Helper()
	client := &watchClient{}
	router, service := newRouteFixture(t, client)
	seedRouteConfig(t, service)
	return router, service, service.Store()
}

func TestControllerPullRequestWatchRouteLifecycle(t *testing.T) {
	router, _, store := newWatchRouteFixture(t)
	const path = "/api/v1/azure-devops/watches/pull-requests"
	const query = "?workspace_id=ws-1"

	created := performRequest(t, router, http.MethodPost, path+query, pullRequestWatchPayload())
	if created.Code != http.StatusOK {
		t.Fatalf("POST watch = %d: %s", created.Code, created.Body.String())
	}
	watch := decodeJSON[PullRequestWatch](t, created)
	if watch.ID == "" || watch.WorkspaceID != "ws-1" || watch.ProjectID != "project-1" ||
		watch.AzureRepositoryID != "azure-repo-1" || watch.Status != "active" ||
		watch.RepositoryID != "repo-1" || watch.BaseBranch != "main" ||
		!watch.Enabled || watch.PollIntervalSeconds != minWatchPollIntervalSeconds ||
		watch.CleanupPolicy != CleanupPolicyAuto {
		t.Fatalf("created watch = %+v", watch)
	}

	listed := performRequest(t, router, http.MethodGet, path+query, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("GET watches = %d: %s", listed.Code, listed.Body.String())
	}
	list := decodeJSON[struct {
		Watches []PullRequestWatch `json:"watches"`
	}](t, listed)
	if len(list.Watches) != 1 || list.Watches[0].ID != watch.ID {
		t.Fatalf("listed watches = %+v", list.Watches)
	}
	// The list is workspace scoped.
	other := performRequest(t, router, http.MethodGet, path+"?workspace_id=ws-2", nil)
	if other.Code != http.StatusOK {
		t.Fatalf("GET watches for another workspace = %d: %s", other.Code, other.Body.String())
	}
	if otherList := decodeJSON[struct {
		Watches []PullRequestWatch `json:"watches"`
	}](t, other); len(otherList.Watches) != 0 {
		t.Fatalf("watch leaked into another workspace: %+v", otherList.Watches)
	}

	updated := performRequest(t, router, http.MethodPatch, path+"/"+watch.ID+query, map[string]any{
		"enabled": false, "status": "completed", "prompt": "Fix {{title}}",
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("PATCH watch = %d: %s", updated.Code, updated.Body.String())
	}
	patched := decodeJSON[PullRequestWatch](t, updated)
	if patched.Enabled || patched.Status != "completed" || patched.Prompt != "Fix {{title}}" ||
		patched.AzureRepositoryID != "azure-repo-1" {
		t.Fatalf("patched watch = %+v", patched)
	}

	triggered := performRequest(t, router, http.MethodPost, path+"/"+watch.ID+"/trigger"+query, nil)
	if triggered.Code != http.StatusOK {
		t.Fatalf("POST trigger = %d: %s", triggered.Code, triggered.Body.String())
	}
	if result := decodeJSON[WatchTriggerResult](t, triggered); result.Matched != 0 {
		t.Fatalf("trigger result = %+v, want no matches from the empty client", result)
	}

	preview := performRequest(t, router, http.MethodGet, path+"/"+watch.ID+"/reset/preview"+query, nil)
	if preview.Code != http.StatusOK || decodeJSON[map[string]int](t, preview)["taskCount"] != 0 {
		t.Fatalf("reset preview = %d: %s", preview.Code, preview.Body.String())
	}

	reset := performRequest(t, router, http.MethodPost, path+"/"+watch.ID+"/reset"+query, nil)
	if reset.Code != http.StatusOK {
		t.Fatalf("POST reset = %d: %s", reset.Code, reset.Body.String())
	}
	resetBody := decodeJSON[map[string]int](t, reset)
	if resetBody["generation"] != 2 || resetBody["taskCount"] != 0 {
		t.Fatalf("reset body = %+v", resetBody)
	}

	deleted := performRequest(t, router, http.MethodDelete, path+"/"+watch.ID+query, nil)
	if deleted.Code != http.StatusOK || decodeJSON[map[string]bool](t, deleted)["deleted"] != true {
		t.Fatalf("DELETE watch = %d: %s", deleted.Code, deleted.Body.String())
	}
	if got, err := store.GetPullRequestWatch(t.Context(), watch.ID); err != nil || got != nil {
		t.Fatalf("watch after delete = %+v, %v", got, err)
	}
	if repeat := performRequest(t, router, http.MethodDelete, path+"/"+watch.ID+query, nil); repeat.Code != http.StatusNotFound {
		t.Fatalf("repeat DELETE = %d: %s", repeat.Code, repeat.Body.String())
	}
}

func TestControllerWorkItemWatchDeleteAndTriggerRoutes(t *testing.T) {
	router, _, store := newWatchRouteFixture(t)
	watch := seedWorkItemWatch(t, store)
	const path = "/api/v1/azure-devops/watches/work-items"
	const query = "?workspace_id=ws-1"

	triggered := performRequest(t, router, http.MethodPost, path+"/"+watch.ID+"/trigger"+query, nil)
	if triggered.Code != http.StatusOK {
		t.Fatalf("POST trigger = %d: %s", triggered.Code, triggered.Body.String())
	}
	if result := decodeJSON[WatchTriggerResult](t, triggered); result.Matched != 0 {
		t.Fatalf("trigger result = %+v", result)
	}

	deleted := performRequest(t, router, http.MethodDelete, path+"/"+watch.ID+query, nil)
	if deleted.Code != http.StatusOK || decodeJSON[map[string]bool](t, deleted)["deleted"] != true {
		t.Fatalf("DELETE watch = %d: %s", deleted.Code, deleted.Body.String())
	}
	if got, err := store.GetWorkItemWatch(t.Context(), watch.ID); err != nil || got != nil {
		t.Fatalf("watch after delete = %+v, %v", got, err)
	}
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, path + "/" + watch.ID + "/trigger" + query},
		{http.MethodDelete, path + "/" + watch.ID + query},
		{http.MethodGet, path + "/" + watch.ID + "/reset/preview" + query},
		{http.MethodPost, path + "/" + watch.ID + "/reset" + query},
	} {
		response := performRequest(t, router, route.method, route.path, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s %s after delete = %d: %s", route.method, route.path, response.Code, response.Body.String())
		}
	}
}

func TestControllerWatchRoutesRejectMalformedPayloads(t *testing.T) {
	router, _, store := newWatchRouteFixture(t)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	const query = "?workspace_id=ws-1"
	tests := []struct {
		name   string
		method string
		path   string
		body   any
		want   int
	}{
		{"create work-item watch with malformed json", http.MethodPost, "/api/v1/azure-devops/watches/work-items" + query, "nope", http.StatusBadRequest},
		{"create work-item watch without dependencies", http.MethodPost, "/api/v1/azure-devops/watches/work-items" + query, map[string]any{"projectId": "project-1"}, http.StatusBadRequest},
		{"update work-item watch with malformed json", http.MethodPatch, "/api/v1/azure-devops/watches/work-items/" + work.ID + query, "nope", http.StatusBadRequest},
		{"create pull-request watch with malformed json", http.MethodPost, "/api/v1/azure-devops/watches/pull-requests" + query, "nope", http.StatusBadRequest},
		{"create pull-request watch without dependencies", http.MethodPost, "/api/v1/azure-devops/watches/pull-requests" + query, map[string]any{"projectId": "project-1"}, http.StatusBadRequest},
		{"update pull-request watch with malformed json", http.MethodPatch, "/api/v1/azure-devops/watches/pull-requests/" + pr.ID + query, "nope", http.StatusBadRequest},
		{"create work-item watch without a workspace", http.MethodPost, "/api/v1/azure-devops/watches/work-items", map[string]any{}, http.StatusBadRequest},
		{"list work-item watches without a workspace", http.MethodGet, "/api/v1/azure-devops/watches/work-items", nil, http.StatusBadRequest},
		{"list pull-request watches without a workspace", http.MethodGet, "/api/v1/azure-devops/watches/pull-requests", nil, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performRequest(t, router, tt.method, tt.path, tt.body)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.want, response.Body.String())
			}
		})
	}
	if watches, err := store.ListPullRequestWatches(t.Context(), "ws-1"); err != nil || len(watches) != 1 {
		t.Fatalf("rejected payloads created watches: %+v %v", watches, err)
	}
}

func TestControllerTaskWorkItemRoutes(t *testing.T) {
	client := &taskWorkItemClient{fakeDetailClient: fakeDetailClient{detail: &WorkItemDetail{WorkItem: WorkItem{
		ID: 101, Title: "Fix build", State: "Active", Type: "Bug",
		Project: "project-1", WebURL: "https://dev.azure.com/acme/project/_workitems/edit/101",
	}}}}
	router, service := newRouteFixture(t, client)
	seedRouteConfig(t, service)
	if _, err := service.Store().db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := service.Store().db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1'), ('task-2', 'ws-other')`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	associated := performRequest(t, router, http.MethodPost,
		"/api/v1/azure-devops/tasks/task-1/work-items?workspace_id=ws-1",
		map[string]any{"projectId": "project-1", "workItemId": 101})
	if associated.Code != http.StatusOK {
		t.Fatalf("POST task work item = %d: %s", associated.Code, associated.Body.String())
	}
	row := decodeJSON[TaskWorkItem](t, associated)
	if row.TaskID != "task-1" || row.WorkspaceID != "ws-1" || row.ProjectID != "project-1" ||
		row.WorkItemID != 101 || row.Title != "Fix build" || row.State != "Active" ||
		row.Type != "Bug" || row.WorkItemURL == "" {
		t.Fatalf("associated row = %+v", row)
	}

	listed := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/workspaces/ws-1/task-work-items", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("GET task work items = %d: %s", listed.Code, listed.Body.String())
	}
	response := decodeJSON[TaskWorkItemsResponse](t, listed)
	if len(response.TaskWorkItems["task-1"]) != 1 || response.TaskWorkItems["task-1"][0].WorkItemID != 101 {
		t.Fatalf("listed task work items = %+v", response.TaskWorkItems)
	}
	if empty := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/workspaces/ws-other/task-work-items", nil); empty.Code != http.StatusOK {
		t.Fatalf("GET for another workspace = %d: %s", empty.Code, empty.Body.String())
	}
}

func TestControllerTaskWorkItemRouteRejectsInvalidAssociations(t *testing.T) {
	client := &taskWorkItemClient{fakeDetailClient: fakeDetailClient{
		detail: &WorkItemDetail{WorkItem: WorkItem{ID: 101, Project: "project-1"}},
	}}
	router, service := newRouteFixture(t, client)
	seedRouteConfig(t, service)
	if _, err := service.Store().db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := service.Store().db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1'), ('task-2', 'ws-other')`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}
	tests := []struct {
		name string
		path string
		body any
		want int
	}{
		{"malformed json", "/api/v1/azure-devops/tasks/task-1/work-items?workspace_id=ws-1", "nope", http.StatusBadRequest},
		{"missing project", "/api/v1/azure-devops/tasks/task-1/work-items?workspace_id=ws-1", map[string]any{"workItemId": 101}, http.StatusBadRequest},
		{"non-positive work item", "/api/v1/azure-devops/tasks/task-1/work-items?workspace_id=ws-1", map[string]any{"projectId": "project-1", "workItemId": 0}, http.StatusBadRequest},
		{"task in another workspace", "/api/v1/azure-devops/tasks/task-2/work-items?workspace_id=ws-1", map[string]any{"projectId": "project-1", "workItemId": 101}, http.StatusBadRequest},
		{"work item in another project", "/api/v1/azure-devops/tasks/task-1/work-items?workspace_id=ws-1", map[string]any{"projectId": "project-2", "workItemId": 101}, http.StatusBadRequest},
		{"missing workspace", "/api/v1/azure-devops/tasks/task-1/work-items", map[string]any{"projectId": "project-1", "workItemId": 101}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performRequest(t, router, http.MethodPost, tt.path, tt.body)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.want, response.Body.String())
			}
		})
	}
	rows, err := service.ListTaskWorkItemsByWorkspace(t.Context(), "ws-1")
	if err != nil || len(rows) != 0 {
		t.Fatalf("rejected associations persisted rows: %+v %v", rows, err)
	}
}

// TestControllerMapsErrorsToTransportStatuses pins the writeError translation
// table, including the Azure-specific response codes the frontend switches on.
func TestControllerMapsErrorsToTransportStatuses(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		want     int
		wantCode string
	}{
		{name: "invalid config", err: ErrInvalidConfig, want: http.StatusBadRequest},
		{name: "invalid task PR association", err: ErrInvalidTaskPRAssociation, want: http.StatusBadRequest},
		{name: "watch not found", err: ErrWatchNotFound, want: http.StatusNotFound},
		{name: "reservation not found", err: ErrReservationNotFound, want: http.StatusNotFound},
		{name: "watch ownership lost", err: ErrWatchOwnershipLost, want: http.StatusConflict},
		{name: "not configured", err: ErrNotConfigured, want: http.StatusServiceUnavailable, wantCode: notConfiguredCode},
		{
			name: "upstream revision conflict", want: http.StatusConflict,
			wantCode: "azure_devops_revision_conflict",
			err:      &APIError{StatusCode: http.StatusConflict, Endpoint: "/_apis/projects", Body: "rev mismatch"},
		},
		{name: "upstream unauthorized", err: &APIError{StatusCode: http.StatusUnauthorized}, want: http.StatusUnauthorized},
		{name: "upstream forbidden", err: &APIError{StatusCode: http.StatusForbidden}, want: http.StatusForbidden},
		{name: "upstream rate limited", err: &APIError{StatusCode: http.StatusTooManyRequests}, want: http.StatusTooManyRequests},
		{name: "upstream server error", err: &APIError{StatusCode: http.StatusInternalServerError}, want: http.StatusBadGateway},
		{name: "upstream bad gateway", err: &APIError{StatusCode: http.StatusBadGateway}, want: http.StatusBadGateway},
		{name: "upstream not found", err: &APIError{StatusCode: http.StatusNotFound}, want: http.StatusBadRequest},
		{name: "wrapped upstream error", err: errors.Join(errors.New("context"), &APIError{StatusCode: http.StatusUnauthorized}), want: http.StatusUnauthorized},
		{name: "unclassified failure", err: errors.New("something broke"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, service := newRouteFixture(t, &invalidClient{err: tt.err})
			seedRouteConfig(t, service)
			response := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/projects?workspace_id=ws-1", nil)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.want, response.Body.String())
			}
			body := decodeJSON[map[string]string](t, response)
			if body["error"] != tt.err.Error() {
				t.Fatalf("error body = %q, want %q", body["error"], tt.err.Error())
			}
			if body["code"] != tt.wantCode {
				t.Fatalf("error code = %q, want %q", body["code"], tt.wantCode)
			}
		})
	}
}
