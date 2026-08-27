package azuredevops

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// newRouteFixture mounts the real router over a real store with a caller
// supplied Azure client, and leaves the workspace unconfigured so the config
// routes themselves can be exercised end to end.
func newRouteFixture(t *testing.T, client Client) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service := NewService(store, newFakeSecretStore(), func(*Config, string) Client { return client }, logger.Default())
	router := gin.New()
	RegisterRoutes(router, service, logger.Default())
	return router, service
}

func seedRouteConfig(t *testing.T, service *Service) {
	t.Helper()
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func decodeJSON[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var decoded T
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode %d response %q: %v", response.Code, response.Body.String(), err)
	}
	return decoded
}

func TestControllerConfigRouteLifecycle(t *testing.T) {
	client := &readsClient{auth: &TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada"}}
	router, _ := newRouteFixture(t, client)
	const query = "?workspace_id=ws-1"

	missing := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/config"+query, nil)
	if missing.Code != http.StatusNoContent || missing.Body.Len() != 0 {
		t.Fatalf("unconfigured GET /config = %d %q, want 204 with no body", missing.Code, missing.Body.String())
	}

	created := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/config"+query, map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "pat": "super-secret",
		"defaultProjectId": "project-1", "defaultProjectName": "Platform",
	})
	if created.Code != http.StatusOK {
		t.Fatalf("POST /config = %d: %s", created.Code, created.Body.String())
	}
	cfg := decodeJSON[Config](t, created)
	if cfg.OrganizationURL != "https://dev.azure.com/acme" || cfg.DefaultProjectID != "project-1" ||
		cfg.DefaultProjectName != "Platform" || cfg.AuthMethod != AuthMethodPAT ||
		!cfg.HasSecret || !cfg.LastOK || cfg.LastCheckedAt == nil {
		t.Fatalf("created config = %+v", cfg)
	}
	if raw := created.Body.String(); jsonContainsKey(t, raw, "pat") {
		t.Fatalf("config response leaked the PAT: %s", raw)
	}

	loaded := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/config"+query, nil)
	if loaded.Code != http.StatusOK || decodeJSON[Config](t, loaded).OrganizationURL != cfg.OrganizationURL {
		t.Fatalf("GET /config = %d: %s", loaded.Code, loaded.Body.String())
	}

	deleted := performRequest(t, router, http.MethodDelete, "/api/v1/azure-devops/config"+query, nil)
	if deleted.Code != http.StatusOK || decodeJSON[map[string]bool](t, deleted)["deleted"] != true {
		t.Fatalf("DELETE /config = %d: %s", deleted.Code, deleted.Body.String())
	}
	gone := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/config"+query, nil)
	if gone.Code != http.StatusNoContent {
		t.Fatalf("GET /config after delete = %d: %s", gone.Code, gone.Body.String())
	}
}

func jsonContainsKey(t *testing.T, raw, key string) bool {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	_, ok := decoded[key]
	return ok
}

func TestControllerConfigRouteRejectsBadInput(t *testing.T) {
	router, _ := newRouteFixture(t, &readsClient{auth: &TestConnectionResult{OK: true, ID: "me"}})
	tests := []struct {
		name string
		path string
		body any
	}{
		{"malformed json", "/api/v1/azure-devops/config?workspace_id=ws-1", "not-an-object"},
		{"missing organization", "/api/v1/azure-devops/config?workspace_id=ws-1", map[string]any{"pat": "x"}},
		{"non-canonical organization", "/api/v1/azure-devops/config?workspace_id=ws-1", map[string]any{
			"organizationUrl": "https://dev.azure.com/acme/project", "pat": "x",
		}},
		{"unsupported auth method", "/api/v1/azure-devops/config?workspace_id=ws-1", map[string]any{
			"organizationUrl": "https://dev.azure.com/acme", "pat": "x", "authMethod": "oauth",
		}},
		{"missing workspace", "/api/v1/azure-devops/config", map[string]any{
			"organizationUrl": "https://dev.azure.com/acme", "pat": "x",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performRequest(t, router, http.MethodPost, tt.path, tt.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
		})
	}
	if response := performRequest(t, router, http.MethodDelete, "/api/v1/azure-devops/config", nil); response.Code != http.StatusBadRequest {
		t.Fatalf("DELETE without workspace = %d: %s", response.Code, response.Body.String())
	}
}

func TestControllerTestConfigRouteProbesSubmittedAndStoredCredentials(t *testing.T) {
	client := &readsClient{auth: &TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada", Email: "ada@example.com"}}
	router, _ := newRouteFixture(t, client)
	const query = "?workspace_id=ws-1"

	submitted := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/config/test"+query, map[string]any{
		"organizationUrl": "https://dev.azure.com/acme", "pat": "probe-only",
	})
	if submitted.Code != http.StatusOK {
		t.Fatalf("POST /config/test = %d: %s", submitted.Code, submitted.Body.String())
	}
	result := decodeJSON[TestConnectionResult](t, submitted)
	if !result.OK || result.ID != "me" || result.DisplayName != "Ada" || result.Email != "ada@example.com" {
		t.Fatalf("probe result = %+v", result)
	}
	// A probe must not persist anything.
	if stored := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/config"+query, nil); stored.Code != http.StatusNoContent {
		t.Fatalf("probe persisted a config: %d %s", stored.Code, stored.Body.String())
	}

	// With no body and no stored credentials the failure is reported inside
	// the 200 result rather than as an HTTP error.
	empty := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/config/test"+query, nil)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty-body probe = %d: %s", empty.Code, empty.Body.String())
	}
	if emptyResult := decodeJSON[TestConnectionResult](t, empty); emptyResult.OK || emptyResult.Error != ErrNotConfigured.Error() {
		t.Fatalf("empty-body probe result = %+v", emptyResult)
	}

	invalid := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/config/test"+query, "not-an-object")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("malformed probe payload = %d: %s", invalid.Code, invalid.Body.String())
	}
}

func TestControllerCopyConfigRoute(t *testing.T) {
	client := &readsClient{auth: &TestConnectionResult{OK: true, ID: "me"}}
	router, service := newRouteFixture(t, client)
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", DefaultProjectID: "project-1", PAT: "pat",
	}); err != nil {
		t.Fatalf("seed source config: %v", err)
	}

	copied := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/config/copy?workspace_id=ws-1",
		map[string]any{"targetWorkspaceId": "ws-2"})
	if copied.Code != http.StatusOK {
		t.Fatalf("POST /config/copy = %d: %s", copied.Code, copied.Body.String())
	}
	target := decodeJSON[Config](t, copied)
	if target.WorkspaceID != "ws-2" || target.OrganizationURL != "https://dev.azure.com/acme" ||
		target.DefaultProjectID != "project-1" || !target.HasSecret {
		t.Fatalf("copied config = %+v", target)
	}
	if target.LastCheckedAt != nil || target.LastOK {
		t.Fatalf("copied config inherited health state: %+v", target)
	}

	invalid := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/config/copy?workspace_id=ws-1", "nope")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("malformed copy payload = %d: %s", invalid.Code, invalid.Body.String())
	}
	blank := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/config/copy?workspace_id=ws-1",
		map[string]any{"targetWorkspaceId": " "})
	if blank.Code != http.StatusBadRequest {
		t.Fatalf("blank target workspace = %d: %s", blank.Code, blank.Body.String())
	}
}

func TestControllerWorkspaceSettingsRoutes(t *testing.T) {
	router, service := newRouteFixture(t, &readsClient{})
	seedRouteConfig(t, service)
	const path = "/api/v1/azure-devops/workspace-settings?workspace_id=ws-1"

	defaults := performRequest(t, router, http.MethodGet, path, nil)
	if defaults.Code != http.StatusOK {
		t.Fatalf("GET workspace-settings = %d: %s", defaults.Code, defaults.Body.String())
	}
	settings := decodeJSON[WorkspaceSettings](t, defaults)
	if settings.WorkspaceID != "ws-1" ||
		len(settings.WorkItemQueries) != len(DefaultWorkItemQueryPresets()) ||
		len(settings.PullRequestQueries) != len(DefaultPullRequestQueryPresets()) ||
		len(settings.WorkItemActions) != len(DefaultWorkItemActionPresets()) ||
		len(settings.PullRequestActions) != len(DefaultPullRequestActionPresets()) {
		t.Fatalf("default settings = %+v", settings)
	}

	patched := performRequest(t, router, http.MethodPatch, path, map[string]any{
		"workItemActions": []map[string]any{{
			"id": "triage", "label": "Triage", "hint": "Sort it", "icon": "inbox",
			"promptTemplate": "Triage {{url}}",
		}},
	})
	if patched.Code != http.StatusOK {
		t.Fatalf("PATCH workspace-settings = %d: %s", patched.Code, patched.Body.String())
	}
	updated := decodeJSON[WorkspaceSettings](t, patched)
	if len(updated.WorkItemActions) != 1 || updated.WorkItemActions[0].ID != "triage" ||
		updated.WorkItemActions[0].PromptTemplate != "Triage {{url}}" {
		t.Fatalf("patched work-item actions = %+v", updated.WorkItemActions)
	}
	if len(updated.PullRequestActions) != len(DefaultPullRequestActionPresets()) {
		t.Fatalf("unrelated presets were replaced: %+v", updated.PullRequestActions)
	}

	reloaded := decodeJSON[WorkspaceSettings](t, performRequest(t, router, http.MethodGet, path, nil))
	if len(reloaded.WorkItemActions) != 1 || reloaded.WorkItemActions[0].Label != "Triage" {
		t.Fatalf("override did not persist: %+v", reloaded.WorkItemActions)
	}
}

func TestControllerWorkspaceSettingsRouteRejectsBadInput(t *testing.T) {
	router, service := newRouteFixture(t, &readsClient{})
	seedRouteConfig(t, service)
	tests := []struct {
		name string
		path string
		body any
		want int
	}{
		{"malformed json", "/api/v1/azure-devops/workspace-settings?workspace_id=ws-1", "nope", http.StatusBadRequest},
		{"missing workspace", "/api/v1/azure-devops/workspace-settings", map[string]any{}, http.StatusBadRequest},
		{
			name: "preset without a label", path: "/api/v1/azure-devops/workspace-settings?workspace_id=ws-1",
			body: map[string]any{"workItemQueries": []map[string]any{{"id": "x", "filters": map[string]any{"wiql": "SELECT"}}}},
			want: http.StatusBadRequest,
		},
		{
			name: "empty preset list", path: "/api/v1/azure-devops/workspace-settings?workspace_id=ws-1",
			body: map[string]any{"pullRequestActions": []map[string]any{}},
			want: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performRequest(t, router, http.MethodPatch, tt.path, tt.body)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.want, response.Body.String())
			}
		})
	}
	if response := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/workspace-settings", nil); response.Code != http.StatusBadRequest {
		t.Fatalf("GET without workspace = %d: %s", response.Code, response.Body.String())
	}
}

func TestControllerRepositoryAndBranchRoutes(t *testing.T) {
	client := &readsClient{
		repositories: []Repository{{
			ID: "repo-1", Name: "widgets", ProjectName: "Platform",
			DefaultBranch: "main", WebURL: "https://dev.azure.com/acme/Platform/_git/widgets",
		}},
		branches: []Branch{{Name: "main"}, {Name: "feature/azure"}},
	}
	router, service := newRouteFixture(t, client)
	seedRouteConfig(t, service)

	repositories := performRequest(t, router, http.MethodGet,
		"/api/v1/azure-devops/repositories?workspace_id=ws-1&project=project-1", nil)
	if repositories.Code != http.StatusOK {
		t.Fatalf("GET /repositories = %d: %s", repositories.Code, repositories.Body.String())
	}
	repoBody := decodeJSON[struct {
		Repositories []Repository `json:"repositories"`
	}](t, repositories)
	want := Repository{
		ID: "repo-1", Name: "widgets", ProjectID: "project-1", ProjectName: "Platform",
		DefaultBranch: "main", WebURL: "https://dev.azure.com/acme/Platform/_git/widgets",
	}
	if len(repoBody.Repositories) != 1 || repoBody.Repositories[0] != want {
		t.Fatalf("repositories = %+v, want [%+v]", repoBody.Repositories, want)
	}
	if noProject := performRequest(t, router, http.MethodGet,
		"/api/v1/azure-devops/repositories?workspace_id=ws-1", nil); noProject.Code != http.StatusBadRequest {
		t.Fatalf("GET /repositories without project = %d: %s", noProject.Code, noProject.Body.String())
	}

	branches := performRequest(t, router, http.MethodGet,
		"/api/v1/azure-devops/branches?workspace_id=ws-1&organization=acme&project=project-1&repository=repo-1", nil)
	if branches.Code != http.StatusOK {
		t.Fatalf("GET /branches = %d: %s", branches.Code, branches.Body.String())
	}
	branchBody := decodeJSON[struct {
		Branches []Branch `json:"branches"`
	}](t, branches)
	if len(branchBody.Branches) != 2 || branchBody.Branches[0].Name != "main" ||
		branchBody.Branches[1].Name != "feature/azure" {
		t.Fatalf("branches = %+v", branchBody.Branches)
	}
	if client.lastBranchArgs != [2]string{"project-1", "repo-1"} {
		t.Fatalf("client received %v", client.lastBranchArgs)
	}

	mismatched := performRequest(t, router, http.MethodGet,
		"/api/v1/azure-devops/branches?workspace_id=ws-1&organization=other&project=project-1&repository=repo-1", nil)
	if mismatched.Code != http.StatusBadRequest {
		t.Fatalf("organization mismatch = %d: %s", mismatched.Code, mismatched.Body.String())
	}
}

func TestControllerPullRequestReadRoutes(t *testing.T) {
	client := &readsClient{
		pr: &PullRequest{
			ID: 42, Title: "Ship it", Status: "active", IsDraft: true,
			SourceBranch: "feature/azure", TargetBranch: "main",
			ProjectID: "project-1", RepositoryID: "repo-1",
			WebURL: "https://dev.azure.com/acme/Platform/_git/widgets/pullrequest/42",
		},
		page: &PullRequestPage{Items: []PullRequest{{ID: 42, Title: "Ship it"}}, Count: 1, Skip: 5, Top: 25},
	}
	router, service := newRouteFixture(t, client)
	seedRouteConfig(t, service)

	detail := performRequest(t, router, http.MethodGet,
		"/api/v1/azure-devops/pull-requests/project-1/repo-1/42?workspace_id=ws-1", nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("GET pull request = %d: %s", detail.Code, detail.Body.String())
	}
	got := decodeJSON[PullRequest](t, detail)
	if got.ID != 42 || got.Title != "Ship it" || got.Status != "active" || !got.IsDraft ||
		got.SourceBranch != "feature/azure" || got.TargetBranch != "main" ||
		got.ProjectID != "project-1" || got.RepositoryID != "repo-1" || got.WebURL == "" {
		t.Fatalf("pull request = %+v", got)
	}

	for _, id := range []string{"0", "-3", "abc"} {
		invalid := performRequest(t, router, http.MethodGet,
			"/api/v1/azure-devops/pull-requests/project-1/repo-1/"+id+"?workspace_id=ws-1", nil)
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("pull request id %q = %d: %s", id, invalid.Code, invalid.Body.String())
		}
		feedback := performRequest(t, router, http.MethodGet,
			"/api/v1/azure-devops/pull-requests/project-1/repo-1/"+id+"/feedback?workspace_id=ws-1", nil)
		if feedback.Code != http.StatusBadRequest {
			t.Fatalf("feedback for pull request id %q = %d: %s", id, feedback.Code, feedback.Body.String())
		}
	}

	listed := performRequest(t, router, http.MethodGet,
		"/api/v1/azure-devops/pull-requests?workspace_id=ws-1&project=project-1&repository=repo-1"+
			"&status=active&creator=creator-1&reviewer=reviewer-1"+
			"&source_branch=feature/azure&target_branch=main&skip=5&top=25", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("GET /pull-requests = %d: %s", listed.Code, listed.Body.String())
	}
	page := decodeJSON[PullRequestPage](t, listed)
	if page.Count != 1 || len(page.Items) != 1 || page.Skip != 5 || page.Top != 25 {
		t.Fatalf("page = %+v", page)
	}
	wantFilter := PullRequestFilter{
		ProjectID: "project-1", RepositoryID: "repo-1", Status: "active",
		CreatorID: "creator-1", ReviewerID: "reviewer-1",
		SourceBranch: "feature/azure", TargetBranch: "main", Skip: 5, Top: 25,
	}
	if client.lastFilter != wantFilter {
		t.Fatalf("filter sent to the client = %+v, want %+v", client.lastFilter, wantFilter)
	}
}
