package github

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type repositoryVisibilityClient struct {
	Client
	repos    map[string]GitHubRepo
	prs      []*PR
	issues   []*Issue
	paginate bool
}

func (c repositoryVisibilityClient) SearchPRs(context.Context, string, string) ([]*PR, error) {
	return append([]*PR(nil), c.prs...), nil
}

func (c repositoryVisibilityClient) SearchPRsPaged(
	_ context.Context, _, _ string, page, perPage int,
) (*PRSearchPage, error) {
	prs := append([]*PR(nil), c.prs...)
	if c.paginate {
		prs = paginateSearchResults(prs, page, perPage)
	}
	return &PRSearchPage{PRs: prs, TotalCount: len(c.prs), Page: page, PerPage: perPage}, nil
}

func (c repositoryVisibilityClient) ListIssues(context.Context, string, string) ([]*Issue, error) {
	return append([]*Issue(nil), c.issues...), nil
}

func (c repositoryVisibilityClient) ListIssuesPaged(
	_ context.Context, _, _ string, page, perPage int,
) (*IssueSearchPage, error) {
	issues := append([]*Issue(nil), c.issues...)
	if c.paginate {
		issues = paginateSearchResults(issues, page, perPage)
	}
	return &IssueSearchPage{
		Issues: issues, TotalCount: len(c.issues), Page: page, PerPage: perPage,
	}, nil
}

func (c repositoryVisibilityClient) GetPRStatus(
	_ context.Context, owner, repo string, number int,
) (*PRStatus, error) {
	return &PRStatus{PR: &PR{RepoOwner: owner, RepoName: repo, Number: number}}, nil
}

func (c repositoryVisibilityClient) HasRepositoryAccess(
	_ context.Context, owner, repo string,
) (bool, error) {
	if _, ok := c.repos[strings.ToLower(owner+"/"+repo)]; !ok {
		return false, &GitHubAPIError{StatusCode: 404, Endpoint: "/repos/" + owner + "/" + repo}
	}
	return true, nil
}

func (c repositoryVisibilityClient) ListAccessibleRepos(
	_ context.Context, query string, _ int,
) ([]GitHubRepo, error) {
	result := make([]GitHubRepo, 0, len(c.repos))
	for _, repo := range c.repos {
		if query == "" || strings.Contains(strings.ToLower(repo.FullName), strings.ToLower(query)) {
			result = append(result, repo)
		}
	}
	return result, nil
}

type fixedUserCredentialProvider struct{ client Client }

func (p fixedUserCredentialProvider) ResolveUser(
	_ context.Context, connection *UserConnection, _ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return &ResolvedCredential{
		Client: p.client,
		Principal: AuthPrincipal{
			Kind: AuthPrincipalHuman, Source: ConnectionSourceGitHubAppUser, Login: connection.Login,
		},
	}, nil
}

func TestResolveServiceClientRequiresWorkspace(t *testing.T) {
	svc := &Service{}
	_, err := svc.resolveAutomationClient(context.Background(), "", "octo", "repo")
	if !errors.Is(err, ErrGitHubWorkspaceRequired) {
		t.Fatalf("expected workspace required, got %v", err)
	}
}

func TestCredentialCacheScopeSeparatesWorkspacePrincipalAndGeneration(t *testing.T) {
	base := &ResolvedCredential{
		Principal: AuthPrincipal{
			WorkspaceID: "workspace-a",
			UserID:      "user-a",
			Source:      ConnectionSourcePAT,
			Kind:        AuthPrincipalHuman,
			Login:       "octocat",
		},
		CredentialGeneration:    1,
		AppRegistrationID:       "registration-a",
		AppCredentialGeneration: 7,
	}
	baseScope := credentialCacheScope(base, CredentialPurposePersonalRead)

	variants := []*ResolvedCredential{
		{Principal: AuthPrincipal{WorkspaceID: "workspace-b", UserID: "user-a", Source: ConnectionSourcePAT, Kind: AuthPrincipalHuman, Login: "octocat"}, CredentialGeneration: 1, AppRegistrationID: "registration-a", AppCredentialGeneration: 7},
		{Principal: AuthPrincipal{WorkspaceID: "workspace-a", UserID: "user-b", Source: ConnectionSourcePAT, Kind: AuthPrincipalHuman, Login: "octocat"}, CredentialGeneration: 1, AppRegistrationID: "registration-a", AppCredentialGeneration: 7},
		{Principal: base.Principal, CredentialGeneration: 2, AppRegistrationID: "registration-a", AppCredentialGeneration: 7},
		{Principal: base.Principal, CredentialGeneration: 1, AppRegistrationID: "registration-b", AppCredentialGeneration: 7},
		{Principal: base.Principal, CredentialGeneration: 1, AppRegistrationID: "registration-a", AppCredentialGeneration: 8},
	}
	for _, variant := range variants {
		if got := credentialCacheScope(variant, CredentialPurposePersonalRead); got == baseScope {
			t.Fatalf("cache scope collision: %q", got)
		}
	}
	if got := credentialCacheScope(base, CredentialPurposePersonalWrite); got == baseScope {
		t.Fatalf("purpose must be part of cache scope: %q", got)
	}
}

func TestRepositoryInWorkspaceScope(t *testing.T) {
	tests := []struct {
		name     string
		settings *WorkspaceSettings
		owner    string
		repo     string
		want     bool
	}{
		{name: "all", settings: &WorkspaceSettings{RepoScopeMode: RepoScopeModeAll}, owner: "octo", repo: "one", want: true},
		{name: "org match", settings: &WorkspaceSettings{RepoScopeMode: RepoScopeModeOrgs, RepoScopeOrgs: []string{"Octo"}}, owner: "octo", repo: "one", want: true},
		{name: "org miss", settings: &WorkspaceSettings{RepoScopeMode: RepoScopeModeOrgs, RepoScopeOrgs: []string{"other"}}, owner: "octo", repo: "one"},
		{name: "repo match", settings: &WorkspaceSettings{RepoScopeMode: RepoScopeModeRepos, RepoScopeRepos: []RepoFilter{{Owner: "Octo", Name: "One"}}}, owner: "octo", repo: "one", want: true},
		{name: "empty fails closed", settings: &WorkspaceSettings{RepoScopeMode: RepoScopeModeRepos}, owner: "octo", repo: "one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repositoryInWorkspaceScope(tt.settings, tt.owner, tt.repo); got != tt.want {
				t.Fatalf("repositoryInWorkspaceScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewWatchQueryUsesPersistedTargetLogin(t *testing.T) {
	tests := []struct {
		name  string
		watch *ReviewWatch
		want  string
	}{
		{
			name:  "user and teams",
			watch: &ReviewWatch{ReviewScope: ReviewScopeUserAndTeams, TargetLogin: "octocat"},
			want:  "type:pr state:open review-requested:octocat -is:draft",
		},
		{
			name:  "user only",
			watch: &ReviewWatch{ReviewScope: ReviewScopeUser, TargetLogin: "octocat"},
			want:  "type:pr state:open user-review-requested:octocat -is:draft",
		},
		{
			name:  "custom query",
			watch: &ReviewWatch{CustomQuery: "is:open label:urgent", TargetLogin: "octocat"},
			want:  "is:open label:urgent review-requested:octocat",
		},
		{
			name:  "human legacy remains current user",
			watch: &ReviewWatch{CustomQuery: "is:open"},
			want:  "is:open",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reviewWatchQuery(tt.watch); got != tt.want {
				t.Fatalf("reviewWatchQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

type rejectingInstallationProvider struct {
	t *testing.T
}

func (p rejectingInstallationProvider) ResolveInstallation(
	context.Context, *WorkspaceConnection, ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	p.t.Fatal("installation provider called for a review watch without target_login")
	return nil, nil
}

func TestCheckReviewWatchDisablesAppWatchWithoutTargetBeforeProviderCall(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO workspaces (id) VALUES ('workspace-app')`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	installationID := int64(123)
	if err := store.UpsertDeploymentAppRegistration(
		ctx, newAppRegistration("registration-watch", 123, "Watch App", time.Now().UTC()),
	); err != nil {
		t.Fatalf("seed App registration: %v", err)
	}
	if err := store.UpsertWorkspaceConnection(ctx, &WorkspaceConnection{
		WorkspaceID:              "workspace-app",
		Source:                   ConnectionSourceGitHubAppInstallation,
		Status:                   ConnectionStatusActive,
		GitHubHost:               defaultGitHubHost,
		InstallationID:           &installationID,
		InstallationAccountLogin: "octo-org",
		InstallationAccountType:  "Organization",
		AppRegistrationID:        "registration-watch",
	}); err != nil {
		t.Fatalf("upsert workspace connection: %v", err)
	}
	watch := &ReviewWatch{
		ID:                  "watch-app",
		WorkspaceID:         "workspace-app",
		Enabled:             true,
		PollIntervalSeconds: minWatchPollIntervalSec,
	}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	svc := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	svc.resolver.SetInstallationProvider(rejectingInstallationProvider{t: t})

	_, err := svc.CheckReviewWatch(ctx, watch)
	if !errors.Is(err, ErrGitHubPersonalRequired) {
		t.Fatalf("CheckReviewWatch error = %v, want personal required", err)
	}
	stored, err := store.GetReviewWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get review watch: %v", err)
	}
	if stored.Enabled || stored.LastError == "" {
		t.Fatalf("watch was not disabled with actionable error: %+v", stored)
	}
}

func TestSyncWorkspaceWatchesBatchedRejectsMixedOwnershipBeforeResolve(t *testing.T) {
	svc := &Service{}
	_, err := svc.SyncWorkspaceWatchesBatched(context.Background(), "workspace-a", []*PRWatch{
		{WorkspaceID: "workspace-a"},
		{WorkspaceID: "workspace-b"},
	})
	if !errors.Is(err, ErrGitHubWorkspaceRequired) {
		t.Fatalf("error = %v, want workspace ownership error", err)
	}
}

func TestRequireGitHubCapabilityFailsClosedForAppOnly(t *testing.T) {
	app := &resolvedServiceClient{
		Principal:    AuthPrincipal{Kind: AuthPrincipalApp},
		Capabilities: map[GitHubAppCapability]bool{CapabilityPullRequestRead: true},
	}
	if err := requireGitHubCapability(app, CapabilityPullRequestWrite); !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("app capability error = %v", err)
	}
	human := &resolvedServiceClient{Principal: AuthPrincipal{Kind: AuthPrincipalHuman}}
	if err := requireGitHubCapability(human, CapabilityPullRequestWrite); err != nil {
		t.Fatalf("human token should defer permissions to GitHub: %v", err)
	}
}

func TestPersonalRepositoryResolutionRequiresAutomationVisibility(t *testing.T) {
	service := personalBoundaryTestService(t,
		map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")},
		map[string]GitHubRepo{
			"acme/allowed":       testGitHubRepo("acme", "allowed"),
			"personal-only/repo": testGitHubRepo("personal-only", "repo"),
		},
	)

	_, err := service.resolvePersonalReadClient(
		context.Background(), "workspace-1", "user-1", "personal-only", "repo",
	)
	if !errors.Is(err, ErrRepoNotResolvable) {
		t.Fatalf("resolvePersonalReadClient() error = %v, want repository boundary denial", err)
	}
	if _, err := service.resolvePersonalReadClient(
		context.Background(), "workspace-1", "user-1", "acme", "allowed",
	); err != nil {
		t.Fatalf("allowed repository resolution failed: %v", err)
	}
}

func TestPersonalAccessibleReposIntersectAutomationVisibility(t *testing.T) {
	service := personalBoundaryTestService(t,
		map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")},
		map[string]GitHubRepo{
			"acme/allowed":       testGitHubRepo("acme", "allowed"),
			"personal-only/repo": testGitHubRepo("personal-only", "repo"),
		},
	)

	repos, err := service.ListAccessibleReposForWorkspace(
		context.Background(), "workspace-1", "user-1", "", 50,
	)
	if err != nil {
		t.Fatalf("ListAccessibleReposForWorkspace() error = %v", err)
	}
	if len(repos) != 1 || repos[0].FullName != "acme/allowed" {
		t.Fatalf("accessible repositories = %+v, want only automation-visible repository", repos)
	}
}

func TestPersonalSearchAndStatusResultsIntersectAutomationVisibility(t *testing.T) {
	service := personalBoundaryTestService(t,
		map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")},
		map[string]GitHubRepo{
			"acme/allowed":       testGitHubRepo("acme", "allowed"),
			"personal-only/repo": testGitHubRepo("personal-only", "repo"),
		},
	)
	ctx := context.Background()

	prs, err := service.SearchUserPRsForWorkspace(ctx, "workspace-1", "user-1", "", "")
	if err != nil || len(prs) != 1 || prs[0].RepoName != "allowed" {
		t.Fatalf("unpaged PR search = %+v, %v", prs, err)
	}
	issues, err := service.SearchUserIssuesPagedForWorkspaceUser(
		ctx, "workspace-1", "user-1", "", "", 1, 50,
	)
	if err != nil || len(issues.Issues) != 1 || issues.TotalCount != 1 || issues.Issues[0].RepoName != "allowed" {
		t.Fatalf("paged issue search = %+v, %v", issues, err)
	}
	statuses, err := service.GetPRStatusesBatchForWorkspace(ctx, "workspace-1", "user-1", []PRRef{
		{Owner: "acme", Repo: "allowed", Number: 1},
		{Owner: "personal-only", Repo: "repo", Number: 2},
	})
	if err != nil {
		t.Fatalf("status batch error = %v", err)
	}
	if len(statuses) != 1 || statuses[prStatusCacheKey("acme", "allowed", 1)] == nil {
		t.Fatalf("status batch = %+v, want only automation-visible repository", statuses)
	}
}

func TestPersonalSearchPaginatesAfterAutomationVisibilityFiltering(t *testing.T) {
	personalPRs := make([]*PR, 100, 101)
	personalIssues := make([]*Issue, 100, 101)
	for i := range 100 {
		personalPRs[i] = &PR{RepoOwner: "personal-only", RepoName: "repo", Number: i + 1}
		personalIssues[i] = &Issue{RepoOwner: "personal-only", RepoName: "repo", Number: i + 1}
	}
	personalPRs = append(personalPRs, &PR{RepoOwner: "acme", RepoName: "allowed", Number: 101})
	personalIssues = append(personalIssues, &Issue{RepoOwner: "acme", RepoName: "allowed", Number: 101})
	service := personalBoundaryTestServiceWithResults(t, personalPRs, personalIssues, true)
	ctx := context.Background()

	prs, err := service.SearchUserPRsPagedForWorkspaceUser(
		ctx, "workspace-1", "user-1", "", "", 1, 1,
	)
	if err != nil || len(prs.PRs) != 1 || prs.TotalCount != 1 || prs.PRs[0].Number != 101 {
		t.Fatalf("paged PR search = %+v, %v", prs, err)
	}
	issues, err := service.SearchUserIssuesPagedForWorkspaceUser(
		ctx, "workspace-1", "user-1", "", "", 1, 1,
	)
	if err != nil || len(issues.Issues) != 1 || issues.TotalCount != 1 || issues.Issues[0].Number != 101 {
		t.Fatalf("paged issue search = %+v, %v", issues, err)
	}
}

func personalBoundaryTestService(
	t *testing.T,
	automationRepos, personalRepos map[string]GitHubRepo,
) *Service {
	return personalBoundaryTestServiceWithClients(t, automationRepos, personalRepos, []*PR{
		{RepoOwner: "acme", RepoName: "allowed", Number: 1},
		{RepoOwner: "personal-only", RepoName: "repo", Number: 2},
	}, []*Issue{
		{RepoOwner: "acme", RepoName: "allowed", Number: 1},
		{RepoOwner: "personal-only", RepoName: "repo", Number: 2},
	}, false)
}

func personalBoundaryTestServiceWithResults(
	t *testing.T, prs []*PR, issues []*Issue, paginate bool,
) *Service {
	return personalBoundaryTestServiceWithClients(t,
		map[string]GitHubRepo{"acme/allowed": testGitHubRepo("acme", "allowed")},
		map[string]GitHubRepo{
			"acme/allowed":       testGitHubRepo("acme", "allowed"),
			"personal-only/repo": testGitHubRepo("personal-only", "repo"),
		}, prs, issues, paginate,
	)
}

func personalBoundaryTestServiceWithClients(
	t *testing.T,
	automationRepos, personalRepos map[string]GitHubRepo,
	prs []*PR,
	issues []*Issue,
	paginate bool,
) *Service {
	t.Helper()
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-1")
	if err := store.UpsertWorkspaceSettings(context.Background(), &WorkspaceSettings{
		WorkspaceID: "workspace-1", RepoScopeMode: RepoScopeModeAll,
	}); err != nil {
		t.Fatal(err)
	}
	installationID := int64(42)
	connections := &fakeConnectionReader{
		workspaces: map[string]*WorkspaceConnection{
			"workspace-1": {
				WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
				InstallationID: &installationID, AppRegistrationID: "registration-test",
				Status: ConnectionStatusActive, CredentialGeneration: 1,
			},
		},
		users: map[string]*UserConnection{
			"workspace-1:user-1": {
				WorkspaceID: "workspace-1", UserID: "user-1",
				AppRegistrationID: "registration-test", Login: "octocat",
				Status: ConnectionStatusActive, CredentialGeneration: 1,
			},
		},
	}
	automationClient := repositoryVisibilityClient{Client: NewMockClient(), repos: automationRepos}
	personalClient := repositoryVisibilityClient{
		Client: NewMockClient(), repos: personalRepos, prs: prs, issues: issues, paginate: paginate,
	}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetAutomationProvider(testAutomationCredentialProvider{client: automationClient})
	resolver.SetUserProvider(fixedUserCredentialProvider{client: personalClient})
	service := NewService(nil, AuthMethodNone, nil, store, nil, testLogger(t))
	service.resolver = resolver
	return service
}

func testGitHubRepo(owner, name string) GitHubRepo {
	return GitHubRepo{FullName: owner + "/" + name, Owner: owner, Name: name}
}
