package gitlab

import (
	"context"
	"net/http"
	"testing"
)

// --- Task MR listing routes ---

func TestHTTPListWorkspaceTaskMRsGroupsByTask(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	ctx := context.Background()
	seedTask(t, store, "task-a", "ws-1")
	seedTask(t, store, "task-b", "ws-1")
	seedTask(t, store, "task-foreign", "ws-2")
	for _, mr := range []*TaskMR{
		newTestMR("task-a", "repo-1", "group/a", 11),
		newTestMR("task-b", "repo-1", "group/b", 22),
		newTestMR("task-foreign", "repo-1", "group/c", 33),
	} {
		if err := store.UpsertTaskMR(ctx, mr); err != nil {
			t.Fatalf("seed %s MR: %v", mr.TaskID, err)
		}
	}

	var body TaskMRsResponse
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/workspaces/ws-1/task-mrs", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.TaskMRs) != 2 {
		t.Fatalf("grouped tasks = %d, want 2 (%v)", len(body.TaskMRs), body.TaskMRs)
	}
	if _, leaked := body.TaskMRs["task-foreign"]; leaked {
		t.Error("the ws-2 task leaked into the ws-1 listing")
	}
	if got := body.TaskMRs["task-a"]; len(got) != 1 || got[0].MRIID != 11 {
		t.Errorf("task-a group = %+v, want the single !11 row", got)
	}
}

func TestHTTPListTaskMRsReturnsRowsForTask(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	ctx := context.Background()
	seedTask(t, store, "task-a", "ws-1")
	if err := store.UpsertTaskMR(ctx, newTestMR("task-a", "repo-1", "group/a", 11)); err != nil {
		t.Fatalf("seed MR: %v", err)
	}

	var body struct {
		TaskMRs []*TaskMR `json:"task_mrs"`
	}
	recorder := httpJSON(t, router, http.MethodGet, "/api/v1/gitlab/tasks/task-a/mrs", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.TaskMRs) != 1 || body.TaskMRs[0].ProjectPath != "group/a" {
		t.Fatalf("task_mrs = %+v, want the single group/a row", body.TaskMRs)
	}
}

func TestHTTPListTaskMRsReturnsEmptyListForUnknownTask(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	var body struct {
		TaskMRs []*TaskMR `json:"task_mrs"`
	}
	recorder := httpJSON(t, router, http.MethodGet, "/api/v1/gitlab/tasks/nope/mrs", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.TaskMRs) != 0 {
		t.Errorf("task_mrs = %+v, want an empty list", body.TaskMRs)
	}
}

// --- Watch reset routes ---

func TestHTTPPreviewResetReviewWatchCountsTasks(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	ctx := context.Background()
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}
	if _, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url"); err != nil {
		t.Fatalf("reserve review MR task: %v", err)
	}
	if err := store.AssignReviewMRTaskID(ctx, watch.ID, "group/p", 1, "task-1"); err != nil {
		t.Fatalf("assign review MR task: %v", err)
	}

	var body struct {
		TaskCount int `json:"taskCount"`
	}
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/watches/review/"+watch.ID+"/reset/preview?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if body.TaskCount != 1 {
		t.Errorf("taskCount = %d, want 1", body.TaskCount)
	}
}

func TestHTTPResetReviewWatchDeletesTaskTrees(t *testing.T) {
	svc, store, router := newWatchHTTPFixture(t)
	ctx := context.Background()
	deleter := &recordingCascadeDeleter{}
	svc.SetCascadeTaskDeleter(deleter)
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}
	if _, err := store.ReserveReviewMRTask(ctx, watch.ID, "group/p", 1, "url"); err != nil {
		t.Fatalf("reserve review MR task: %v", err)
	}
	if err := store.AssignReviewMRTaskID(ctx, watch.ID, "group/p", 1, "task-1"); err != nil {
		t.Fatalf("assign review MR task: %v", err)
	}

	var body struct {
		TasksDeleted int `json:"tasksDeleted"`
	}
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/review/"+watch.ID+"/reset?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if body.TasksDeleted != 1 {
		t.Errorf("tasksDeleted = %d, want 1", body.TasksDeleted)
	}
	if len(deleter.ids) != 1 || deleter.ids[0] != "task-1" {
		t.Errorf("deleted task trees = %v, want [task-1]", deleter.ids)
	}
}

// TestHTTPResetReviewWatchWithoutCascadeDeleterFails covers writeWatchResetError:
// an unwired cascade deleter is a server-side misconfiguration, and the reply
// must be a 500 whose message names the operation, not the internal cause.
func TestHTTPResetReviewWatchWithoutCascadeDeleterFails(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	watch := validReviewWatchRow()
	if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/review/"+watch.ID+"/reset?workspace_id=ws-1", nil)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "reset review watch failed" {
		t.Errorf("error = %q, want \"reset review watch failed\"", got)
	}
}

func TestHTTPResetReviewWatchRejectsForeignWatch(t *testing.T) {
	svc, store, router := newWatchHTTPFixture(t)
	deleter := &recordingCascadeDeleter{}
	svc.SetCascadeTaskDeleter(deleter)
	foreign := validReviewWatchRow()
	foreign.WorkspaceID = "ws-2"
	if err := store.CreateReviewWatch(t.Context(), foreign); err != nil {
		t.Fatalf("seed review watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/review/"+foreign.ID+"/reset?workspace_id=ws-1", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(deleter.ids) != 0 {
		t.Errorf("a cross-workspace reset deleted task trees %v", deleter.ids)
	}
}

func TestHTTPPreviewResetIssueWatchCountsTasks(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	ctx := context.Background()
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/p", 1, "url"); err != nil {
		t.Fatalf("reserve issue task: %v", err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "group/p", 1, "task-1"); err != nil {
		t.Fatalf("assign issue task: %v", err)
	}

	var body struct {
		TaskCount int `json:"taskCount"`
	}
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/watches/issue/"+watch.ID+"/reset/preview?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if body.TaskCount != 1 {
		t.Errorf("taskCount = %d, want 1", body.TaskCount)
	}
}

func TestHTTPResetIssueWatchDeletesTaskTrees(t *testing.T) {
	svc, store, router := newWatchHTTPFixture(t)
	ctx := context.Background()
	deleter := &recordingCascadeDeleter{}
	svc.SetCascadeTaskDeleter(deleter)
	watch := validIssueWatchRow()
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, watch.ID, "group/p", 1, "url"); err != nil {
		t.Fatalf("reserve issue task: %v", err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, "group/p", 1, "task-1"); err != nil {
		t.Fatalf("assign issue task: %v", err)
	}

	var body struct {
		TasksDeleted int `json:"tasksDeleted"`
	}
	recorder := httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/watches/issue/"+watch.ID+"/reset?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if body.TasksDeleted != 1 {
		t.Errorf("tasksDeleted = %d, want 1", body.TasksDeleted)
	}
	if len(deleter.ids) != 1 || deleter.ids[0] != "task-1" {
		t.Errorf("deleted task trees = %v, want [task-1]", deleter.ids)
	}
}

func TestHTTPPreviewResetIssueWatchRejectsForeignWatch(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)
	foreign := validIssueWatchRow()
	foreign.WorkspaceID = "ws-2"
	if err := store.CreateIssueWatch(t.Context(), foreign); err != nil {
		t.Fatalf("seed issue watch: %v", err)
	}

	recorder := watchScopeRequest(t, router, http.MethodGet,
		"/api/v1/gitlab/watches/issue/"+foreign.ID+"/reset/preview?workspace_id=ws-1", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "issue watch not found" {
		t.Errorf("error = %q, want \"issue watch not found\"", got)
	}
}

// --- Action presets over HTTP ---

func TestHTTPActionPresetsRoundTrip(t *testing.T) {
	_, store, router := newWatchHTTPFixture(t)

	var defaults ActionPresets
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/action-presets?workspace_id=ws-1", nil, &defaults)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(defaults.MR) == 0 {
		t.Fatal("mr presets are empty; the built-in defaults must be substituted")
	}

	var updated ActionPresets
	recorder = httpJSON(t, router, http.MethodPut,
		"/api/v1/gitlab/action-presets?workspace_id=ws-1",
		UpdateActionPresetsRequest{
			MR: &[]ActionPreset{{ID: "ship", Label: "Ship it", PromptTemplate: "ship {{mr}}"}},
		}, &updated)
	if recorder.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(updated.MR) != 1 || updated.MR[0].ID != "ship" {
		t.Fatalf("mr presets = %+v, want only the custom ship preset", updated.MR)
	}
	if updated.WorkspaceID != "ws-1" {
		t.Errorf("workspace = %q, want ws-1 from the query parameter", updated.WorkspaceID)
	}

	var reset ActionPresets
	recorder = httpJSON(t, router, http.MethodPost,
		"/api/v1/gitlab/action-presets/reset?workspace_id=ws-1", nil, &reset)
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	for _, preset := range reset.MR {
		if preset.ID == "ship" {
			t.Fatalf("mr presets = %+v, want the custom preset gone after reset", reset.MR)
		}
	}
	stored, err := store.GetActionPresets(t.Context(), "ws-1")
	if err != nil {
		t.Fatalf("reload presets: %v", err)
	}
	if len(stored.MR) != 0 {
		t.Errorf("stored mr presets = %+v, want the row deleted", stored.MR)
	}
}

func TestHTTPUpdateActionPresetsRejectsInvalidPayload(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	recorder := watchScopeRequest(t, router, http.MethodPut,
		"/api/v1/gitlab/action-presets?workspace_id=ws-1", []string{"not", "an", "object"})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
	}
	if got := httpErrorMessage(t, recorder); got != "invalid payload" {
		t.Errorf("error = %q, want \"invalid payload\"", got)
	}
}

// --- Project routes ---

func TestHTTPListUserProjectsReturnsProjects(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	var body struct {
		Projects []Project `json:"projects"`
	}
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/projects?workspace_id=ws-1", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.Projects) != 1 || body.Projects[0].PathWithNamespace != "kandev/sample" {
		t.Fatalf("projects = %+v, want the single kandev/sample project", body.Projects)
	}
}

func TestHTTPSearchProjectsFiltersByQuery(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"matching", "sample", 1},
		{"non-matching", "no-such-project", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body struct {
				Projects []Project `json:"projects"`
			}
			recorder := httpJSON(t, router, http.MethodGet,
				"/api/v1/gitlab/projects/search?workspace_id=ws-1&query="+test.query, nil, &body)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
			}
			if len(body.Projects) != test.want {
				t.Errorf("projects = %d, want %d (%+v)", len(body.Projects), test.want, body.Projects)
			}
		})
	}
}

func TestHTTPProjectRoutesRequireProjectQuery(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	for _, path := range []string{"/projects/branches", "/projects/merge-methods"} {
		t.Run(path, func(t *testing.T) {
			recorder := watchScopeRequest(t, router, http.MethodGet,
				"/api/v1/gitlab"+path+"?workspace_id=ws-1", nil)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
			}
			if got := httpErrorMessage(t, recorder); got != "project required" {
				t.Errorf("error = %q, want \"project required\"", got)
			}
		})
	}
}

func TestHTTPGetProjectMergeMethodsReturnsAllowedMethods(t *testing.T) {
	_, _, router := newWatchHTTPFixture(t)

	var methods ProjectMergeMethods
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/projects/merge-methods?workspace_id=ws-1&project=group%2Fp", nil, &methods)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if !methods.Merge || !methods.AllowSquash {
		t.Errorf("methods = %+v, want the mock's merge+squash defaults", methods)
	}
}

func TestHTTPListProjectBranchesReturnsBranches(t *testing.T) {
	svc, _, router := newWatchHTTPFixture(t)
	svc.workspaceClients["ws-1"].(*MockClient).SeedBranches("group/p",
		[]RepoBranch{{Name: "main"}, {Name: "dev"}})

	var body struct {
		Branches []RepoBranch `json:"branches"`
	}
	recorder := httpJSON(t, router, http.MethodGet,
		"/api/v1/gitlab/projects/branches?workspace_id=ws-1&project=group%2Fp", nil, &body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if len(body.Branches) != 2 || body.Branches[0].Name != "main" {
		t.Fatalf("branches = %+v, want [main dev]", body.Branches)
	}
}
