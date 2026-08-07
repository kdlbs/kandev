package azuredevops

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

type fakeReadClient struct {
	invalidClient
	projects   []Project
	work       *WorkItemSearchResult
	pr         *PullRequest
	reviewers  []Reviewer
	threads    []Thread
	refs       []WorkItemRef
	policies   []PolicyEvaluation
	teams      []Team
	boards     []BoardReference
	snapshot   *BoardSnapshot
	updated    *BoardWorkItem
	assigned   *WorkItem
	assignment *WorkItemAssignmentRequest
	detail     *WorkItemDetail
	comments   *WorkItemCommentPage
	identity   *Identity
	lastFilter PullRequestFilter
	prPage     *PullRequestPage
}

func (f *fakeReadClient) ListPullRequests(_ context.Context, filter PullRequestFilter) (*PullRequestPage, error) {
	f.lastFilter = filter
	if f.prPage != nil {
		return f.prPage, nil
	}
	return &PullRequestPage{}, nil
}

func (f *fakeReadClient) TestAuth(context.Context) (*TestConnectionResult, error) {
	return &TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada"}, nil
}
func (f *fakeReadClient) ListProjects(context.Context) ([]Project, error)   { return f.projects, nil }
func (f *fakeReadClient) ListTeams(context.Context, string) ([]Team, error) { return f.teams, nil }
func (f *fakeReadClient) ListBoards(context.Context, string, string) ([]BoardReference, error) {
	return f.boards, nil
}
func (f *fakeReadClient) GetBoardSnapshot(context.Context, string, string, string) (*BoardSnapshot, error) {
	return f.snapshot, nil
}
func (f *fakeReadClient) UpdateBoardWorkItem(context.Context, string, string, string, int, BoardWorkItemUpdateRequest) (*BoardWorkItem, error) {
	if f.updated != nil {
		return f.updated, nil
	}
	return &BoardWorkItem{WorkItem: WorkItem{ID: 101, Revision: 8, Title: "Updated"}}, nil
}
func (f *fakeReadClient) UpdateWorkItem(_ context.Context, _ string, id int, request WorkItemAssignmentRequest) (*WorkItem, error) {
	f.assignment = &request
	if f.assigned != nil {
		copy := *f.assigned
		copy.ID = id
		return &copy, nil
	}
	return &WorkItem{ID: id, Revision: request.Revision + 1, AssignedTo: "Ada"}, nil
}
func (f *fakeReadClient) QueryWIQL(context.Context, string, string, int) (*WorkItemSearchResult, error) {
	return f.work, nil
}
func (f *fakeReadClient) GetWorkItemDetail(context.Context, string, int) (*WorkItemDetail, error) {
	return f.detail, nil
}
func (f *fakeReadClient) ListWorkItemComments(context.Context, string, int, string) (*WorkItemCommentPage, error) {
	return f.comments, nil
}
func (f *fakeReadClient) GetCurrentIdentity(context.Context) (*Identity, error) {
	return f.identity, nil
}
func (f *fakeReadClient) GetPullRequest(context.Context, string, string, int) (*PullRequest, error) {
	return f.pr, nil
}
func (f *fakeReadClient) ListReviewers(context.Context, string, string, int) ([]Reviewer, error) {
	return f.reviewers, nil
}
func (f *fakeReadClient) ListThreads(context.Context, string, string, int) ([]Thread, error) {
	return f.threads, nil
}
func (f *fakeReadClient) ListLinkedWorkItems(context.Context, string, string, int) ([]WorkItemRef, error) {
	return f.refs, nil
}
func (f *fakeReadClient) ListPolicyEvaluations(context.Context, string, int) ([]PolicyEvaluation, error) {
	return f.policies, nil
}

func newControllerFixture(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	client := &fakeReadClient{
		projects:  []Project{{ID: "project-1", Name: "Platform"}},
		work:      &WorkItemSearchResult{Items: []WorkItem{{ID: 101, Title: "Fix build"}}, Count: 1},
		pr:        &PullRequest{ID: 42, Title: "Ship it", ProjectID: "project-1", RepositoryID: "repo-1"},
		reviewers: []Reviewer{{Identity: Identity{ID: "u2", DisplayName: "Grace"}, Vote: 10, IsRequired: true}},
		threads:   []Thread{{ID: 7, Status: "active"}},
		refs:      []WorkItemRef{{ID: 101}},
		policies:  []PolicyEvaluation{{ID: "p1", Status: "approved", IsBlocking: true}},
		teams:     []Team{{ID: "team-1", Name: "Platform Team", ProjectID: "project-1"}},
		boards:    []BoardReference{{ID: "board-1", Name: "Stories"}},
		snapshot:  &BoardSnapshot{Board: Board{ID: "board-1", Name: "Stories"}, Items: []BoardWorkItem{{WorkItem: WorkItem{ID: 101, Title: "Fix build"}}}},
		updated:   &BoardWorkItem{WorkItem: WorkItem{ID: 101, Revision: 8, Title: "Updated"}},
		detail:    &WorkItemDetail{WorkItem: WorkItem{ID: 101, Revision: 8, Title: "Fix build", Description: "Useful detail"}},
		comments:  &WorkItemCommentPage{Comments: []WorkItemComment{{ID: 9, Content: "Looks good"}}},
		identity:  &Identity{ID: "me", DisplayName: "Ada"},
	}
	service := NewService(store, newFakeSecretStore(), func(*Config, string) Client { return client }, logger.Default())
	if _, err := service.SetConfigForWorkspace(context.Background(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, service, logger.Default())
	return router, service
}

func TestControllerGetWorkItemDetail(t *testing.T) {
	router, _ := newControllerFixture(t)
	response := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/work-items/101?workspace_id=ws-1&project=project-1", nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("Useful detail")) {
		t.Fatalf("detail response %d: %s", response.Code, response.Body.String())
	}
}

func TestControllerListWorkItemComments(t *testing.T) {
	router, _ := newControllerFixture(t)
	response := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/work-items/101/comments?workspace_id=ws-1&project=project-1&continuation_token=opaque", nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("Looks good")) {
		t.Fatalf("comments response %d: %s", response.Code, response.Body.String())
	}
}

func TestControllerGetIdentity(t *testing.T) {
	router, _ := newControllerFixture(t)
	response := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/identity?workspace_id=ws-1", nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("Ada")) {
		t.Fatalf("identity response %d: %s", response.Code, response.Body.String())
	}
}

func TestControllerBoardReads(t *testing.T) {
	router, _ := newControllerFixture(t)
	teams := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/teams?workspace_id=ws-1&project=project-1", nil)
	if teams.Code != http.StatusOK || !bytes.Contains(teams.Body.Bytes(), []byte("Platform Team")) {
		t.Fatalf("teams response %d: %s", teams.Code, teams.Body.String())
	}
	boards := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/boards?workspace_id=ws-1&project=project-1&team=team-1", nil)
	if boards.Code != http.StatusOK || !bytes.Contains(boards.Body.Bytes(), []byte("Stories")) {
		t.Fatalf("boards response %d: %s", boards.Code, boards.Body.String())
	}
	snapshot := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/boards/board-1?workspace_id=ws-1&project=project-1&team=team-1", nil)
	if snapshot.Code != http.StatusOK || !bytes.Contains(snapshot.Body.Bytes(), []byte("Fix build")) {
		t.Fatalf("snapshot response %d: %s", snapshot.Code, snapshot.Body.String())
	}
}

func TestControllerUpdateBoardWorkItem(t *testing.T) {
	router, _ := newControllerFixture(t)
	columnID := "column-active"
	updated := performRequest(t, router, http.MethodPatch, "/api/v1/azure-devops/boards/board-1/work-items/101?workspace_id=ws-1&project=project-1&team=team-1", map[string]any{
		"revision": 7, "columnId": columnID,
	})
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte("Updated")) {
		t.Fatalf("updated response %d: %s", updated.Code, updated.Body.String())
	}
}

func TestControllerRejectsUnsupportedBoardWorkItemFields(t *testing.T) {
	router, _ := newControllerFixture(t)
	updated := performRequest(t, router, http.MethodPatch, "/api/v1/azure-devops/boards/board-1/work-items/101?workspace_id=ws-1&project=project-1&team=team-1", map[string]any{
		"revision": 7, "title": "Cannot edit",
	})
	if updated.Code != http.StatusBadRequest {
		t.Fatalf("updated response %d: %s", updated.Code, updated.Body.String())
	}
	unknown := performRequest(t, router, http.MethodPatch, "/api/v1/azure-devops/boards/board-1/work-items/101?workspace_id=ws-1&project=project-1&team=team-1", map[string]any{
		"revision": 7, "columnId": "column-active", "description": "Cannot edit",
	})
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field response %d: %s", unknown.Code, unknown.Body.String())
	}
}

func TestControllerUpdatesWorkItemAssignmentWithoutEditingFields(t *testing.T) {
	router, _ := newControllerFixture(t)
	updated := performRequest(t, router, http.MethodPatch, "/api/v1/azure-devops/work-items/101?workspace_id=ws-1&project=project-1", map[string]any{
		"revision": 7, "assigneeAction": assignCurrentUserAction,
	})
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte("Ada")) {
		t.Fatalf("assignment response %d: %s", updated.Code, updated.Body.String())
	}
	invalid := performRequest(t, router, http.MethodPatch, "/api/v1/azure-devops/work-items/101?workspace_id=ws-1&project=project-1", map[string]any{
		"revision": 7, "title": "Cannot edit",
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("unsupported field response %d: %s", invalid.Code, invalid.Body.String())
	}
}

func TestControllerConfigAndWorkspaceScopedReads(t *testing.T) {
	router, _ := newControllerFixture(t)

	config := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/config?workspace_id=ws-1", nil)
	if config.Code != http.StatusOK || bytes.Contains(config.Body.Bytes(), []byte("\"pat\":")) {
		t.Fatalf("config response %d: %s", config.Code, config.Body.String())
	}
	projects := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/projects?workspace_id=ws-1", nil)
	if projects.Code != http.StatusOK || !bytes.Contains(projects.Body.Bytes(), []byte("Platform")) {
		t.Fatalf("projects response %d: %s", projects.Code, projects.Body.String())
	}
	search := performRequest(t, router, http.MethodPost, "/api/v1/azure-devops/work-items/search?workspace_id=ws-1", map[string]any{
		"project": "project-1", "wiql": "SELECT [System.Id] FROM WorkItems", "top": 20,
	})
	if search.Code != http.StatusOK || !bytes.Contains(search.Body.Bytes(), []byte("Fix build")) {
		t.Fatalf("search response %d: %s", search.Code, search.Body.String())
	}
}

func TestControllerWorkItemWatchCRUDAndReset(t *testing.T) {
	router, _ := newControllerFixture(t)
	path := "/api/v1/azure-devops/watches/work-items"
	query := "?workspace_id=ws-1"
	created := performRequest(t, router, http.MethodPost, path+query, map[string]any{
		"workflowId": "wf-1", "workflowStepId": "step-1", "projectId": "project-1",
		"wiql": "SELECT [System.Id] FROM WorkItems", "repositoryId": "repo-1",
		"baseBranch": "main", "agentProfileId": "agent-1", "executorProfileId": "executor-1",
		"prompt": "Fix {{title}}", "pollIntervalSeconds": 1,
	})
	if created.Code != http.StatusOK || !bytes.Contains(created.Body.Bytes(), []byte("project-1")) {
		t.Fatalf("create watch response %d: %s", created.Code, created.Body.String())
	}
	var watch WorkItemWatch
	if err := json.Unmarshal(created.Body.Bytes(), &watch); err != nil {
		t.Fatalf("decode watch: %v", err)
	}
	if watch.ID == "" || !watch.Enabled || watch.PollIntervalSeconds != minWatchPollIntervalSeconds {
		t.Fatalf("created watch = %+v", watch)
	}
	listed := performRequest(t, router, http.MethodGet, path+query, nil)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(watch.ID)) {
		t.Fatalf("list watch response %d: %s", listed.Code, listed.Body.String())
	}
	updated := performRequest(t, router, http.MethodPatch, path+"/"+watch.ID+query, map[string]any{"enabled": false})
	if updated.Code != http.StatusOK || bytes.Contains(updated.Body.Bytes(), []byte(`"enabled":true`)) {
		t.Fatalf("update watch response %d: %s", updated.Code, updated.Body.String())
	}
	preview := performRequest(t, router, http.MethodGet, path+"/"+watch.ID+"/reset/preview"+query, nil)
	if preview.Code != http.StatusOK || !bytes.Contains(preview.Body.Bytes(), []byte(`"taskCount":0`)) {
		t.Fatalf("reset preview response %d: %s", preview.Code, preview.Body.String())
	}
	reset := performRequest(t, router, http.MethodPost, path+"/"+watch.ID+"/reset"+query, nil)
	if reset.Code != http.StatusOK || !bytes.Contains(reset.Body.Bytes(), []byte(`"generation":2`)) {
		t.Fatalf("reset response %d: %s", reset.Code, reset.Body.String())
	}
}

func TestControllerPersistsWorkspaceSavedViews(t *testing.T) {
	router, _ := newControllerFixture(t)
	path := "/api/v1/azure-devops/views?workspace_id=ws-1"
	created := performRequest(t, router, http.MethodPut, path, map[string]any{
		"views": []map[string]any{{
			"id": "mine", "kind": "work_item", "label": "Assigned to me",
			"projectId": "project-1", "wiql": "SELECT [System.Id] FROM WorkItems", "top": 50,
		}},
	})
	if created.Code != http.StatusOK || !bytes.Contains(created.Body.Bytes(), []byte("Assigned to me")) {
		t.Fatalf("save views response %d: %s", created.Code, created.Body.String())
	}
	loaded := performRequest(t, router, http.MethodGet, path, nil)
	if loaded.Code != http.StatusOK || !bytes.Contains(loaded.Body.Bytes(), []byte("Assigned to me")) {
		t.Fatalf("get views response %d: %s", loaded.Code, loaded.Body.String())
	}
	invalid := performRequest(t, router, http.MethodPut, path, map[string]any{
		"views": []map[string]any{{
			"id": "bad", "kind": "pull_request", "label": "Bad", "projectId": "project-1",
		}},
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid views response %d: %s", invalid.Code, invalid.Body.String())
	}
}

func TestControllerPullRequestFeedbackAggregation(t *testing.T) {
	router, _ := newControllerFixture(t)
	path := "/api/v1/azure-devops/pull-requests/project-1/repo-1/42/feedback?workspace_id=ws-1"
	response := performRequest(t, router, http.MethodGet, path, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("feedback response %d: %s", response.Code, response.Body.String())
	}
	var feedback PullRequestFeedback
	if err := json.Unmarshal(response.Body.Bytes(), &feedback); err != nil {
		t.Fatalf("decode feedback: %v", err)
	}
	if feedback.PullRequest.ID != 42 || feedback.ReviewState != "approved" || feedback.PolicyState != "success" || len(feedback.Threads) != 1 {
		t.Fatalf("feedback = %+v", feedback)
	}
}

func TestPullRequestSearchResolvesCurrentUserSentinel(t *testing.T) {
	router, service := newControllerFixture(t)
	client := service.clientFn(nil, "").(*fakeReadClient)
	response := performRequest(
		t, router, http.MethodGet,
		"/api/v1/azure-devops/pull-requests?workspace_id=ws-1&project=project-1&repository=repo-1&reviewer=%40me", nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("pull request response %d: %s", response.Code, response.Body.String())
	}
	if client.lastFilter.ReviewerID != "me" {
		t.Fatalf("reviewer filter = %q, want authenticated identity", client.lastFilter.ReviewerID)
	}
}

func TestControllerRejectsMissingAndUnconfiguredWorkspace(t *testing.T) {
	router, _ := newControllerFixture(t)
	missing := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/projects", nil)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing workspace status = %d: %s", missing.Code, missing.Body.String())
	}
	unconfigured := performRequest(t, router, http.MethodGet, "/api/v1/azure-devops/projects?workspace_id=ws-2", nil)
	if unconfigured.Code != http.StatusServiceUnavailable || !bytes.Contains(unconfigured.Body.Bytes(), []byte("azure_devops_not_configured")) {
		t.Fatalf("unconfigured status = %d: %s", unconfigured.Code, unconfigured.Body.String())
	}
}

func TestControllerTaskPullRequestAssociationRoutes(t *testing.T) {
	router, service := newControllerFixture(t)
	service.SetRepositoryLookup(fakeAzureRepositoryLookup{binding: &RepositoryBinding{
		WorkspaceID: "ws-1", Provider: RepositoryProvider,
		ProviderOwner: "project-1", ProviderRepoID: "repo-1",
	}})
	if _, err := service.Store().db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := service.Store().db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	body := map[string]any{"repositoryId": "repository-1", "pullRequestId": 42}
	associated := performRequest(
		t, router, http.MethodPost,
		"/api/v1/azure-devops/tasks/task-1/pull-requests?workspace_id=ws-1", body,
	)
	if associated.Code != http.StatusOK || !bytes.Contains(associated.Body.Bytes(), []byte("Ship it")) {
		t.Fatalf("associate response %d: %s", associated.Code, associated.Body.String())
	}
	synced := performRequest(
		t, router, http.MethodPost,
		"/api/v1/azure-devops/tasks/task-1/pull-requests/sync?workspace_id=ws-1", body,
	)
	if synced.Code != http.StatusOK {
		t.Fatalf("sync response %d: %s", synced.Code, synced.Body.String())
	}
	listed := performRequest(
		t, router, http.MethodGet,
		"/api/v1/azure-devops/workspaces/ws-1/task-prs", nil,
	)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"taskPrs":{"task-1"`)) {
		t.Fatalf("list response %d: %s", listed.Code, listed.Body.String())
	}
}

func TestControllerTaskPullRequestAssociationRejectsInvalidInput(t *testing.T) {
	router, service := newControllerFixture(t)
	service.SetRepositoryLookup(fakeAzureRepositoryLookup{binding: &RepositoryBinding{
		WorkspaceID: "ws-1", Provider: "github",
		ProviderOwner: "project-1", ProviderRepoID: "repo-1",
	}})
	response := performRequest(
		t, router, http.MethodPost,
		"/api/v1/azure-devops/tasks/task-1/pull-requests?workspace_id=ws-1",
		map[string]any{"repositoryId": "repository-1", "pullRequestId": 42},
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid association response %d: %s", response.Code, response.Body.String())
	}
}

func performRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}
