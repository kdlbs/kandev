package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
)

// seedLocalGitCheckoutForProviderTest writes a minimal ".git/config" (no real
// git binary needed) whose origin remote is remoteURL, mirroring the on-disk
// shape service.ResolveGitRemoteProviderIdentity reads.
func seedLocalGitCheckoutForProviderTest(t *testing.T, dir, remoteURL string) {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir git dir: %v", err)
	}
	config := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = " + remoteURL + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write git config: %v", err)
	}
}

// seedUnbackfilledRepoForProviderTest seeds a session/task/repository row
// with no durable provider tag yet (Provider/ProviderOwner both blank) whose
// LocalPath is a local checkout of remoteURL — the pre-backfill state
// matchPushRepo's detached goroutine would otherwise fill in.
func seedUnbackfilledRepoForProviderTest(t *testing.T, remoteURL string) *Service {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	localPath := t.TempDir()
	seedLocalGitCheckoutForProviderTest(t, localPath, remoteURL)

	repoObj := &models.Repository{
		ID: "repo1", WorkspaceID: "ws1", Name: "widgets",
		SourceType: "local", LocalPath: localPath,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateRepository(ctx, repoObj); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "tr1", TaskID: "t1", RepositoryID: "repo1",
		CheckoutBranch: "feat/a",
		CreatedAt:      now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	session, _ := repo.GetTaskSession(ctx, "s1")
	session.RepositoryID = "repo1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	return createTestService(repo, newMockStepGetter(), newMockTaskRepo())
}

// TestResolvePushRepositoryProvider_LiveFallbackForUnbackfilledGitHubRepo
// closes the race cubic-dev-ai flagged: a repository row with no durable
// provider yet (ProviderOwner == "", matchPushRepo's own trigger condition
// for its detached backfill goroutine) must not route to the wrong provider
// just because that backfill's DB write hasn't landed by the time
// resolvePushRepositoryProvider does its own separate read. Recomputing live
// from the same local checkout removes the dependency on that write's
// timing.
func TestResolvePushRepositoryProvider_LiveFallbackForUnbackfilledGitHubRepo(t *testing.T) {
	svc := seedUnbackfilledRepoForProviderTest(t, "https://github.com/acme/widgets.git")

	provider := svc.resolvePushRepositoryProvider(context.Background(), "s1", "t1", "")
	if provider != "github" {
		t.Fatalf("provider = %q, want %q (resolved live from the local checkout, not a possibly-stale DB column)", provider, "github")
	}
}

// TestResolvePushRepositoryProvider_LiveFallbackForUnbackfilledGitLabRepo
// pins the same guarantee for GitLab: the live fallback must use a helper
// that recognizes gitlab.com (ResolveGitRemoteProviderIdentity), not one
// that only recognizes GitHub (ResolveGitRemoteProvider) — otherwise an
// unbackfilled GitLab repository's push is silently routed to the GitHub
// detection path and never auto-links.
func TestResolvePushRepositoryProvider_LiveFallbackForUnbackfilledGitLabRepo(t *testing.T) {
	svc := seedUnbackfilledRepoForProviderTest(t, "https://gitlab.com/acme/widgets.git")

	provider := svc.resolvePushRepositoryProvider(context.Background(), "s1", "t1", "")
	if provider != gitlabProviderName {
		t.Fatalf("provider = %q, want %q (resolved live from the local checkout, not a possibly-stale DB column)", provider, gitlabProviderName)
	}
}

// TestResolvePushRepositoryProvider_LiveFallbackForUnbackfilledSelfManagedGitLabRepo
// closes the routing gap CodeRabbit flagged on PR #2515: a self-managed
// GitLab repository whose remote_url column hasn't been backfilled yet
// (LocalPath is the only signal) previously routed nowhere, because the
// self-managed-host check only ever compared repoObj.RemoteURL —  never a
// live read of LocalPath — against the workspace's configured GitLab host.
// dispatchPushDetection therefore sent the push down the GitHub detection
// path, and the shared identity resolver's local-checkout fallback
// (event_handlers_git.go) never got a chance to run because routing
// bailed one step earlier than that fallback.
func TestResolvePushRepositoryProvider_LiveFallbackForUnbackfilledSelfManagedGitLabRepo(t *testing.T) {
	const selfManagedRemote = "https://gitlab.internal.example.com/group/subgroup/widgets"
	svc := seedUnbackfilledRepoForProviderTest(t, selfManagedRemote)

	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	fake.isConfiguredGitLabHostFunc = func(_ context.Context, workspaceID, remoteURL string) bool {
		return workspaceID == "ws1" && remoteURL == selfManagedRemote
	}
	svc.SetGitLabMRLinkService(fake)

	provider := svc.resolvePushRepositoryProvider(context.Background(), "s1", "t1", "")
	if provider != gitlabProviderName {
		t.Fatalf("provider = %q, want %q (resolved live from the local checkout's remote, not requiring a persisted remote_url)", provider, gitlabProviderName)
	}
	if fake.isConfiguredGitLabHostCalls != 1 {
		t.Fatalf("expected IsConfiguredGitLabHost to be consulted exactly once, got %d", fake.isConfiguredGitLabHostCalls)
	}
}

// TestDispatchPushDetection_LiveFallbackForUnbackfilledSelfManagedGitLabRepo
// covers the complete push path for a legacy row whose only repository
// identity is the local checkout's origin. Provider routing and project-path
// resolution must agree so the push reaches GitLab rather than falling back to
// GitHub or being dropped before the MR lookup.
func TestDispatchPushDetection_LiveFallbackForUnbackfilledSelfManagedGitLabRepo(t *testing.T) {
	const selfManagedRemote = "https://gitlab.internal.example.com/group/subgroup/widgets"
	svc := seedUnbackfilledRepoForProviderTest(t, selfManagedRemote)

	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	fake.isConfiguredGitLabHostFunc = func(_ context.Context, workspaceID, remoteURL string) bool {
		return workspaceID == "ws1" && remoteURL == selfManagedRemote
	}
	var gotProjectPath string
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		gotProjectPath = projectPath
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 5, HeadBranch: branch}, nil
	}
	svc.SetGitLabMRLinkService(fake)
	ghSvc := &mockGitHubService{}
	svc.SetGitHubService(ghSvc)

	svc.dispatchPushDetection(context.Background(), "s1", "t1", "", "feat/a")

	if fake.autoLinkCalls != 1 {
		t.Fatalf("expected one GitLab auto-link call, got %d", fake.autoLinkCalls)
	}
	if gotProjectPath != "group/subgroup/widgets" {
		t.Fatalf("projectPath = %q, want %q", gotProjectPath, "group/subgroup/widgets")
	}
	if fake.lastAutoLinkRepositoryID != "repo1" {
		t.Fatalf("repositoryID = %q, want repo1", fake.lastAutoLinkRepositoryID)
	}
	if ghSvc.getPRWatchBySessionRepoAndBranchCalls != 0 {
		t.Fatalf("expected zero GitHub lookups for a self-managed GitLab repository, got %d", ghSvc.getPRWatchBySessionRepoAndBranchCalls)
	}
}

// TestCheckSessionMR_LiveFallbackForUnbackfilledSelfManagedGitLabRepo covers
// the on-demand path for the same local-only repository shape. It must apply
// the provider guard and pass the local checkout's full nested project path to
// GitLab before creating the association.
func TestCheckSessionMR_LiveFallbackForUnbackfilledSelfManagedGitLabRepo(t *testing.T) {
	const selfManagedRemote = "https://gitlab.internal.example.com/group/subgroup/widgets"
	svc := seedUnbackfilledRepoForProviderTest(t, selfManagedRemote)

	fake := &fakeGitLabMRLinkService{taskMRs: make(map[string][]*gitlab.TaskMR)}
	fake.isConfiguredGitLabHostFunc = func(_ context.Context, workspaceID, remoteURL string) bool {
		return workspaceID == "ws1" && remoteURL == selfManagedRemote
	}
	var gotProjectPath string
	fake.autoLinkFunc = func(_ context.Context, _, _, _, repositoryID, projectPath, branch string) (*gitlab.TaskMR, error) {
		gotProjectPath = projectPath
		return &gitlab.TaskMR{TaskID: "t1", RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: 5, HeadBranch: branch}, nil
	}
	svc.SetGitLabMRLinkService(fake)

	found, err := svc.CheckSessionMR(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatalf("CheckSessionMR: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for a self-managed GitLab repository with only a local origin")
	}
	if gotProjectPath != "group/subgroup/widgets" {
		t.Fatalf("projectPath = %q, want %q", gotProjectPath, "group/subgroup/widgets")
	}
}
