package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
)

// fakeGitLabMRLinkService implements GitLabMRLinkService for testing.
type fakeGitLabMRLinkService struct {
	mu sync.Mutex

	autoLinkCalls int
	autoLinkFunc  func(ctx context.Context, workspaceID, sessionID, taskID, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error)
	// lastAutoLinkRepositoryID records the repositoryID argument of the most
	// recent AutoLinkMRForBranch call, so multi-repo tests can assert scoping.
	lastAutoLinkRepositoryID string
	// lastAutoLinkTaskID records the taskID argument of the most recent
	// AutoLinkMRForBranch call, so workspace-group redirect tests can assert
	// which task the write actually landed under.
	lastAutoLinkTaskID string

	ensureWatchCalls int
	// lastEnsureWatchTaskID records the taskID argument of the most recent
	// EnsureMRWatch call, mirroring lastAutoLinkTaskID above.
	lastEnsureWatchTaskID string

	taskMRs map[string][]*gitlab.TaskMR

	isConfiguredGitLabHostCalls int
	isConfiguredGitLabHostFunc  func(ctx context.Context, workspaceID, remoteURL string) bool
}

func (f *fakeGitLabMRLinkService) AutoLinkMRForBranch(
	ctx context.Context, workspaceID, sessionID, taskID, repositoryID, projectPath, branch string,
) (*gitlab.TaskMR, error) {
	f.mu.Lock()
	f.autoLinkCalls++
	f.lastAutoLinkRepositoryID = repositoryID
	f.lastAutoLinkTaskID = taskID
	fn := f.autoLinkFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, workspaceID, sessionID, taskID, repositoryID, projectPath, branch)
	}
	return nil, nil
}

func (f *fakeGitLabMRLinkService) EnsureMRWatch(
	_ context.Context, _, taskID, _, _ string, _ int, _ string,
) (*gitlab.MRWatch, error) {
	f.mu.Lock()
	f.ensureWatchCalls++
	f.lastEnsureWatchTaskID = taskID
	f.mu.Unlock()
	return &gitlab.MRWatch{}, nil
}

func (f *fakeGitLabMRLinkService) ListTaskMRsByTask(_ context.Context, taskID string) ([]*gitlab.TaskMR, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.taskMRs[taskID], nil
}

func (f *fakeGitLabMRLinkService) IsConfiguredGitLabHost(ctx context.Context, workspaceID, remoteURL string) bool {
	f.mu.Lock()
	f.isConfiguredGitLabHostCalls++
	fn := f.isConfiguredGitLabHostFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, workspaceID, remoteURL)
	}
	return false
}

// seedGitLabSessionWithRepo mirrors event_handlers_github_test.go's
// seedSessionWithRepo, but tags the repository as a GitLab provider so
// resolvePushRepo/resolvePushRepositoryProvider route to the GitLab path.
func seedGitLabSessionWithRepo(t *testing.T) (*Service, *fakeGitLabMRLinkService) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myproj",
		SourceType: "provider", Provider: gitlabProviderName,
		ProviderOwner: "group", ProviderName: "myproj",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1", CheckoutBranch: "feat/a",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}

	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	svc.SetGitLabMRLinkService(fake)
	return svc, fake
}

func TestDetectPushAndAssociateMR_Immediate(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 5, HeadBranch: branch}, nil
	}

	svc.detectPushAndAssociateMR(context.Background(), "s1", "t1", "", "feat/a")

	if fake.autoLinkCalls != 1 {
		t.Fatalf("expected 1 AutoLinkMRForBranch call, got %d", fake.autoLinkCalls)
	}
	if fake.lastAutoLinkRepositoryID != "repo1" {
		t.Fatalf("repositoryID = %q, want repo1", fake.lastAutoLinkRepositoryID)
	}
}

func TestDetectPushAndAssociateMR_AlreadyLinkedIsNoOp(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.taskMRs["t1"] = []*gitlab.TaskMR{
		{TaskID: "t1", RepositoryID: "repo1", HeadBranch: "feat/a", MRIID: 9, State: gitlabMRStateOpen},
	}

	svc.detectPushAndAssociateMR(context.Background(), "s1", "t1", "", "feat/a")

	if fake.autoLinkCalls != 0 {
		t.Fatalf("expected no AutoLinkMRForBranch calls when already linked, got %d", fake.autoLinkCalls)
	}
}

func TestDetectPushAndAssociateMR_NoServiceIsNoOp(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Must not panic with gitlabMRLinkService unwired.
	svc.detectPushAndAssociateMR(context.Background(), "s1", "t1", "", "feat/a")
}

func TestTrackPushDispatchesByProvider_GitLabRepoCallsZeroGitHub(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 1, HeadBranch: branch}, nil
	}
	ghSvc := &mockGitHubService{}
	svc.SetGitHubService(ghSvc)

	svc.dispatchPushDetection(context.Background(), "s1", "t1", "", "feat/a")

	if fake.autoLinkCalls != 1 {
		t.Fatalf("expected the GitLab path to run, got %d AutoLinkMRForBranch calls", fake.autoLinkCalls)
	}
	if ghSvc.associateCalls != 0 || ghSvc.createWatchCalls != 0 || ghSvc.getTaskPRCalls != 0 {
		t.Fatalf("expected zero GitHub calls for a GitLab-provider repository, got associate=%d createWatch=%d getTaskPR=%d",
			ghSvc.associateCalls, ghSvc.createWatchCalls, ghSvc.getTaskPRCalls)
	}
}

func TestTrackPushDispatchesByProvider_GitHubRepoCallsZeroGitLab(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myrepo",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "myrepo",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{
		prWatch: &github.PRWatch{
			ID: "w1", SessionID: "s1", TaskID: "t1",
			Owner: "myorg", Repo: "myrepo", PRNumber: 10, Branch: "feature-branch",
		},
	}
	svc.SetGitHubService(ghSvc)
	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	svc.SetGitLabMRLinkService(fake)

	svc.dispatchPushDetection(ctx, "s1", "t1", "", "feature-branch")

	if fake.autoLinkCalls != 0 || fake.ensureWatchCalls != 0 {
		t.Fatalf("expected zero GitLab calls for a GitHub-provider repository, got autoLink=%d ensureWatch=%d",
			fake.autoLinkCalls, fake.ensureWatchCalls)
	}
	// Positively assert the GitHub path actually ran — zero GitLab calls
	// alone can't distinguish "routed to GitHub" from "routed nowhere",
	// which would silently pass this test even if dispatchPushDetection's
	// routing broke entirely.
	if ghSvc.getPRWatchBySessionRepoAndBranchCalls != 1 {
		t.Fatalf("expected detectPushAndAssociatePR to look up the PR watch exactly once, got %d",
			ghSvc.getPRWatchBySessionRepoAndBranchCalls)
	}
}

func TestCheckSessionMR_Found(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 3, HeadBranch: branch}, nil
	}

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatal("expected found=true when an open MR exists on the session branch")
	}
}

func TestCheckSessionMR_NotFound(t *testing.T) {
	svc, _ := seedGitLabSessionWithRepo(t)

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if found {
		t.Fatal("expected found=false when no MR is open on the session branch")
	}
}

// TestCheckSessionMR_ScopesToCurrentBranchNotAnyLinkedMR guards against a
// multi-repo/multi-branch regression: a task that already has an MR linked
// on a DIFFERENT (repository, branch) than the caller's current session must
// not short-circuit as "found" on that unrelated association — it must still
// search and link the current branch's own MR.
func TestCheckSessionMR_ScopesToCurrentBranchNotAnyLinkedMR(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	// An MR already linked on a sibling branch of the same repo.
	fake.taskMRs["t1"] = []*gitlab.TaskMR{
		{TaskID: "t1", RepositoryID: "repo1", HeadBranch: "feat/other", MRIID: 99},
	}
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 3, HeadBranch: branch}, nil
	}

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after linking the current branch's own MR")
	}
	if fake.autoLinkCalls != 1 {
		t.Fatalf("expected AutoLinkMRForBranch to run for the current branch despite a sibling-branch association, got %d calls", fake.autoLinkCalls)
	}
}

// TestCheckSessionMR_EnsuresWatchForAlreadyLinkedMR covers the case an
// already-linked MR (via Create-MR action or manual URL linking, which write
// gitlab_task_mrs but no watch) is found for the exact current (repository,
// branch): the check must ensure its watch rather than just reporting found.
func TestCheckSessionMR_EnsuresWatchForAlreadyLinkedMR(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.taskMRs["t1"] = []*gitlab.TaskMR{
		{TaskID: "t1", RepositoryID: "repo1", ProjectPath: "group/myproj", HeadBranch: "feat/a", MRIID: 7, State: gitlabMRStateOpen},
	}

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for an already-linked MR on the current branch")
	}
	if fake.autoLinkCalls != 0 {
		t.Fatalf("expected no re-link search for an already-linked MR, got %d auto-link calls", fake.autoLinkCalls)
	}
	if fake.ensureWatchCalls != 1 {
		t.Fatalf("expected the missing watch to be ensured exactly once, got %d", fake.ensureWatchCalls)
	}
}

// TestCheckSessionMR_EnsuresWatchWhenNoMRFound mirrors CheckSessionPR's
// EnsurePRWatchForWorkspace-before-lookup ordering: even when no MR is open
// yet, a watch must exist afterward so the background poller keeps checking
// this branch instead of the caller needing to poll again themselves.
func TestCheckSessionMR_EnsuresWatchWhenNoMRFound(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if found {
		t.Fatal("expected found=false when no MR is open")
	}
	if fake.ensureWatchCalls != 1 {
		t.Fatalf("expected a placeholder watch to be ensured even when no MR was found, got %d ensure-watch calls", fake.ensureWatchCalls)
	}
}

// TestCheckSessionMR_RejectsGitHubRepository guards the gap cubic-dev-ai
// flagged: on-demand check has no router in front of it the way push
// detection has in dispatchPushDetection, so it must apply the same
// provider guard itself — otherwise calling gitlab.check_session_mr for a
// GitHub-backed session would install a bogus GitLab watch keyed off the
// GitHub repository's owner/name.
func TestCheckSessionMR_RejectsGitHubRepository(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "myrepo",
		SourceType: "provider", Provider: "github",
		ProviderOwner: "myorg", ProviderName: "myrepo",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	svc.SetGitLabMRLinkService(fake)

	found, err := svc.CheckSessionMR(ctx, "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if found {
		t.Fatal("expected found=false for a GitHub-provider repository")
	}
	if fake.autoLinkCalls != 0 || fake.ensureWatchCalls != 0 {
		t.Fatalf("expected zero GitLab calls for a GitHub-provider repository, got autoLink=%d ensureWatch=%d",
			fake.autoLinkCalls, fake.ensureWatchCalls)
	}
}

// TestResolvePushRepositoryProvider_RecognizesSelfManagedGitLabByConfiguredHost
// pins the gap cubic-dev-ai flagged: a self-managed GitLab repository never
// gets a durable "gitlab" Provider tag at all (only github.com/gitlab.com
// are recognized at discovery time), so the well-known-host live-checkout
// fallback can never resolve it either — that's a permanent gap, not a
// narrow backfill race. remote_url must instead be compared against the
// workspace's own configured GitLab connection.
func TestResolvePushRepositoryProvider_RecognizesSelfManagedGitLabByConfiguredHost(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	const selfManagedRemote = "https://gitlab.internal.example.com/group/widgets"
	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "widgets",
		SourceType: "provider", RemoteURL: selfManagedRemote,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	fake.isConfiguredGitLabHostFunc = func(_ context.Context, workspaceID, remoteURL string) bool {
		return workspaceID == "ws1" && remoteURL == selfManagedRemote
	}
	svc.SetGitLabMRLinkService(fake)

	provider := svc.resolvePushRepositoryProvider(ctx, "s1", "t1", "")
	if provider != gitlabProviderName {
		t.Fatalf("provider = %q, want gitlab (resolved via the workspace's configured GitLab host, not a github.com/gitlab.com allowlist)", provider)
	}
	if fake.isConfiguredGitLabHostCalls != 1 {
		t.Fatalf("expected IsConfiguredGitLabHost to be consulted exactly once, got %d", fake.isConfiguredGitLabHostCalls)
	}
}

// TestDetectPushAndAssociateMR_ClosedExistingMRDoesNotShadowReplacementSearch
// covers the replacement-MR gap cubic-dev-ai flagged: a merged/closed MR's
// leftover row must not make the "already linked" shortcut trust it
// indefinitely — otherwise a new MR opened later from the same branch is
// never found, and EnsureMRWatch's iid-replacement support (added for
// exactly this scenario) is never reached because the search that would
// discover the new iid never re-runs.
func TestDetectPushAndAssociateMR_ClosedExistingMRDoesNotShadowReplacementSearch(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.taskMRs["t1"] = []*gitlab.TaskMR{
		{TaskID: "t1", RepositoryID: "repo1", HeadBranch: "feat/a", MRIID: 9, State: "closed"},
	}
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 11, HeadBranch: branch}, nil
	}

	svc.detectPushAndAssociateMR(context.Background(), "s1", "t1", "", "feat/a")

	if fake.autoLinkCalls != 1 {
		t.Fatalf("expected the closed existing MR to not shadow a re-search, got %d AutoLinkMRForBranch calls", fake.autoLinkCalls)
	}
}

// TestCheckSessionMR_ClosedExistingMRDoesNotShadowReplacementSearch is the
// on-demand-check twin of the push-detection replacement-MR test above.
func TestCheckSessionMR_ClosedExistingMRDoesNotShadowReplacementSearch(t *testing.T) {
	svc, fake := seedGitLabSessionWithRepo(t)
	fake.taskMRs["t1"] = []*gitlab.TaskMR{
		{TaskID: "t1", RepositoryID: "repo1", HeadBranch: "feat/a", MRIID: 9, State: "merged"},
	}
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 11, HeadBranch: branch}, nil
	}

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after linking the replacement MR")
	}
	if fake.autoLinkCalls != 1 {
		t.Fatalf("expected the merged existing MR to not shadow a re-search, got %d AutoLinkMRForBranch calls", fake.autoLinkCalls)
	}
}

func TestCheckSessionMRDeniesForeignSession(t *testing.T) {
	called := false
	s := &Service{
		logger:             scopeTestLogger(t),
		sessionAccessCheck: denyingChecker(&called),
	}

	found, err := s.CheckSessionMR(context.Background(), "task-b", "sess-b")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !called {
		t.Fatal("session access checker was not consulted before the early return")
	}
	if found {
		t.Error("found = true; a denied session must not report an MR")
	}
}

func TestCheckSessionMRDeniesOwnSessionPairedWithForeignTask(t *testing.T) {
	s := ownSessionForeignTask()
	s.logger = scopeTestLogger(t)

	found, err := s.CheckSessionMR(context.Background(), "task-victim", "sess-mine")

	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if found {
		t.Error("found = true; an owned session must not unlock another user's task")
	}
}

// TestGitLabProjectPathFromRemoteURL covers the remote-URL shapes a
// self-managed GitLab (or a gitlab.com repo added as source_type: local)
// must resolve a project path from, including nested subgroups, since
// detectPushAndAssociateMR reassembles owner+"/"+repoName into the project
// path it sends to the GitLab API.
func TestGitLabProjectPathFromRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"self-managed ssh nested subgroup", "ssh://gitlab.example.com/group/subgroup/project", "group/subgroup/project"},
		{"self-managed https with .git", "https://gitlab.example.com/group/project.git", "group/project"},
		{"self-managed scp-style", "git@gitlab.example.com:group/subgroup/project.git", "group/subgroup/project"},
		{"gitlab.com", "https://gitlab.com/owner/repo", "owner/repo"},
		{"empty", "", ""},
		{"malformed", "not-a-url", ""},
		{"no project segment", "https://gitlab.example.com/onlyowner", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gitLabProjectPathFromRemoteURL(tt.url)
			if got != tt.want {
				t.Errorf("gitLabProjectPathFromRemoteURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// seedSelfManagedGitLabSessionWithRepo mirrors seedGitLabSessionWithRepo but
// reproduces the live-instance row shape for a self-managed GitLab
// repository: all four provider columns (provider, provider_host,
// provider_owner, provider_name) are empty, and remote_url is the only
// identity signal — source_type: local repos, and any repo added before
// self-managed hosts got a discovery-time provider tag, look exactly like
// this.
func seedSelfManagedGitLabSessionWithRepo(t *testing.T) (*Service, *fakeGitLabMRLinkService) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "forge",
		SourceType: "local",
		RemoteURL:  "ssh://gitlab.example.com/group/subgroup/project",
		CreatedAt:  now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1", CheckoutBranch: "feat/a",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	fake.isConfiguredGitLabHostFunc = func(_ context.Context, _, _ string) bool { return true }
	svc.SetGitLabMRLinkService(fake)
	return svc, fake
}

// TestDetectPushAndAssociateMR_SelfManagedGitLabResolvesProjectFromRemoteURL
// is the regression test for the self-managed-GitLab auto-link gap: a
// repository row with every provider column empty (source_type: local, or
// any repo added before self-managed hosts got a discovery-time tag) must
// still reach AutoLinkMRForBranch, deriving the project path from remote_url
// instead of the never-populated provider_owner/provider_name. Before the
// shared identity resolver fallback this failed with 0 AutoLinkMRForBranch
// calls, because resolvePushRepo alone returns empty owner/name for any
// non-github.com, non-provider-tagged repository.
func TestDetectPushAndAssociateMR_SelfManagedGitLabResolvesProjectFromRemoteURL(t *testing.T) {
	svc, fake := seedSelfManagedGitLabSessionWithRepo(t)
	var gotProjectPath string
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		gotProjectPath = projectPath
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 5, HeadBranch: branch}, nil
	}

	svc.detectPushAndAssociateMR(context.Background(), "s1", "t1", "", "feat/a")

	if fake.autoLinkCalls != 1 {
		t.Fatalf("expected 1 AutoLinkMRForBranch call for a self-managed GitLab repo with empty provider columns, got %d", fake.autoLinkCalls)
	}
	if gotProjectPath != "group/subgroup/project" {
		t.Fatalf("projectPath = %q, want %q (nested subgroup must survive)", gotProjectPath, "group/subgroup/project")
	}
	if fake.lastAutoLinkRepositoryID != "repo1" {
		t.Fatalf("repositoryID = %q, want repo1", fake.lastAutoLinkRepositoryID)
	}
}

// TestCheckSessionMR_SelfManagedGitLabResolvesProjectFromRemoteURL is the
// on-demand-check twin of the push-detection regression test above.
func TestCheckSessionMR_SelfManagedGitLabResolvesProjectFromRemoteURL(t *testing.T) {
	svc, fake := seedSelfManagedGitLabSessionWithRepo(t)
	var gotProjectPath string
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		gotProjectPath = projectPath
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 5, HeadBranch: branch}, nil
	}

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")

	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for a self-managed GitLab repo with empty provider columns")
	}
	if gotProjectPath != "group/subgroup/project" {
		t.Fatalf("projectPath = %q, want %q (nested subgroup must survive)", gotProjectPath, "group/subgroup/project")
	}
}

// TestDetectPushAndAssociatePR_SelfManagedGitLabRemoteStillBails pins the
// "do not widen the shared helper" decision behind the self-managed-GitLab
// auto-link fix: resolvePushRepo (shared with the GitHub path) must keep
// returning empty owner/name for a non-github.com remote even though
// shared identity resolver now has its own remote_url fallback on the
// GitLab-only path. If a future refactor widened resolvePushRepo instead,
// this repository would start reaching the GitHub API with a GitLab
// owner/name whenever routing ever misfired (e.g. no GitLab connection
// configured for the workspace).
func TestDetectPushAndAssociatePR_SelfManagedGitLabRemoteStillBails(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "forge",
		SourceType: "local",
		RemoteURL:  "ssh://gitlab.example.com/group/subgroup/project",
		CreatedAt:  now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ghSvc := &mockGitHubService{}
	svc.SetGitHubService(ghSvc)

	svc.detectPushAndAssociatePR(ctx, "s1", "t1", "", "feat/a")

	if ghSvc.getPRWatchBySessionRepoAndBranchCalls != 0 {
		t.Fatalf("expected detectPushAndAssociatePR to bail before any GitHub lookup for a GitLab remote (resolvePushRepo must still return empty owner/name), got %d watch lookups",
			ghSvc.getPRWatchBySessionRepoAndBranchCalls)
	}
}
