package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		CreatedAt: now, UpdatedAt: now,
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
