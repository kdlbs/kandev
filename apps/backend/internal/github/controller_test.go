package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestValidGitHubCallbackState(t *testing.T) {
	valid := base64.RawURLEncoding.EncodeToString(make([]byte, oauthRandomBytes))
	for name, test := range map[string]struct {
		state string
		want  bool
	}{
		"valid":              {state: valid, want: true},
		"empty":              {state: "", want: false},
		"malformed":          {state: "%%%", want: false},
		"wrong decoded size": {state: base64.RawURLEncoding.EncodeToString([]byte("short")), want: false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := validGitHubCallbackState(test.state); got != test.want {
				t.Fatalf("validGitHubCallbackState(%q) = %v, want %v", test.state, got, test.want)
			}
		})
	}
}

// stubClient implements Client with no-op defaults; override fields as needed.
type stubClient struct {
	getPRFunc             func(ctx context.Context, owner, repo string, number int) (*PR, error)
	getIssueFunc          func(ctx context.Context, owner, repo string, number int) (*Issue, error)
	mergePRFn             func(ctx context.Context, owner, repo string, number int, mergeMethod string) error
	getRepoMergeMethodsFn func() (RepoMergeMethods, error)
	requestReviewersFn    func(ctx context.Context, owner, repo string, number int, reviewers []string) error
}

func (s *stubClient) IsAuthenticated(context.Context) (bool, error) { return true, nil }
func (s *stubClient) GetAuthenticatedUser(context.Context) (string, error) {
	return "test-user", nil
}
func (s *stubClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	if s.getPRFunc != nil {
		return s.getPRFunc(ctx, owner, repo, number)
	}
	return nil, fmt.Errorf("not implemented")
}
func (s *stubClient) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	if s.getIssueFunc != nil {
		return s.getIssueFunc(ctx, owner, repo, number)
	}
	return nil, fmt.Errorf("not implemented")
}
func (s *stubClient) FindPRByBranch(context.Context, string, string, string) (*PR, error) {
	return nil, nil
}
func (s *stubClient) ListAuthoredPRs(context.Context, string, string) ([]*PR, error) {
	return nil, nil
}
func (s *stubClient) SearchPRs(context.Context, string, string) ([]*PR, error) {
	return nil, nil
}
func (s *stubClient) SearchPRsPaged(context.Context, string, string, int, int) (*PRSearchPage, error) {
	return &PRSearchPage{PRs: []*PR{}}, nil
}
func (s *stubClient) ListReviewRequestedPRs(context.Context, string, string, string) ([]*PR, error) {
	return nil, nil
}
func (s *stubClient) ListUserOrgs(context.Context) ([]GitHubOrg, error) { return nil, nil }
func (s *stubClient) SearchOrgRepos(context.Context, string, string, int) ([]GitHubRepo, error) {
	return nil, nil
}
func (s *stubClient) ListUserRepos(context.Context, string, int) ([]GitHubRepo, error) {
	return nil, nil
}
func (s *stubClient) ListAccessibleRepos(context.Context, string, int) ([]GitHubRepo, error) {
	return nil, nil
}
func (s *stubClient) HasRepositoryAccess(context.Context, string, string) (bool, error) {
	return true, nil
}
func (s *stubClient) ListPRReviews(context.Context, string, string, int) ([]PRReview, error) {
	return nil, nil
}
func (s *stubClient) ListPRComments(context.Context, string, string, int, *time.Time) ([]PRComment, error) {
	return nil, nil
}
func (s *stubClient) ListCheckRuns(context.Context, string, string, string) ([]CheckRun, error) {
	return nil, nil
}
func (s *stubClient) GetPRFeedback(context.Context, string, string, int) (*PRFeedback, error) {
	return nil, nil
}
func (s *stubClient) ListPRFiles(context.Context, string, string, int) ([]PRFile, error) {
	return nil, nil
}
func (s *stubClient) ListPRCommits(context.Context, string, string, int) ([]PRCommitInfo, error) {
	return nil, nil
}
func (s *stubClient) SubmitReview(context.Context, string, string, int, string, string) error {
	return nil
}

func (s *stubClient) RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error {
	if s.requestReviewersFn != nil {
		return s.requestReviewersFn(ctx, owner, repo, number, reviewers)
	}
	return nil
}
func (s *stubClient) MergePR(ctx context.Context, owner, repo string, number int, mergeMethod string) error {
	if s.mergePRFn != nil {
		return s.mergePRFn(ctx, owner, repo, number, mergeMethod)
	}
	return nil
}
func (s *stubClient) ListRepoBranches(context.Context, string, string) ([]RepoBranch, error) {
	return nil, nil
}
func (s *stubClient) GetRepoMergeMethods(context.Context, string, string) (RepoMergeMethods, error) {
	if s.getRepoMergeMethodsFn != nil {
		return s.getRepoMergeMethodsFn()
	}
	return RepoMergeMethods{Merge: true, Squash: true, Rebase: true}, nil
}
func (s *stubClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	return nil, nil
}
func (s *stubClient) ListIssuesPaged(context.Context, string, string, int, int) (*IssueSearchPage, error) {
	return &IssueSearchPage{Issues: []*Issue{}}, nil
}
func (s *stubClient) GetIssueState(context.Context, string, string, int) (string, error) {
	return defaultPRState, nil
}
func (s *stubClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	return nil, nil
}
func (s *stubClient) CreateGist(context.Context, CreateGistInput) (*GistResponse, error) {
	return nil, nil
}
func (s *stubClient) DeleteGist(context.Context, string) error { return nil }
func (s *stubClient) ListRepoDirectory(context.Context, string, string, string, string) ([]RepoContentEntry, error) {
	return nil, nil
}
func (s *stubClient) GetRepoFileContent(context.Context, string, string, string, string) ([]byte, error) {
	return nil, nil
}

func newControllerTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	return log
}

type controllerTestConnectionReader struct {
	connection *WorkspaceConnection
}

func (r controllerTestConnectionReader) GetWorkspaceConnection(
	context.Context,
	string,
) (*WorkspaceConnection, error) {
	return r.connection, nil
}

func (controllerTestConnectionReader) GetUserConnection(
	context.Context,
	string,
	string,
) (*UserConnection, error) {
	return nil, nil
}

func useControllerTestWorkspace(router *gin.Engine) {
	router.Use(func(ctx *gin.Context) {
		query := ctx.Request.URL.Query()
		if query.Get("workspace_id") == "" && ctx.GetHeader("X-Test-Omit-Workspace") == "" {
			query.Set("workspace_id", "ws-1")
			ctx.Request.URL.RawQuery = query.Encode()
		}
		ctx.Next()
	})
}

func configureControllerTestResolver(svc *Service, client Client, workspaceID string) {
	resolver := NewCredentialResolver(controllerTestConnectionReader{connection: &WorkspaceConnection{
		WorkspaceID:          workspaceID,
		Source:               ConnectionSourceLegacyShared,
		GitHubHost:           defaultGitHubHost,
		Status:               ConnectionStatusActive,
		CredentialGeneration: 1,
	}}, nil)
	resolver.SetLegacyFactory(func(context.Context) (Client, string, error) {
		if client == nil {
			return nil, AuthMethodNone, ErrGitHubNotConfigured
		}
		return client, AuthMethodPAT, nil
	})
	svc.resolver = resolver
}

func setupControllerTest(client Client) (*gin.Engine, *Controller) {
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	svc := NewService(client, "pat", nil, nil, nil, log)
	configureControllerTestResolver(svc, client, "ws-1")
	ctrl := NewController(svc, log)
	router := gin.New()
	useControllerTestWorkspace(router)
	ctrl.RegisterHTTPRoutes(router)
	return router, ctrl
}

type staticPromptResolver struct {
	content string
}

func (r staticPromptResolver) ResolvePromptContent(context.Context, string, string) string {
	if r.content == "" {
		return "default auto-fix prompt"
	}
	return r.content
}

func setupControllerStoreTest(t *testing.T) (*gin.Engine, *Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmp, "github.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	if _, err := sqlxDB.Exec(`CREATE TABLE workspaces (
		id TEXT PRIMARY KEY
	);
	INSERT INTO workspaces (id) VALUES ('ws-1');
	CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT,
		title TEXT NOT NULL DEFAULT '',
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		archived_at DATETIME
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	log := newControllerTestLogger()
	svc := NewService(&stubClient{}, "pat", nil, store, nil, log)
	if err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID:          "ws-1",
		Source:               ConnectionSourceLegacyShared,
		GitHubHost:           defaultGitHubHost,
		Status:               ConnectionStatusActive,
		CredentialGeneration: 1,
	}); err != nil {
		t.Fatalf("seed workspace connection: %v", err)
	}
	svc.resolver.SetLegacyFactory(func(context.Context) (Client, string, error) {
		return svc.client, AuthMethodPAT, nil
	})
	svc.SetPromptResolver(staticPromptResolver{content: "resolved default prompt"})
	ctrl := NewController(svc, log)
	router := gin.New()
	useControllerTestWorkspace(router)
	ctrl.RegisterHTTPRoutes(router)
	return router, store
}

func TestCredentialBrokerReadinessUsesExactResolveRoute(t *testing.T) {
	router, controller := setupControllerTest(&stubClient{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/github/credentials/resolve", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured readiness status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}

	controller.service.SetCredentialBroker(&CredentialBroker{})
	request = httptest.NewRequest(http.MethodGet, "/api/v1/github/credentials/resolve", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("configured readiness response = %d headers=%v", response.Code, response.Header())
	}
}

func TestHttpDeleteTaskPRDetachesOnlyTheRequestedWorkspaceAssociation(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	ctx := context.Background()
	first := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "demo",
		PRNumber: 10, PRURL: "https://github.com/acme/demo/pull/10", PRTitle: "old", HeadBranch: "old",
		BaseBranch: "main", State: "merged", CreatedAt: time.Now().UTC(),
	}
	second := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "demo",
		PRNumber: 11, PRURL: "https://github.com/acme/demo/pull/11", PRTitle: "new", HeadBranch: "new",
		BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC().Add(time.Second),
	}
	if err := store.CreateTaskPR(ctx, first); err != nil {
		t.Fatalf("create first PR: %v", err)
	}
	if err := store.CreateTaskPR(ctx, second); err != nil {
		t.Fatalf("create second PR: %v", err)
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/github/task-prs/"+first.ID+"?workspace_id=ws-1", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Deleted bool `json:"deleted"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if !body.Deleted {
		t.Fatal("delete response deleted=false, want true")
	}
	active, err := store.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list active PRs: %v", err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active PRs = %+v, want second association only", active)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/github/task-prs/"+second.ID+"?workspace_id=other", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace delete status = %d, want 404", response.Code)
	}
}

func TestHttpListTaskIssues_WorkspaceScopedMetadataLinks(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	rows := []struct {
		id          string
		workspaceID string
		title       string
		metadata    string
	}{
		{
			id:          "manual-task",
			workspaceID: "ws-1",
			title:       "Manual issue task",
			metadata:    `{"issue_url":"https://github.com/kdlbs/kandev/issues/1672","issue_number":1672,"issue_owner":"kdlbs","issue_repo":"kandev","github_issue_linked":true}`,
		},
		{
			id:          "watch-task",
			workspaceID: "ws-1",
			title:       "Watched issue task",
			metadata:    `{"issue_url":"https://github.com/kdlbs/kandev/issues/1672","issue_number":1672,"issue_repo":"kdlbs/kandev","issue_watch_id":"watch-1"}`,
		},
		{
			id:          "other-workspace",
			workspaceID: "ws-2",
			title:       "Hidden task",
			metadata:    `{"issue_url":"https://github.com/kdlbs/kandev/issues/1672","issue_number":1672}`,
		},
		{
			id:          "invalid-metadata",
			workspaceID: "ws-1",
			title:       "Invalid task",
			metadata:    `{"issue_url":"https://gitlab.com/kdlbs/kandev/issues/1672","issue_number":1672}`,
		},
	}
	for _, row := range rows {
		if _, err := store.db.Exec(
			`INSERT INTO tasks (id, workspace_id, title, metadata) VALUES (?, ?, ?, ?)`,
			row.id, row.workspaceID, row.title, row.metadata,
		); err != nil {
			t.Fatalf("insert task %s: %v", row.id, err)
		}
	}
	if _, err := store.db.Exec(
		`INSERT INTO tasks (id, workspace_id, title, metadata, archived_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		"archived-task",
		"ws-1",
		"Archived issue task",
		`{"issue_url":"https://github.com/kdlbs/kandev/issues/1672","issue_number":1672}`,
	); err != nil {
		t.Fatalf("insert archived task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/task-issues?workspace_id=ws-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	type listedTaskIssue struct {
		TaskTitle   string `json:"task_title"`
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		IssueNumber int    `json:"issue_number"`
	}
	var got struct {
		TaskIssues map[string]listedTaskIssue `json:"task_issues"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.TaskIssues) != 2 {
		t.Fatalf("task issue count = %d, want 2: %+v", len(got.TaskIssues), got.TaskIssues)
	}
	if _, ok := got.TaskIssues["archived-task"]; ok {
		t.Fatalf("archived task should not be listed: %+v", got.TaskIssues)
	}
	manual := got.TaskIssues["manual-task"]
	if manual.TaskTitle != "Manual issue task" || manual.Owner != "kdlbs" ||
		manual.Repo != "kandev" || manual.IssueNumber != 1672 {
		t.Fatalf("unexpected manual link: %+v", manual)
	}
	watch := got.TaskIssues["watch-task"]
	if watch.TaskTitle != "Watched issue task" || watch.Owner != manual.Owner ||
		watch.Repo != manual.Repo || watch.IssueNumber != manual.IssueNumber {
		t.Fatalf("unexpected watch link: %+v", watch)
	}
}

func TestHttpListTaskIssues_RequiresWorkspace(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/task-issues", nil)
	req.Header.Set("X-Test-Omit-Workspace", "true")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestHttpTriggerReviewWatchPublishesNewPREvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmp, "github.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	client := NewMockClient()
	client.AddPR(&PR{
		Number:     42,
		Title:      "Review me",
		HTMLURL:    "https://github.com/acme/widget/pull/42",
		State:      "open",
		HeadBranch: "feature/review",
		BaseBranch: "main",
		RepoOwner:  "acme",
		RepoName:   "widget",
		RequestedReviewers: []RequestedReviewer{
			{Login: "test-user", Type: "user"},
		},
	})

	watch := &ReviewWatch{
		WorkspaceID:         "ws-1",
		WorkflowID:          "wf-1",
		WorkflowStepID:      "step-1",
		Repos:               []RepoFilter{{Owner: "acme", Name: "widget"}},
		ReviewScope:         ReviewScopeUserAndTeams,
		Enabled:             true,
		PollIntervalSeconds: defaultWatchPollIntervalSec,
		CleanupPolicy:       CleanupPolicyNever,
	}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}

	log := newControllerTestLogger()
	eb := &mockEventBus{}
	svc := NewService(client, "pat", nil, store, eb, log)
	configureControllerTestResolver(svc, client, "ws-1")
	router := gin.New()
	NewController(svc, log).RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/watches/review/"+watch.ID+"/trigger?workspace_id=ws-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		NewPRs      int `json:"new_prs"`
		NewPRsFound int `json:"new_prs_found"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode trigger response: %v", err)
	}
	if body.NewPRs != 1 || body.NewPRsFound != 1 {
		t.Fatalf("response counts = %+v, want both counts 1", body)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Fatalf("published events = %d, want 1", got)
	}
}

func TestHttpUpdateReviewWatchReturnsUpdatedWatch(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	watch := &ReviewWatch{
		WorkspaceID:         "ws-1",
		WorkflowID:          "wf-1",
		WorkflowStepID:      "step-1",
		Enabled:             true,
		PollIntervalSeconds: defaultWatchPollIntervalSec,
		CleanupPolicy:       CleanupPolicyNever,
	}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/github/watches/review/"+watch.ID+"?workspace_id=ws-1",
		strings.NewReader(`{"enabled":false}`),
	)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var updated ReviewWatch
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.ID != watch.ID || updated.Enabled {
		t.Fatalf("updated watch = %+v, want id %q and enabled=false", updated, watch.ID)
	}
}

func TestHttpListReviewWatchesRequiresWorkspaceForRequests(t *testing.T) {
	store := newTestStore(t)
	watch := &ReviewWatch{WorkspaceID: "victim-workspace", Prompt: "secret review prompt"}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, newControllerTestLogger())
	router := gin.New()
	NewController(svc, newControllerTestLogger()).RegisterHTTPRoutes(router)

	foreign := httptest.NewRequest(http.MethodGet, "/api/v1/github/watches/review", nil)
	foreign = foreign.WithContext(authn.WithIdentity(foreign.Context(), authn.Identity{
		UserID: "foreign-member", Role: authn.RoleMember,
	}))
	foreignResponse := httptest.NewRecorder()
	router.ServeHTTP(foreignResponse, foreign)
	if foreignResponse.Code != http.StatusBadRequest || strings.Contains(foreignResponse.Body.String(), watch.Prompt) {
		t.Fatalf("foreign response = %d %s, want no unscoped watch disclosure", foreignResponse.Code, foreignResponse.Body.String())
	}

	for _, identity := range []authn.Identity{
		{UserID: "owner-member", Role: authn.RoleMember},
		{UserID: "owner-admin", Role: authn.RoleAdmin},
	} {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/github/watches/review?workspace_id=victim-workspace", nil)
		req = req.WithContext(authn.WithIdentity(req.Context(), identity))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), watch.Prompt) {
			t.Fatalf("%s response = %d %s, want scoped watch", identity.Role, response.Code, response.Body.String())
		}
	}
}

func TestHttpReviewWatchMutationMapsForeignWorkspaceToNotFound(t *testing.T) {
	store := newTestStore(t)
	watch := &ReviewWatch{WorkspaceID: "victim-workspace", Prompt: "secret review prompt"}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, newControllerTestLogger())
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return repoerrors.ErrWorkspaceNotFound })
	router := gin.New()
	NewController(svc, newControllerTestLogger()).RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/github/watches/review/"+watch.ID+"?workspace_id=victim-workspace", nil)
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "foreign", Role: authn.RoleMember}))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "workspace") {
		t.Fatalf("foreign mutation response = %d %s, want sanitized 404", response.Code, response.Body.String())
	}
}

func TestHTTPWatchListsRequireWorkspace(t *testing.T) {
	store := newTestStore(t)
	issueWatch := &IssueWatch{WorkspaceID: "victim-workspace", Prompt: "secret issue prompt"}
	if err := store.CreateIssueWatch(context.Background(), issueWatch); err != nil {
		t.Fatal(err)
	}
	prWatch := &PRWatch{WorkspaceID: "victim-workspace", SessionID: "session-1", TaskID: "task-1", Owner: "acme", Repo: "private", Branch: "main"}
	if err := store.CreatePRWatch(context.Background(), prWatch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, prWatch.TaskID, prWatch.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, newControllerTestLogger())
	router := gin.New()
	NewController(svc, newControllerTestLogger()).RegisterHTTPRoutes(router)

	for _, path := range []string{"/api/v1/github/watches/issue", "/api/v1/github/watches/pr"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "foreign", Role: authn.RoleMember}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "secret issue prompt") || strings.Contains(response.Body.String(), "private") {
			t.Fatalf("GET %s = %d %s, want no unscoped disclosure", path, response.Code, response.Body.String())
		}
	}

	for _, tc := range []struct {
		path   string
		secret string
	}{
		{"/api/v1/github/watches/issue?workspace_id=victim-workspace", "secret issue prompt"},
		{"/api/v1/github/watches/pr?workspace_id=victim-workspace", "private"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "workspace-member", Role: authn.RoleMember}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), tc.secret) {
			t.Fatalf("GET %s = %d %s, want scoped watch", tc.path, response.Code, response.Body.String())
		}
	}
}

func TestWatchHTTPAndWSMutationsHideForeignRows(t *testing.T) {
	store := newTestStore(t)
	issueWatch := &IssueWatch{WorkspaceID: "victim-workspace"}
	if err := store.CreateIssueWatch(context.Background(), issueWatch); err != nil {
		t.Fatal(err)
	}
	prWatch := &PRWatch{WorkspaceID: "victim-workspace", SessionID: "session-1", TaskID: "task-1", Owner: "acme", Repo: "private", Branch: "main"}
	if err := store.CreatePRWatch(context.Background(), prWatch); err != nil {
		t.Fatal(err)
	}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, newControllerTestLogger())
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return repoerrors.ErrWorkspaceNotFound })
	router := gin.New()
	NewController(svc, newControllerTestLogger()).RegisterHTTPRoutes(router)

	for _, path := range []string{
		"/api/v1/github/watches/issue/" + issueWatch.ID + "?workspace_id=victim-workspace",
		"/api/v1/github/watches/pr/" + prWatch.ID + "?workspace_id=victim-workspace",
	} {
		req := httptest.NewRequest(http.MethodDelete, path, nil)
		req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "foreign", Role: authn.RoleMember}))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "workspace") {
			t.Fatalf("DELETE %s = %d %s, want sanitized 404", path, response.Code, response.Body.String())
		}
	}

	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "foreign", Role: authn.RoleMember})
	for _, tc := range []struct {
		name    string
		handler func(context.Context, *ws.Message) (*ws.Message, error)
		payload map[string]any
	}{
		{"issue-list", wsListIssueWatches(svc, newControllerTestLogger()), map[string]any{"workspace_id": "victim-workspace"}},
		{"issue-delete", wsDeleteIssueWatch(svc, newControllerTestLogger()), map[string]any{"id": issueWatch.ID}},
		{"pr-list", wsListPRWatches(svc, newControllerTestLogger()), map[string]any{"workspace_id": "victim-workspace"}},
		{"pr-delete", wsDeletePRWatch(svc, newControllerTestLogger()), map[string]any{"id": prWatch.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message, err := ws.NewRequest("request-1", "github.watch", tc.payload)
			if err != nil {
				t.Fatal(err)
			}
			response, err := tc.handler(ctx, message)
			if err != nil {
				t.Fatal(err)
			}
			if response.Type != ws.MessageTypeError || !strings.Contains(string(response.Payload), ws.ErrorCodeNotFound) {
				t.Fatalf("response = %#v, want sanitized NOT_FOUND", response)
			}
		})
	}
}

func waitForPublishedCount(t *testing.T, eb *mockEventBus, want int) {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		if got := eb.publishedCount(); got == want {
			return
		}
		select {
		case <-eb.publishedCh:
		case <-timeout:
			t.Fatalf("published events = %d, want %d", eb.publishedCount(), want)
		}
	}
}

type noopCascadeTaskDeleter struct{}

func (noopCascadeTaskDeleter) DeleteTaskTree(
	context.Context,
	string,
	bool,
) (*taskservice.CascadeOutcome, error) {
	return &taskservice.CascadeOutcome{}, nil
}

func TestResetReviewWatchPublishesNewPREvents(t *testing.T) {
	tmp := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmp, "github.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	client := NewMockClient()
	client.AddPR(&PR{
		Number:     42,
		Title:      "Review me again",
		HTMLURL:    "https://github.com/acme/widget/pull/42",
		State:      "open",
		HeadBranch: "feature/review-again",
		BaseBranch: "main",
		RepoOwner:  "acme",
		RepoName:   "widget",
		RequestedReviewers: []RequestedReviewer{
			{Login: "test-user", Type: "user"},
		},
	})

	watch := &ReviewWatch{
		WorkspaceID:         "ws-1",
		WorkflowID:          "wf-1",
		WorkflowStepID:      "step-1",
		Repos:               []RepoFilter{{Owner: "acme", Name: "widget"}},
		ReviewScope:         ReviewScopeUserAndTeams,
		Enabled:             true,
		PollIntervalSeconds: defaultWatchPollIntervalSec,
		CleanupPolicy:       CleanupPolicyNever,
	}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	if err := store.CreateReviewPRTask(context.Background(), &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      42,
		PRURL:         "https://github.com/acme/widget/pull/42",
		TaskID:        "task-old",
	}); err != nil {
		t.Fatalf("create review PR task: %v", err)
	}

	log := newControllerTestLogger()
	eb := &mockEventBus{publishedCh: make(chan struct{}, 1)}
	svc := NewService(client, "pat", nil, store, eb, log)
	configureControllerTestResolver(svc, client, "ws-1")
	svc.SetCascadeTaskDeleter(noopCascadeTaskDeleter{})

	deleted, err := svc.ResetReviewWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("reset review watch: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	waitForPublishedCount(t, eb, 1)
}

func TestHttpResetReviewWatchPublishesNewPREvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmp, "github.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	client := NewMockClient()
	client.AddPR(&PR{
		Number:     42,
		Title:      "Review me through reset",
		HTMLURL:    "https://github.com/acme/widget/pull/42",
		State:      "open",
		HeadBranch: "feature/review-reset",
		BaseBranch: "main",
		RepoOwner:  "acme",
		RepoName:   "widget",
		RequestedReviewers: []RequestedReviewer{
			{Login: "test-user", Type: "user"},
		},
	})

	watch := &ReviewWatch{
		WorkspaceID:         "ws-1",
		WorkflowID:          "wf-1",
		WorkflowStepID:      "step-1",
		Repos:               []RepoFilter{{Owner: "acme", Name: "widget"}},
		ReviewScope:         ReviewScopeUserAndTeams,
		Enabled:             true,
		PollIntervalSeconds: defaultWatchPollIntervalSec,
		CleanupPolicy:       CleanupPolicyNever,
	}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	if err := store.CreateReviewPRTask(context.Background(), &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      42,
		PRURL:         "https://github.com/acme/widget/pull/42",
		TaskID:        "task-old",
	}); err != nil {
		t.Fatalf("create review PR task: %v", err)
	}

	log := newControllerTestLogger()
	eb := &mockEventBus{publishedCh: make(chan struct{}, 1)}
	svc := NewService(client, "pat", nil, store, eb, log)
	configureControllerTestResolver(svc, client, "ws-1")
	svc.SetCascadeTaskDeleter(noopCascadeTaskDeleter{})
	router := gin.New()
	NewController(svc, log).RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/watches/review/"+watch.ID+"/reset?workspace_id=ws-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body struct {
		TasksDeleted int `json:"tasksDeleted"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if body.TasksDeleted != 1 {
		t.Fatalf("tasksDeleted = %d, want 1", body.TasksDeleted)
	}
	waitForPublishedCount(t, eb, 1)
}

func TestHttpLinkTaskIssue_SuccessAndInvalidJSON(t *testing.T) {
	router, ctrl := setupControllerTest(&stubClient{
		getIssueFunc: func(_ context.Context, owner, repo string, number int) (*Issue, error) {
			return &Issue{
				Number:    number,
				Title:     "Link existing task",
				HTMLURL:   fmt.Sprintf("https://github.com/%s/%s/issues/%d", owner, repo, number),
				RepoOwner: owner,
				RepoName:  repo,
			}, nil
		},
	})
	ctrl.service.SetTaskIssueStore(&fakeTaskIssueStore{
		task:  &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Metadata: map[string]interface{}{"keep": "me"}},
		repos: []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
		entities: map[string]*taskmodels.Repository{
			"repo-1": {ID: "repo-1", Provider: "github", ProviderOwner: "kdlbs", ProviderName: "kandev"},
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/tasks/task-1/issue", bytes.NewBufferString(`{"issue":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid JSON status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertJSONError(t, w.Body.Bytes(), "invalid payload")

	req = httptest.NewRequest(http.MethodPut, "/api/v1/github/tasks/task-1/issue", bytes.NewBufferString(`{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected link status 200, got %d: %s", w.Code, w.Body.String())
	}
	var got TaskIssueLinkResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.TaskID != "task-1" || got.Owner != "kdlbs" || got.Repo != "kandev" || got.IssueNumber != 1470 {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestHttpUnlinkTaskIssue_Success(t *testing.T) {
	router, ctrl := setupControllerTest(&stubClient{})
	store := &fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-1", Metadata: map[string]interface{}{
			taskMetaIssueURL:    "https://github.com/kdlbs/kandev/issues/1470",
			taskMetaIssueNumber: 1470,
			"keep":              "me",
		}},
	}
	ctrl.service.SetTaskIssueStore(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/github/tasks/task-1/issue", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected unlink status 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got["unlinked"] {
		t.Fatalf("unexpected response: %+v", got)
	}
	if _, ok := store.updated[taskMetaIssueURL]; ok {
		t.Fatalf("issue metadata should be removed: %+v", store.updated)
	}
}

func TestHttpUnlinkTaskIssue_TaskNotFound(t *testing.T) {
	router, ctrl := setupControllerTest(&stubClient{})
	ctrl.service.SetTaskIssueStore(&fakeTaskIssueStore{
		taskErr: fmt.Errorf("task lookup failed: %w", ErrTaskNotFound),
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/github/tasks/missing-task/issue", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected unlink task-not-found status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertJSONError(t, w.Body.Bytes(), "task not found")
}

func TestHttpLinkTaskIssue_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		client     Client
		store      TaskIssueStore
		body       string
		wantStatus int
		wantError  string
		wantCode   string
	}{
		{
			name:       "no client",
			client:     nil,
			store:      &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1"}},
			body:       `{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`,
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "GitHub is not configured. Connect GitHub in Settings > Integrations.",
			wantCode:   "github_not_configured",
		},
		{
			name:       "store unavailable",
			client:     &stubClient{},
			body:       `{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`,
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "GitHub task issue linking is temporarily unavailable. Please try again.",
			wantCode:   "github_task_issue_unavailable",
		},
		{
			name:       "invalid reference",
			client:     &stubClient{},
			store:      &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1"}},
			body:       `{"issue":"#1470"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid GitHub issue reference: owner and repo are required for issue numbers",
		},
		{
			name: "repository mismatch",
			client: &stubClient{getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
				return &Issue{Number: 1, HTMLURL: "https://github.com/other/repo/issues/1", RepoOwner: "other", RepoName: "repo"}, nil
			}},
			store: &fakeTaskIssueStore{
				task:  &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Metadata: map[string]interface{}{}},
				repos: []*taskmodels.TaskRepository{{RepositoryID: "repo-1"}},
				entities: map[string]*taskmodels.Repository{
					"repo-1": {ID: "repo-1", Provider: "github", ProviderOwner: "kdlbs", ProviderName: "kandev"},
				},
			},
			body:       `{"issue":"https://github.com/other/repo/issues/1"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "GitHub issue repository is not attached to task",
		},
		{
			name: "upstream not found",
			client: &stubClient{getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
				return nil, &GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: "/repos/kdlbs/kandev/issues/1470", Body: "private detail"}
			}},
			store:      &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Metadata: map[string]interface{}{}}},
			body:       `{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`,
			wantStatus: http.StatusNotFound,
			wantError:  "failed to fetch GitHub issue",
		},
		{
			name: "upstream unauthorized",
			client: &stubClient{getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
				return nil, &GitHubAPIError{StatusCode: http.StatusUnauthorized, Endpoint: "/repos/kdlbs/kandev/issues/1470", Body: "private detail"}
			}},
			store:      &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Metadata: map[string]interface{}{}}},
			body:       `{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`,
			wantStatus: http.StatusUnauthorized,
			wantError:  "failed to fetch GitHub issue",
		},
		{
			name: "upstream forbidden",
			client: &stubClient{getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
				return nil, &GitHubAPIError{StatusCode: http.StatusForbidden, Endpoint: "/repos/kdlbs/kandev/issues/1470", Body: "private detail"}
			}},
			store:      &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Metadata: map[string]interface{}{}}},
			body:       `{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`,
			wantStatus: http.StatusForbidden,
			wantError:  "failed to fetch GitHub issue",
		},
		{
			name: "task not found",
			client: &stubClient{getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
				return &Issue{Number: 1470, HTMLURL: "https://github.com/kdlbs/kandev/issues/1470", RepoOwner: "kdlbs", RepoName: "kandev"}, nil
			}},
			store:      &fakeTaskIssueStore{taskErr: fmt.Errorf("task lookup failed: %w", ErrTaskNotFound)},
			body:       `{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`,
			wantStatus: http.StatusNotFound,
			wantError:  "task not found",
		},
		{
			name: "default error",
			client: &stubClient{getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
				return &Issue{Number: 1470, HTMLURL: "https://github.com/kdlbs/kandev/issues/1470", RepoOwner: "kdlbs", RepoName: "kandev"}, nil
			}},
			store: &fakeTaskIssueStore{
				task:      &taskmodels.Task{ID: "task-1", WorkspaceID: "ws-1", Metadata: map[string]interface{}{}},
				updateErr: errors.New("database unavailable"),
			},
			body:       `{"issue":"https://github.com/kdlbs/kandev/issues/1470"}`,
			wantStatus: http.StatusInternalServerError,
			wantError:  "failed to link GitHub issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, ctrl := setupControllerTest(tt.client)
			if tt.store != nil {
				ctrl.service.SetTaskIssueStore(tt.store)
			}
			req := httptest.NewRequest(http.MethodPut, "/api/v1/github/tasks/task-1/issue", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			got := assertJSONError(t, w.Body.Bytes(), tt.wantError)
			if tt.wantCode != "" && got["code"] != tt.wantCode {
				t.Fatalf("code = %v, want %s; body=%+v", got["code"], tt.wantCode, got)
			}
		})
	}
}

func assertJSONError(t *testing.T, body []byte, want string) map[string]string {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode error body: %v; body=%s", err, string(body))
	}
	if got["error"] != want {
		t.Fatalf("error = %q, want %q; body=%+v", got["error"], want, got)
	}
	return got
}

func TestHttpTaskCIOptions_DefaultAndPatch(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	ctx := context.Background()
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "widget", PRNumber: 42,
		PRURL: "https://github.com/acme/widget/pull/42", PRTitle: "Fix",
		State: "open", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed task pr: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/tasks/task-1/ci-options", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got TaskCIOptionsResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode default response: %v", err)
	}
	if got.TaskID != "task-1" || got.AutoFixEnabled || got.AutoMergeEnabled {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if !got.UsingDefaultPrompt || got.EffectiveAutoFixPrompt != "resolved default prompt" {
		t.Fatalf("unexpected prompt fields: %+v", got)
	}
	if got.AutoFixMaxRounds != TaskCIAutoFixMaxRounds {
		t.Fatalf("auto-fix max rounds = %d, want %d", got.AutoFixMaxRounds, TaskCIAutoFixMaxRounds)
	}
	if len(got.PRStates) != 1 || got.PRStates[0].PRNumber != 42 {
		t.Fatalf("expected synthesized PR state for PR #42, got %+v", got.PRStates)
	}

	body := bytes.NewBufferString(`{"auto_fix_enabled":true,"auto_merge_enabled":true,"auto_fix_prompt_override":"Task prompt"}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/github/tasks/task-1/ci-options", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if !got.AutoFixEnabled || !got.AutoMergeEnabled || got.AutoFixPromptOverride == nil || *got.AutoFixPromptOverride != "Task prompt" {
		t.Fatalf("unexpected patched response: %+v", got)
	}
	if got.UsingDefaultPrompt || got.EffectiveAutoFixPrompt != "Task prompt" {
		t.Fatalf("expected task override to be effective, got %+v", got)
	}

	body = bytes.NewBufferString(`{"auto_fix_prompt_override":null}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/github/tasks/task-1/ci-options", body)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if got.AutoFixPromptOverride != nil || !got.UsingDefaultPrompt {
		t.Fatalf("expected reset to default prompt, got %+v", got)
	}
}

func TestHttpTaskCIOptions_RejectsLifecyclePromptOverrides(t *testing.T) {
	for _, field := range []string{
		"review_prompt_override",
		"merged_prompt_override",
		"closed_prompt_override",
	} {
		t.Run(field, func(t *testing.T) {
			router, _ := setupControllerStoreTest(t)
			body := bytes.NewBufferString(`{"` + field + `":"untrusted prompt"}`)
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/github/tasks/task-1/ci-options", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			assertJSONError(t, w.Body.Bytes(), "lifecycle prompt overrides are not supported")
		})
	}
}

func TestHttpPatchTaskCIOptions_EmptyPatchDoesNotUpdateRow(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	body := bytes.NewBufferString(`{"auto_fix_enabled":true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/github/tasks/task-1/ci-options", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var before TaskCIOptionsResponse
	if err := json.NewDecoder(w.Body).Decode(&before); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/github/tasks/task-1/ci-options", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var after TaskCIOptionsResponse
	if err := json.NewDecoder(w.Body).Decode(&after); err != nil {
		t.Fatalf("decode empty patch response: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("empty patch updated row timestamp: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestHttpPatchTaskCIOptions_InvalidPayload(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/github/tasks/task-1/ci-options", bytes.NewBufferString(`{"auto_fix_enabled":"yes"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpGetPRInfo_Success(t *testing.T) {
	sc := &stubClient{
		getPRFunc: func(_ context.Context, owner, repo string, number int) (*PR, error) {
			if owner != "acme" || repo != "widget" || number != 42 {
				t.Errorf("unexpected params: %s/%s#%d", owner, repo, number)
			}
			return &PR{Number: 42, Title: "feat: add widget"}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/42/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pr PR
	if err := json.NewDecoder(w.Body).Decode(&pr); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected PR number 42, got %d", pr.Number)
	}
	if pr.Title != "feat: add widget" {
		t.Errorf("expected title 'feat: add widget', got %q", pr.Title)
	}
}

func TestHttpGetPRInfo_InvalidNumber(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/abc/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHttpGetPRInfo_ServiceError(t *testing.T) {
	sc := &stubClient{
		getPRFunc: func(context.Context, string, string, int) (*PR, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/99/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHttpGetPRInfo_NoClient(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/repos/acme/widget/pulls/99" {
			t.Errorf("path = %q, want public PR endpoint", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != githubAccept {
			t.Errorf("Accept = %q, want %q", got, githubAccept)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubAPIVersion {
			t.Errorf("X-GitHub-Api-Version = %q, want %q", got, githubAPIVersion)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"number":99,"title":"Public widget","state":"open","user":{"login":"octo"},"head":{"ref":"feature/public","sha":"abc123"},"base":{"ref":"main"}}`))
	}))
	t.Cleanup(api.Close)

	originalAPIBase := anonymousAPIBase
	anonymousAPIBase = api.URL
	t.Cleanup(func() { anonymousAPIBase = originalAPIBase })

	router, _ := setupControllerTest(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/99/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got PR
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Number != 99 || got.Title != "Public widget" {
		t.Fatalf("PR = %#v, want public PR details", got)
	}
	if got.RepoOwner != "acme" || got.RepoName != "widget" || got.HeadBranch != "feature/public" || got.BaseBranch != "main" || got.AuthorLogin != "octo" {
		t.Fatalf("PR fallback fields = %#v", got)
	}
}

func TestHttpGetPRStatusesBatchAuthorizesBodyWorkspace(t *testing.T) {
	router, controller := setupControllerTest(&stubClient{})
	denied := errors.New("workspace not found")
	controller.service.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "victim-workspace" {
			t.Fatalf("workspaceID = %q, want victim workspace", workspaceID)
		}
		return denied
	})
	body := bytes.NewBufferString(`{"workspace_id":"victim-workspace","refs":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/statuses", body)
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "attacker", Role: authn.RoleMember}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), denied.Error()) {
		t.Fatalf("response = %d %s, want denied body-workspace access", w.Code, w.Body.String())
	}
}

func TestWSGetPRFeedbackAuthorizesPayloadWorkspace(t *testing.T) {
	service := NewService(&stubClient{}, AuthMethodPAT, nil, nil, nil, newControllerTestLogger())
	denied := errors.New("workspace not found")
	service.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID != "victim-workspace" {
			t.Fatalf("workspaceID = %q, want victim workspace", workspaceID)
		}
		return denied
	})
	message, err := ws.NewRequest("request-1", "github.pr_feedback.get", map[string]any{
		"workspace_id": "victim-workspace", "owner": "acme", "repo": "widget", "number": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "attacker", Role: authn.RoleMember})
	response, err := wsGetPRFeedback(service, newControllerTestLogger())(ctx, message)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if response.Type != ws.MessageTypeError || !strings.Contains(string(response.Payload), denied.Error()) {
		t.Fatalf("response = %#v, want denied workspace access", response)
	}
}

func TestHttpGetPRInfo_GitHubAPIErrorStatus(t *testing.T) {
	sc := &stubClient{
		getPRFunc: func(context.Context, string, string, int) (*PR, error) {
			return nil, &GitHubAPIError{
				StatusCode: http.StatusNotFound,
				Endpoint:   "/repos/acme/widget/pulls/99",
				Body:       "upstream error",
			}
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/widget/99/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHttpGetIssueInfo_Success(t *testing.T) {
	sc := &stubClient{
		getIssueFunc: func(_ context.Context, owner, repo string, number int) (*Issue, error) {
			if owner != "acme" || repo != "widget" || number != 1456 {
				t.Errorf("unexpected params: %s/%s#%d", owner, repo, number)
			}
			return &Issue{Number: 1456, Title: "fix remote picker"}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/issues/acme/widget/1456/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var issue Issue
	if err := json.NewDecoder(w.Body).Decode(&issue); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if issue.Number != 1456 {
		t.Errorf("expected issue number 1456, got %d", issue.Number)
	}
	if issue.Title != "fix remote picker" {
		t.Errorf("expected issue title, got %q", issue.Title)
	}
}

func TestHttpGetIssueInfo_InvalidNumber(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/issues/acme/widget/abc/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHttpGetIssueInfo_ServiceError(t *testing.T) {
	sc := &stubClient{
		getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/issues/acme/widget/99/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHttpGetIssueInfo_GitHubAPIErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		apiStatus  int
		wantStatus int
	}{
		{name: "not found", apiStatus: http.StatusNotFound, wantStatus: http.StatusNotFound},
		{name: "unauthorized", apiStatus: http.StatusUnauthorized, wantStatus: http.StatusUnauthorized},
		{name: "forbidden", apiStatus: http.StatusForbidden, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &stubClient{
				getIssueFunc: func(context.Context, string, string, int) (*Issue, error) {
					return nil, &GitHubAPIError{
						StatusCode: tt.apiStatus,
						Endpoint:   "/repos/acme/widget/issues/99",
						Body:       "upstream error",
					}
				},
			}
			router, _ := setupControllerTest(sc)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/github/issues/acme/widget/99/info", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestHttpMergePR_Success(t *testing.T) {
	var called struct {
		owner       string
		repo        string
		number      int
		mergeMethod string
	}
	sc := &stubClient{
		mergePRFn: func(_ context.Context, owner, repo string, number int, mergeMethod string) error {
			called.owner = owner
			called.repo = repo
			called.number = number
			called.mergeMethod = mergeMethod
			return nil
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"merge_method":"squash"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if called.owner != "acme" || called.repo != "widget" || called.number != 42 || called.mergeMethod != "squash" {
		t.Errorf("unexpected MergePR args: %+v", called)
	}
}

func TestHttpMergePR_InvalidMethod(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{})

	body := bytes.NewBufferString(`{"merge_method":"bogus"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHttpMergePR_EmptyBody_ResolvesToAllowedMethod(t *testing.T) {
	// Empty merge_method must NOT propagate to GitHub — that would let
	// GitHub default to "merge" and 405 on squash-only repos. The service
	// should resolve to the first allowed method instead.
	var gotMethod string
	sc := &stubClient{
		mergePRFn: func(_ context.Context, _, _ string, _ int, mergeMethod string) error {
			gotMethod = mergeMethod
			return nil
		},
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			// Squash-only repo (the case that surfaced this bug).
			return RepoMergeMethods{Squash: true}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotMethod != "squash" {
		t.Errorf("expected merge_method=squash, got %q", gotMethod)
	}
}

func TestHttpMergePR_ExplicitMethod_Passthrough(t *testing.T) {
	// When the user picks a method from the dropdown, the service must
	// NOT second-guess it via the repo lookup.
	var gotMethod string
	var lookupCalls int
	sc := &stubClient{
		mergePRFn: func(_ context.Context, _, _ string, _ int, mergeMethod string) error {
			gotMethod = mergeMethod
			return nil
		},
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			lookupCalls++
			return RepoMergeMethods{Merge: true}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"merge_method":"rebase"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotMethod != "rebase" {
		t.Errorf("expected merge_method=rebase, got %q", gotMethod)
	}
	if lookupCalls != 0 {
		t.Errorf("expected no merge-methods lookup for explicit pin, got %d", lookupCalls)
	}
}

func TestHttpMergePR_EmptyBody_LookupFails_FallsBackToGitHubDefault(t *testing.T) {
	// If the repo lookup itself errors, we still attempt the merge with an
	// empty method and let GitHub surface whatever error it surfaces — better
	// than refusing to merge because of an unrelated lookup failure.
	var gotMethod string
	sc := &stubClient{
		mergePRFn: func(_ context.Context, _, _ string, _ int, mergeMethod string) error {
			gotMethod = mergeMethod
			return nil
		},
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{}, fmt.Errorf("rate limited")
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotMethod != "" {
		t.Errorf("expected empty merge_method (fall back to GitHub default), got %q", gotMethod)
	}
}

func TestHttpGetRepoMergeMethods_OK(t *testing.T) {
	sc := &stubClient{
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{Squash: true, Rebase: true}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods?workspace_id=ws-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got RepoMergeMethods
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := RepoMergeMethods{Squash: true, Rebase: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestHttpGetRepoMergeMethods_NoClient_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	// nil client triggers ErrNoClient via Service.GetRepoMergeMethods.
	svc := NewService(nil, "none", nil, nil, nil, log)
	ctrl := NewController(svc, log)
	router := gin.New()
	ctrl.RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods?workspace_id=ws-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpGetRepoMergeMethods_NotFound_Returns404(t *testing.T) {
	sc := &stubClient{
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{}, &GitHubAPIError{StatusCode: http.StatusNotFound, Endpoint: "/repos/acme/widget"}
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpGetRepoMergeMethods_OtherError_Returns500(t *testing.T) {
	sc := &stubClient{
		getRepoMergeMethodsFn: func() (RepoMergeMethods, error) {
			return RepoMergeMethods{}, fmt.Errorf("unexpected upstream failure")
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos/acme/widget/merge-methods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpMergePR_MalformedJSON(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{})

	// Truncated JSON: parser fails with a non-EOF error, which must now
	// surface as 400 rather than silently falling through to the default
	// merge method.
	body := bytes.NewBufferString(`{"merge_method":"squash"`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpMergePR_NoClient_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	// nil client triggers ErrNoClient via Service.MergePR.
	svc := NewService(nil, "none", nil, nil, nil, log)
	ctrl := NewController(svc, log)
	router := gin.New()
	ctrl.RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge?workspace_id=ws-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "github_not_configured" {
		t.Fatalf("response code = %q, want github_not_configured", response.Code)
	}
}

// TestHttpSubmitReview_SelfApproveReturns422 exercises the full controller →
// service guard → HTTP mapping for the ErrSelfApprove path. Guards against a
// future refactor accidentally reclassifying the typed error as 500, which
// would mask "you can't approve your own PR" as an opaque upstream failure.
func TestHttpSubmitReview_SelfApproveReturns422(t *testing.T) {
	sc := &stubClient{
		// stubClient.GetAuthenticatedUser returns "test-user"; matching
		// AuthorLogin triggers the self-approve guard inside SubmitReview.
		getPRFunc: func(_ context.Context, _, _ string, _ int) (*PR, error) {
			return &PR{Number: 42, AuthorLogin: "test-user", State: "open"}, nil
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"event":"APPROVE"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/acme/widget/42/reviews", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Error != ErrSelfApprove.Error() {
		t.Errorf("expected error %q, got %q", ErrSelfApprove.Error(), resp.Error)
	}
}

func TestHttpRequestReviewers_RequestsReviewers(t *testing.T) {
	client := NewMockClient()
	client.AddRepos("test-user", []GitHubRepo{{Owner: "acme", Name: "widget", FullName: "acme/widget"}})
	router, _ := setupControllerTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/acme/widget/42/requested-reviewers", strings.NewReader(`{"reviewers":["octocat"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Requested bool          `json:"requested"`
		Principal AuthPrincipal `json:"principal"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Requested {
		t.Fatal("expected requested=true")
	}
	if response.Principal.WorkspaceID != "ws-1" {
		t.Fatalf("request principal workspace = %q, want ws-1", response.Principal.WorkspaceID)
	}
}

func TestHttpRequestReviewers_NormalizesReviewersBeforeClientDelegation(t *testing.T) {
	client := NewMockClient()
	client.AddRepos("test-user", []GitHubRepo{{Owner: "acme", Name: "widget", FullName: "acme/widget"}})
	router, _ := setupControllerTest(client)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/acme/widget/42/requested-reviewers", strings.NewReader(`{"reviewers":[" OctoCat ","octocat","hubot ","HUBOT"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got, want := client.requestedReviews[0].Reviewers, []string{"OctoCat", "hubot"}; !slices.Equal(got, want) {
		t.Fatalf("reviewers sent to client = %#v, want %#v", got, want)
	}
}

func TestHttpRequestReviewers_RejectsInvalidInput(t *testing.T) {
	router, _ := setupControllerTest(NewMockClient())
	for _, tc := range []struct {
		name string
		path string
		body string
	}{
		{name: "zero PR number", path: "/api/v1/github/prs/acme/widget/0/requested-reviewers", body: `{"reviewers":["octocat"]}`},
		{name: "empty reviewers", path: "/api/v1/github/prs/acme/widget/42/requested-reviewers", body: `{"reviewers":[]}`},
		{name: "blank reviewer", path: "/api/v1/github/prs/acme/widget/42/requested-reviewers", body: `{"reviewers":[" "]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHttpRequestReviewers_RejectsBoundedInputBeforeClientDelegation(t *testing.T) {
	overCountReviewers := make([]string, 51)
	for i := range overCountReviewers {
		overCountReviewers[i] = fmt.Sprintf("reviewer-%d", i)
	}
	overCountBody, err := json.Marshal(map[string][]string{"reviewers": overCountReviewers})
	if err != nil {
		t.Fatalf("marshal over-count body: %v", err)
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"reviewers":[`},
		{name: "oversized body", body: `{"reviewers":["` + strings.Repeat("a", 64*1024) + `"]}`},
		{name: "over-count reviewers", body: string(overCountBody)},
		{name: "overlong login", body: `{"reviewers":["` + strings.Repeat("a", 257) + `"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMockClient()
			router, _ := setupControllerTest(client)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/acme/widget/42/requested-reviewers", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if len(client.requestedReviews) != 0 {
				t.Fatalf("client received reviewers: %#v", client.requestedReviews)
			}
		})
	}
}

func TestHttpRequestReviewers_ErrNoClientReturns503(t *testing.T) {
	router, _ := setupControllerTest(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/acme/widget/42/requested-reviewers", strings.NewReader(`{"reviewers":["octocat"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHttpRequestReviewers_RepositoryOutsideWorkspaceReturnsSanitizedNotFound(t *testing.T) {
	router, _ := setupControllerTest(NewMockClient())
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/github/prs/acme/widget/42/requested-reviewers",
		strings.NewReader(`{"reviewers":["octocat"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "github_repository_not_found" {
		t.Fatalf("code = %q, want github_repository_not_found", response.Code)
	}
	if response.Error != "GitHub repository is not available in this workspace." {
		t.Fatalf("error = %q", response.Error)
	}
}

func TestHttpRequestReviewers_UpstreamErrorReturns500(t *testing.T) {
	router, _ := setupControllerTest(&stubClient{
		requestReviewersFn: func(context.Context, string, string, int, []string) error {
			return errors.New("GitHub rejected reviewers")
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/acme/widget/42/requested-reviewers", strings.NewReader(`{"reviewers":["octocat"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// listAccessibleReposClient is a per-test client tuned for the
// /api/v1/github/repos handler tests. Kept separate from the larger
// stubClient so the existing PR/merge tests above keep their minimal shape.
type listAccessibleReposClient struct {
	stubClient
	repos []GitHubRepo
}

func (c *listAccessibleReposClient) ListAccessibleRepos(_ context.Context, query string, _ int) ([]GitHubRepo, error) {
	return filterReposByQuery(c.repos, query), nil
}

func TestHandleListAccessibleRepos_OK(t *testing.T) {
	sc := &listAccessibleReposClient{
		repos: []GitHubRepo{
			{FullName: "alice/personal", Owner: "alice", Name: "personal", DefaultBranch: "main", Description: "Personal repo", PushedAt: func() *time.Time { t := time.Unix(200, 0); return &t }()},
			{FullName: "acme/widget", Owner: "acme", Name: "widget", DefaultBranch: "trunk", PushedAt: func() *time.Time { t := time.Unix(100, 0); return &t }()},
		},
	}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos?q=&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	raw := w.Body.String()
	var body struct {
		Repos []GitHubRepo `json:"repos"`
	}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Repos) != 2 {
		t.Fatalf("got %d repos, want 2", len(body.Repos))
	}
	// Sorted by pushed_at desc → personal (200) first, widget (100) second.
	if body.Repos[0].FullName != "alice/personal" {
		t.Errorf("first repo = %q, want alice/personal", body.Repos[0].FullName)
	}
	if body.Repos[0].DefaultBranch != "main" {
		t.Errorf("first repo default_branch = %q, want main", body.Repos[0].DefaultBranch)
	}
	if body.Repos[0].Description != "Personal repo" {
		t.Errorf("first repo description = %q, want Personal repo", body.Repos[0].Description)
	}
	if body.Repos[1].FullName != "acme/widget" {
		t.Errorf("second repo = %q, want acme/widget", body.Repos[1].FullName)
	}
	if body.Repos[1].DefaultBranch != "trunk" {
		t.Errorf("second repo default_branch = %q, want trunk", body.Repos[1].DefaultBranch)
	}
	// Empty description must be omitted (omitempty) — verify via the raw JSON
	// so a future struct-tag regression dropping omitempty is caught.
	if strings.Contains(raw, `"description":""`) {
		t.Errorf("expected empty description to be omitted, got body: %s", raw)
	}
}

func TestHandleListAccessibleRepos_503_WhenUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newControllerTestLogger()
	// nil client triggers ErrNoClient via Service.ListAccessibleRepos.
	svc := NewService(nil, "none", nil, nil, nil, log)
	ctrl := NewController(svc, log)
	router := gin.New()
	ctrl.RegisterHTTPRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos?workspace_id=ws-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "github_not_configured" {
		t.Errorf("code = %q, want github_not_configured", body.Code)
	}
	wantMsg := "GitHub is not configured. Install the gh CLI and run 'gh auth login', or add a GITHUB_TOKEN secret."
	if body.Error != wantMsg {
		t.Errorf("error = %q, want %q", body.Error, wantMsg)
	}
}

// listAccessibleReposEmptyClient: orgs+user repos both empty. Verifies the
// handler emits the literal `{"repos":[]}` body so a future regression that
// emits `null` (omitempty + nil slice) is caught.
func TestHandleListAccessibleRepos_Empty(t *testing.T) {
	sc := &listAccessibleReposClient{repos: []GitHubRepo{}}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	got := strings.TrimSpace(w.Body.String())
	if got != `{"repos":[]}` {
		t.Errorf("body = %q, want %q (regression guard for null repos)", got, `{"repos":[]}`)
	}
}

// listAccessibleReposErrClient surfaces an unexpected error from the repo
// fetch — the handler must map it to a 500 rather than swallowing it.
type listAccessibleReposErrClient struct {
	stubClient
	reposErr error
}

func (c *listAccessibleReposErrClient) ListAccessibleRepos(context.Context, string, int) ([]GitHubRepo, error) {
	return nil, c.reposErr
}

func TestHandleListAccessibleRepos_500_OnUnknownError(t *testing.T) {
	sc := &listAccessibleReposErrClient{reposErr: errors.New("boom")}
	router, _ := setupControllerTest(sc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleListAccessibleRepos_PreservesAPIErrorStatus verifies that when the
// upstream GitHub API returns 401 / 403 / 404, the handler surfaces the same
// status to the frontend rather than collapsing it into a generic 500. The
// frontend needs the distinct status to render the right UX (auth re-prompt,
// permission notice, "not found").
func TestHandleListAccessibleRepos_PreservesAPIErrorStatus(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
	}{
		{"unauthorized", http.StatusUnauthorized},
		{"forbidden", http.StatusForbidden},
		{"not_found", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := &listAccessibleReposErrClient{
				reposErr: &GitHubAPIError{
					StatusCode: tc.statusCode,
					Endpoint:   "/user/repos",
					Body:       "{}",
				},
			}
			router, _ := setupControllerTest(sc)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/github/repos", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.statusCode {
				t.Fatalf("expected %d, got %d: %s", tc.statusCode, w.Code, w.Body.String())
			}
		})
	}
}

func TestHttpMergePR_Conflict(t *testing.T) {
	sc := &stubClient{
		mergePRFn: func(context.Context, string, string, int, string) error {
			return &GitHubAPIError{StatusCode: http.StatusMethodNotAllowed, Endpoint: "/merge", Body: "not mergeable"}
		},
	}
	router, _ := setupControllerTest(sc)

	body := bytes.NewBufferString(`{"merge_method":"merge"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/github/prs/acme/widget/42/merge", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}
