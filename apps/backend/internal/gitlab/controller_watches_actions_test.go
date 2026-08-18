package gitlab

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWatchHTTPRoutesRequireWorkspaceAndValidatePayloads(t *testing.T) {
	_, _, router := newWatchScopeFixture(t)
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "missing workspace", method: http.MethodGet, path: "/api/v1/gitlab/projects"},
		{name: "missing project", method: http.MethodGet, path: "/api/v1/gitlab/projects/branches?workspace_id=workspace-a"},
		{name: "invalid merge payload", method: http.MethodPut, path: "/api/v1/gitlab/mrs/merge?workspace_id=workspace-a", body: `{}`},
		{name: "invalid labels payload", method: http.MethodPut, path: "/api/v1/gitlab/mrs/labels?workspace_id=workspace-a", body: `{`},
		{name: "invalid assignees payload", method: http.MethodPut, path: "/api/v1/gitlab/mrs/assignees?workspace_id=workspace-a", body: `{}`},
		{name: "missing files iid", method: http.MethodGet, path: "/api/v1/gitlab/mrs/files?workspace_id=workspace-a&project=acme/widget"},
		{name: "missing commits project", method: http.MethodGet, path: "/api/v1/gitlab/mrs/commits?workspace_id=workspace-a&iid=2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptestRequest(tc.method, tc.path, tc.body)
			response := executeRequest(router, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWatchHTTPProjectAndMRActionRoutes(t *testing.T) {
	_, service, router := newWatchScopeFixture(t)
	client := service.workspaceClients["workspace-a"].(*MockClient)
	client.SeedProjectMembers("acme/widget", []ProjectMember{{ID: 7, Username: "alice", Name: "Alice"}})
	client.SeedBranches("acme/widget", []RepoBranch{{Name: "main"}, {Name: "release"}})
	client.SeedMR("acme/widget", &MR{IID: 2, Title: "Feature", State: mrStateOpen})
	client.SeedFiles("acme/widget", 2, []MRFile{{Filename: "main.go", Additions: 3}})
	client.SeedCommits("acme/widget", 2, []MRCommitInfo{{SHA: "abc123", Message: "change"}})

	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		wantString string
	}{
		{name: "list projects", method: http.MethodGet, path: "/projects", wantString: "kandev/sample"},
		{name: "search projects", method: http.MethodGet, path: "/projects/search?query=sample", wantString: "kandev/sample"},
		{name: "list branches", method: http.MethodGet, path: "/projects/branches?project=acme%2Fwidget", wantString: "release"},
		{name: "merge methods", method: http.MethodGet, path: "/projects/merge-methods?project=acme%2Fwidget", wantString: `"merge":true`},
		{name: "approve", method: http.MethodPost, path: "/mrs/approve", body: map[string]any{"project": "acme/widget", "iid": 2}, wantString: `"approved":true`},
		{name: "unapprove", method: http.MethodPost, path: "/mrs/unapprove", body: map[string]any{"project": "acme/widget", "iid": 2}, wantString: `"unapproved":true`},
		{name: "set labels", method: http.MethodPut, path: "/mrs/labels", body: map[string]any{"project": "acme/widget", "iid": 2, "labels": []string{"backend"}}, wantString: `"updated":true`},
		{name: "set assignees", method: http.MethodPut, path: "/mrs/assignees", body: map[string]any{"project": "acme/widget", "iid": 2, "assignee_ids": []int{7}}, wantString: `"updated":true`},
		{name: "files", method: http.MethodGet, path: "/mrs/files?project=acme%2Fwidget&iid=2", wantString: "main.go"},
		{name: "commits", method: http.MethodGet, path: "/mrs/commits?project=acme%2Fwidget&iid=2", wantString: "abc123"},
		{name: "merge", method: http.MethodPut, path: "/mrs/merge", body: map[string]any{"project": "acme/widget", "iid": 2, "method": "squash", "squash_commit_message": "combined"}, wantString: `"state":"merged"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := ""
			if tc.body != nil {
				encoded, err := json.Marshal(tc.body)
				if err != nil {
					t.Fatal(err)
				}
				body = string(encoded)
			}
			separator := "?"
			if strings.Contains(tc.path, "?") {
				separator = "&"
			}
			request := httptestRequest(tc.method, "/api/v1/gitlab"+tc.path+separator+"workspace_id=workspace-a", body)
			response := executeRequest(router, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), tc.wantString) {
				t.Fatalf("status=%d body=%s, want 200 containing %q", response.Code, response.Body.String(), tc.wantString)
			}
		})
	}

	mr, err := client.GetMR(t.Context(), "acme/widget", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(mr.Labels) != 1 || mr.Labels[0] != "backend" || len(mr.Assignees) != 1 || mr.Assignees[0].Username != "alice" || mr.State != gitlabStateMerged {
		t.Fatalf("mutated MR = %#v", mr)
	}
}

func TestWatchHTTPActionPresetRoutes(t *testing.T) {
	_, _, router := newWatchScopeFixture(t)

	get := watchScopeRequest(t, router, http.MethodGet, "/api/v1/gitlab/action-presets?workspace_id=workspace-a", nil)
	if get.Code != http.StatusOK || !bytes.Contains(get.Body.Bytes(), []byte(`"mr"`)) {
		t.Fatalf("get presets status=%d body=%s", get.Code, get.Body.String())
	}

	custom := []ActionPreset{{ID: "review", Label: "Review", PromptTemplate: "Review {{url}}"}}
	update := watchScopeRequest(t, router, http.MethodPut, "/api/v1/gitlab/action-presets?workspace_id=workspace-a", UpdateActionPresetsRequest{MR: &custom})
	if update.Code != http.StatusOK || !bytes.Contains(update.Body.Bytes(), []byte(`"id":"review"`)) {
		t.Fatalf("update presets status=%d body=%s", update.Code, update.Body.String())
	}

	reset := watchScopeRequest(t, router, http.MethodPost, "/api/v1/gitlab/action-presets/reset?workspace_id=workspace-a", nil)
	if reset.Code != http.StatusOK || bytes.Contains(reset.Body.Bytes(), []byte(`"prompt_template":"Review {{url}}"`)) {
		t.Fatalf("reset presets status=%d body=%s", reset.Code, reset.Body.String())
	}
}

func httptestRequest(method, path, body string) *http.Request {
	request, _ := http.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func executeRequest(router http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
