package github

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/githubhost"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnterpriseHostCacheIdentity(t *testing.T) {
	public := &ResolvedCredential{GitHubHost: "github.com", Principal: AuthPrincipal{WorkspaceID: "workspace"}, CredentialGeneration: 1}
	enterprise := *public
	enterprise.GitHubHost = "git.example.com"
	if credentialCacheScope(public, CredentialPurposeAutomation) == credentialCacheScope(&enterprise, CredentialPurposeAutomation) {
		t.Fatal("cross-host credentials share cache namespace")
	}
}

func TestEnterpriseHostResourcePersistence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &PRWatch{GitHubHost: "git.example.com", WorkspaceID: "workspace", SessionID: "session", TaskID: "task", RepositoryID: "repo", Owner: "acme", Repo: "widget", Branch: "main"}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.GitHubHost != "git.example.com" {
		t.Fatalf("watch lost host: %+v", got)
	}
	pr := &TaskPR{GitHubHost: "git.example.com", TaskID: "task", RepositoryID: "repo", Owner: "acme", Repo: "widget", PRNumber: 7, PRURL: "https://git.example.com/acme/widget/pull/7"}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatal(err)
	}
	saved, err := store.GetTaskPRByRepository(ctx, "task", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if saved.GitHubHost != "git.example.com" {
		t.Fatalf("task association lost host: %+v", saved)
	}
}

func TestEnterpriseHostMigration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, tc := range []struct{ id, url string }{
		{"public", "https://github.com/acme/widget/pull/7"},
		{"enterprise", "https://git.example.com/acme/widget/pull/7"},
		{"invalid", "https://user@github.com/acme/widget/pull/7"},
	} {
		if _, err := store.db.Exec("INSERT INTO tasks (id,workspace_id) VALUES (?, ?)", tc.id, "workspace"); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: tc.id, RepositoryID: tc.id, Owner: "acme", Repo: "widget", PRNumber: 7, PRURL: tc.url}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec("ALTER TABLE github_task_prs DROP COLUMN github_host"); err != nil {
		t.Fatal(err)
	}
	upgraded, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, host string }{{"public", "github.com"}, {"enterprise", "git.example.com"}, {"invalid", ""}} {
		got, err := upgraded.GetTaskPRByRepository(ctx, tc.id, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil {
			t.Fatalf("migration dropped association %s", tc.id)
		}
		if got.GitHubHost != tc.host {
			t.Errorf("%s migrated host %q want %q", tc.id, got.GitHubHost, tc.host)
		}
	}
	replay, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := replay.GetTaskPRByRepository(ctx, "invalid", "invalid")
	if err != nil {
		t.Fatal(err)
	}
	if invalid.GitHubHost != "" {
		t.Fatal("replay relabeled invalid historical record")
	}
}

func TestEnterpriseHostForeignDestinationRejectedBeforeResolution(t *testing.T) {
	calls := 0
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace": {WorkspaceID: "workspace", GitHubHost: "github.com", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, CredentialGeneration: 1},
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(testAutomationCredentialProvider{client: NewMockClient()})
	svc := NewService(nil, AuthMethodNone, nil, newTestStore(t), nil, testLogger(t))
	svc.resolver = resolver
	svc.workspaceAuthorizer = func(context.Context, string) error { calls++; return nil }
	ctx := withExpectedGitHubHost(context.Background(), "git.example.com")
	if _, err := svc.resolveAutomationClient(ctx, "workspace", "", ""); err == nil {
		t.Fatal("foreign host accepted")
	}
	if calls == 0 {
		t.Fatal("host guard bypassed workspace authorization")
	}
}
func TestEnterpriseHostForkRepositoryBinding(t *testing.T) {
	binding, err := repositoryBinding(&GitHubRepository{ID: 42, FullName: "acme/widget", CloneURL: "https://git.example.com/acme/widget.git"})
	if err != nil {
		t.Fatal(err)
	}
	if binding.Host != "git.example.com" {
		t.Fatalf("fork identity lost host: %+v", binding)
	}
}
func TestEnterpriseHostContributionIdentity(t *testing.T) {
	pr := &PR{Number: 7, State: "open", RepoOwner: "acme", RepoName: "widget", HeadRepoOwner: "fork", HeadRepoName: "widget", BaseRepoOwner: "acme", BaseRepoName: "widget", HeadBranch: "feature", BaseBranch: "main", HeadSHA: strings.Repeat("a", 40), MaintainerCanModify: true}
	resolution, err := buildGitHubRemoteContribution("https://git.example.com/acme/widget/pull/7", "acme", "widget", 7, pr)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.TargetHost != "https://git.example.com" || resolution.Binding.SourceRepository.Host != "git.example.com" {
		t.Fatalf("contribution lost host: %+v", resolution)
	}
}

func TestEnterpriseHostReferenceParsing(t *testing.T) {
	host, err := githubhost.Parse("git.example.com:8443")
	if err != nil {
		t.Fatal(err)
	}
	owner, repo, number, err := parseIssueReferenceForHost("https://git.example.com:8443/acme/widget/issues/7", "", "", host)
	if err != nil || owner != "acme" || repo != "widget" || number != 7 {
		t.Fatalf("issue identity %s/%s#%d: %v", owner, repo, number, err)
	}
	owner, repo, number, err = parseGitHubContributionURLForHost("https://git.example.com:8443/acme/widget/pull/7", host)
	if err != nil || owner != "acme" || repo != "widget" || number != 7 {
		t.Fatalf("PR identity %s/%s#%d: %v", owner, repo, number, err)
	}
	if _, _, _, err := parseIssueReferenceForHost("https://github.com/acme/widget/issues/7", "", "", host); err == nil {
		t.Fatal("accepted foreign issue")
	}
}
func TestEnterpriseHostComparisonIdentity(t *testing.T) {
	candidate, ok := githubComparisonTargetCandidate(&PR{Number: 7, HTMLURL: "https://git.example.com/acme/widget/pull/7", HeadRepoOwner: "fork", HeadRepoName: "widget", BaseRepoOwner: "acme", BaseRepoName: "widget", HeadBranch: "feature", BaseBranch: "main"})
	if !ok || candidate.HeadRepository.Host != "git.example.com" || candidate.TargetRepository.Host != "git.example.com" {
		t.Fatalf("comparison lost host: %+v", candidate)
	}
}
func TestEnterpriseHostAccessibleRepositoryIdentity(t *testing.T) {
	svc := personalBoundaryTestService(t, map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")}, map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")})
	repos, err := svc.ListAccessibleReposForWorkspace(context.Background(), "workspace-1", "user-1", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].GitHubHost != "github.com" || repos[0].HTMLURL != "https://github.com/acme/allowed" {
		t.Fatalf("missing repository identity: %+v", repos)
	}
}
func TestEnterpriseHostOldPRWatchDoesNotRunOnReplacement(t *testing.T) {
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace": {WorkspaceID: "workspace", GitHubHost: "git.example.com", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, CredentialGeneration: 2},
	}}
	resolver := NewCredentialResolver(connections, nil)
	client := &hostBoundaryWatchClient{repositoryVisibilityClient: repositoryVisibilityClient{Client: NewMockClient()}}
	resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
	svc := NewService(nil, AuthMethodNone, nil, newTestStore(t), nil, testLogger(t))
	t.Cleanup(svc.Stop)
	svc.resolver = resolver
	results, err := svc.SyncWorkspaceWatchesBatched(context.Background(), "workspace", []*PRWatch{{ID: "watch", GitHubHost: "github.com", WorkspaceID: "workspace", Owner: "acme", Repo: "widget", Branch: "main"}})
	if err != nil || len(results) != 0 || client.calls.Load() != 0 {
		t.Fatalf("old watch reached provider: results=%v err=%v calls=%d", results, err, client.calls.Load())
	}
}
func TestEnterpriseHostReviewAndIssueChildPersistence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	review := &ReviewWatch{GitHubHost: "git.example.com", WorkspaceID: "workspace", Enabled: true}
	if err := store.CreateReviewWatch(ctx, review); err != nil {
		t.Fatal(err)
	}
	issue := &IssueWatch{GitHubHost: "git.example.com", WorkspaceID: "workspace", Enabled: true}
	if err := store.CreateIssueWatch(ctx, issue); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveReviewPRTask(ctx, review.ID, "acme", "widget", 7, "https://git.example.com/acme/widget/pull/7"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, issue.ID, "acme", "widget", 7, "https://git.example.com/acme/widget/issues/7"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"github_review_watches", "github_issue_watches", "github_review_pr_tasks", "github_issue_watch_tasks"} {
		var host string
		if err := store.ro.GetContext(ctx, &host, "SELECT github_host FROM "+table+" LIMIT 1"); err != nil {
			t.Fatal(err)
		}
		if host != "git.example.com" {
			t.Errorf("%s host %q", table, host)
		}
	}
}

func TestEnterpriseHostControllerExpectedHost(t *testing.T) {
	svc := personalBoundaryTestService(t, map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")}, map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")})
	router := gin.New()
	NewController(svc, testLogger(t)).RegisterHTTPRoutes(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/github/prs/acme/allowed/7/info?workspace_id=workspace-1&expected_host=git.example.com", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "github_host_mismatch") {
		t.Fatalf("foreign lookup admitted: %d %s", response.Code, response.Body.String())
	}
}

func TestEnterpriseHostDelayedPRReadCannotCrossReplacement(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace": {WorkspaceID: "workspace", GitHubHost: "github.com", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, CredentialGeneration: 1},
	}}
	client := repositoryVisibilityClient{Client: &stubClient{getPRFunc: func(context.Context, string, string, int) (*PR, error) {
		close(entered)
		<-release
		return &PR{Number: 7, HTMLURL: "https://github.com/acme/widget/pull/7"}, nil
	}}, repos: map[string]GitHubRepo{"acme/widget": testGitHubRepo("acme", "widget")}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(testAutomationCredentialProvider{client: client})
	svc := NewService(nil, AuthMethodNone, nil, newTestStore(t), nil, testLogger(t))
	svc.resolver = resolver
	result := make(chan error, 1)
	go func() {
		_, err := svc.GetPRForWorkspace(context.Background(), "workspace", "", "acme", "widget", 7)
		result <- err
	}()
	<-entered
	connections.workspaces["workspace"].GitHubHost = "git.example.com"
	connections.workspaces["workspace"].CredentialGeneration = 2
	close(release)
	if err := <-result; !errors.Is(err, ErrGitHubHostMismatch) {
		t.Fatalf("stale PR read accepted: %v", err)
	}
}

func TestEnterpriseHostBatchResponseKeys(t *testing.T) {
	svc := NewService(nil, AuthMethodNone, nil, newTestStore(t), nil, testLogger(t))
	client := repositoryVisibilityClient{Client: NewMockClient()}
	statuses, err := svc.getPRStatusesBatchWithClient(context.Background(), client, "scope", []PRRef{{ExpectedHost: "git.example.com", Owner: "acme", Repo: "widget", Number: 7}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := statuses["git.example.com/acme/widget#7"]; !ok {
		t.Fatalf("batch dropped host key: %+v", statuses)
	}
}
func TestEnterpriseHostDelayedWatchResultDoesNotPersist(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec("INSERT INTO tasks (id,workspace_id) VALUES ('task','workspace')"); err != nil {
		t.Fatal(err)
	}
	watch := &PRWatch{WorkspaceID: "workspace", GitHubHost: "github.com", TaskID: "task", RepositoryID: "repo", Owner: "acme", Repo: "widget", PRNumber: 7, Branch: "main"}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{WorkspaceID: "workspace", GitHubHost: "github.com", TaskID: "task", RepositoryID: "repo", Owner: "acme", Repo: "widget", PRNumber: 7, PRURL: "https://github.com/acme/widget/pull/7", State: "open"}); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	gh := &graphQLMockClient{MockClient: NewMockClient(), onExecute: func() { close(entered); <-release }, prResponses: []string{
		`{"data":{"repo0":{"pr0":{"state":"MERGED","title":"late","url":"https://github.com/acme/widget/pull/7","headRefName":"feature","baseRefName":"main","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","reviews":{"nodes":[]},"commits":{"nodes":[]}}}}}`,
	}}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace": {WorkspaceID: "workspace", GitHubHost: "github.com", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, CredentialGeneration: 1},
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(testAutomationCredentialProvider{client: gh})
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	svc.resolver = resolver
	finished := make(chan struct{})
	go func() { _, _ = svc.SyncWorkspaceWatchesBatched(ctx, "workspace", []*PRWatch{watch}); close(finished) }()
	<-entered
	connections.workspaces["workspace"].GitHubHost = "git.example.com"
	connections.workspaces["workspace"].CredentialGeneration = 2
	close(release)
	<-finished
	got, err := store.GetTaskPRByRepository(ctx, "task", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "open" {
		t.Fatalf("stale result changed task association: %+v", got)
	}
	stored, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastCheckedAt != nil {
		t.Fatal("stale result changed watch timestamps")
	}
}

func TestEnterpriseHostPersistedIssueLink(t *testing.T) {
	link, ok := taskIssueLinkFromMetadata(taskIssueMetadataRow{TaskID: "task", Metadata: `{"issue_url":"https://git.example.com/acme/widget/issues/7","issue_number":7,"issue_owner":"acme","issue_repo":"widget","github_issue_linked":true}`})
	if !ok || link.GitHubHost != "git.example.com" || link.IssueNumber != 7 {
		t.Fatalf("issue metadata lost host: %+v %t", link, ok)
	}
}
func TestEnterpriseHostMigrationPreservesRepositoryOriginAndRejectsConflict(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`CREATE TABLE repositories(id TEXT PRIMARY KEY,provider TEXT,provider_host TEXT,provider_owner TEXT,provider_name TEXT,remote_url TEXT);
 INSERT INTO repositories VALUES('repo','github','https://git.example.com','acme','widget','https://git.example.com/acme/widget.git');
 INSERT INTO tasks(id,workspace_id) VALUES('task','workspace')`); err != nil {
		t.Fatal(err)
	}
	watch := &PRWatch{TaskID: "task", WorkspaceID: "workspace", RepositoryID: "repo", Owner: "acme", Repo: "widget", PRNumber: 7, Branch: "main"}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: "task", WorkspaceID: "workspace", RepositoryID: "repo", Owner: "acme", Repo: "widget", PRNumber: 7, PRURL: "https://github.com/acme/widget/pull/7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("ALTER TABLE github_pr_watches DROP COLUMN github_host"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("ALTER TABLE github_task_prs DROP COLUMN github_host"); err != nil {
		t.Fatal(err)
	}
	upgraded, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := upgraded.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.GitHubHost != "git.example.com" {
		t.Fatalf("repository origin lost: %+v", migrated)
	}
	association, err := upgraded.GetTaskPRByRepository(ctx, "task", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if association.GitHubHost != "" {
		t.Fatalf("contradictory URL remained eligible: %+v", association)
	}
}
func TestEnterpriseHostControllerRejectsMalformedScope(t *testing.T) {
	svc := personalBoundaryTestService(t, map[string]GitHubRepo{}, map[string]GitHubRepo{})
	router := gin.New()
	NewController(svc, testLogger(t)).RegisterHTTPRoutes(router)
	for _, scope := range []string{"123", "null", "[]"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/github/prs/statuses", strings.NewReader(`{"workspace_id":"workspace-1","expected_host":`+scope+`,"refs":[]}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "github_host_invalid") {
			t.Fatalf("malformed scope ignored: %d %s", response.Code, response.Body.String())
		}
	}
}
func TestEnterpriseHostNewWatchRejectsHistoricalRepository(t *testing.T) {
	store := newTestStore(t)
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"workspace": {WorkspaceID: "workspace", GitHubHost: "git.example.com", Source: ConnectionSourcePAT, Status: ConnectionStatusActive, CredentialGeneration: 2},
	}}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	svc.resolver = NewCredentialResolver(connections, nil)
	svc.SetTaskIssueStore(&fakeTaskIssueStore{task: &taskmodels.Task{ID: "task", WorkspaceID: "workspace"}, repos: []*taskmodels.TaskRepository{{RepositoryID: "repo"}}, entities: map[string]*taskmodels.Repository{
		"repo": {ID: "repo", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widget", RemoteURL: "https://github.com/acme/widget.git"},
	}})
	if _, err := svc.CreatePRWatchForWorkspace(context.Background(), "workspace", "session", "task", "repo", "acme", "widget", 7, "main"); !errors.Is(err, ErrGitHubHostMismatch) {
		t.Fatalf("historical repository admitted to new host: %v", err)
	}
}
