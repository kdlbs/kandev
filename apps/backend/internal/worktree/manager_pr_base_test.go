package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCreateWorktree_QualifiedPRBaseUsesUpstreamAndKeepsPRHead(t *testing.T) {
	repoPath, target, upstreamBaseOID, prHeadOID, forkMainOID, forkTargetOID := initQualifiedPRBaseFixture(t)
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager(): %v", err)
	}

	base := models.PRBase{Target: target, OID: upstreamBaseOID}
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID: "task-qualified-base", SessionID: "session-qualified-base",
		RepositoryID: "repo-qualified-base", RepositoryPath: repoPath,
		BaseBranch: target.TargetBranch, FallbackBaseBranch: "main",
		CheckoutBranch: target.HeadBranch, PRNumber: target.Number,
		QualifiedPRBase: &base, RemoteSyncHandled: true,
		TaskDirName: "task-qualified-base", RepoName: "repo-qualified-base",
	})
	if err != nil {
		t.Fatalf("Create() with qualified PR base: %v", err)
	}
	if got := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "HEAD")); got != prHeadOID {
		t.Fatalf("worktree HEAD = %q, want exact PR head %q", got, prHeadOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", target.ComparisonRef()+"^{commit}")); got != upstreamBaseOID {
		t.Fatalf("qualified target ref = %q, want upstream commit %q", got, upstreamBaseOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/remotes/origin/main")); got != forkMainOID {
		t.Fatalf("origin/main changed to %q, want fork commit %q", got, forkMainOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/remotes/origin/release/next")); got != forkTargetOID {
		t.Fatalf("origin/release/next changed to %q, want fork commit %q", got, forkTargetOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "config", "--get", "remote.origin.url")); got != filepath.Join(filepath.Dir(repoPath), "fork.git") {
		t.Fatalf("origin URL changed to %q", got)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "config", "--get", "remote.origin.pushurl")); got != "https://push.example/fork/widget.git" {
		t.Fatalf("origin push URL changed to %q", got)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "config", "--get", "remote."+target.ComparisonRemoteName()+".pushurl")); got != "DISABLED" {
		t.Fatalf("comparison push URL = %q, want disabled", got)
	}
}

func TestCreateWorktree_QualifiedPRBaseFailsOnOIDDriftWithoutFallback(t *testing.T) {
	repoPath, target, _, _, _, _ := initQualifiedPRBaseFixture(t)
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager(): %v", err)
	}
	base := models.PRBase{Target: target, OID: strings.Repeat("f", 40)}
	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID: "task-qualified-drift", SessionID: "session-qualified-drift",
		RepositoryID: "repo-qualified-drift", RepositoryPath: repoPath,
		BaseBranch: target.TargetBranch, FallbackBaseBranch: "main",
		CheckoutBranch: target.HeadBranch, PRNumber: target.Number,
		QualifiedPRBase: &base, RemoteSyncHandled: true,
		TaskDirName: "task-qualified-drift", RepoName: "repo-qualified-drift",
	})
	if err == nil || !strings.Contains(err.Error(), "OID changed") {
		t.Fatalf("Create() error = %v, want required qualified-base OID mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfg.TasksBasePath, "task-qualified-drift")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("qualified base mismatch created a task worktree: stat error = %v", statErr)
	}
}

func TestCreateWorktree_QualifiedPRBaseRemoteCollisionStopsFallback(t *testing.T) {
	repoPath, target, baseOID, _, forkMainOID, _ := initQualifiedPRBaseFixture(t)
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "config", "remote."+target.ComparisonRemoteName()+".url", "https://github.com/other/widget.git")
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager(): %v", err)
	}
	base := models.PRBase{Target: target, OID: baseOID}
	_, err = mgr.Create(context.Background(), CreateRequest{
		TaskID: "task-qualified-collision", SessionID: "session-qualified-collision",
		RepositoryID: "repo-qualified-collision", RepositoryPath: repoPath,
		BaseBranch: target.TargetBranch, FallbackBaseBranch: "main",
		CheckoutBranch: target.HeadBranch, PRNumber: target.Number,
		QualifiedPRBase: &base, RemoteSyncHandled: true,
		TaskDirName: "task-qualified-collision", RepoName: "repo-qualified-collision",
	})
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("Create() error = %v, want comparison remote collision", err)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/remotes/origin/main")); got != forkMainOID {
		t.Fatalf("origin/main changed to %q after collision", got)
	}
	if _, statErr := os.Stat(filepath.Join(cfg.TasksBasePath, "task-qualified-collision")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("remote collision created a fallback worktree: stat error = %v", statErr)
	}
}

func TestCreateWorktree_QualifiedPRBasePreservesCancellation(t *testing.T) {
	repoPath, target, baseOID, _, _, _ := initQualifiedPRBaseFixture(t)
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager(): %v", err)
	}
	base := models.PRBase{Target: target, OID: baseOID}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = mgr.Create(ctx, CreateRequest{
		TaskID: "task-qualified-cancel", SessionID: "session-qualified-cancel",
		RepositoryID: "repo-qualified-cancel", RepositoryPath: repoPath,
		BaseBranch: target.TargetBranch, FallbackBaseBranch: "main",
		CheckoutBranch: target.HeadBranch, PRNumber: target.Number,
		QualifiedPRBase: &base, RemoteSyncHandled: true,
		TaskDirName: "task-qualified-cancel", RepoName: "repo-qualified-cancel",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create() error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfg.TasksBasePath, "task-qualified-cancel")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled materialization created a fallback worktree: stat error = %v", statErr)
	}
}

func TestCheckoutCredentialEnvironmentAppliesWithoutCheckoutOptions(t *testing.T) {
	ctx, err := withCheckoutOptions(context.Background(), CreateRequest{CheckoutEnv: map[string]string{
		"KANDEV_TEST_GIT_CREDENTIAL": "scoped-value",
	}})
	if err != nil {
		t.Fatalf("withCheckoutOptions(): %v", err)
	}
	cmd := newGitCommand(ctx, "version")
	found := false
	for _, entry := range cmd.Env {
		if entry == "KANDEV_TEST_GIT_CREDENTIAL=scoped-value" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Git command did not receive the request-scoped credential environment")
	}
}

func TestRecreate_QualifiedPRBaseFailureLeavesExistingPathUntouched(t *testing.T) {
	repoPath, target, _, _, _, _ := initQualifiedPRBaseFixture(t)
	manager := newRecreateTestManager(t)
	worktreePath := filepath.Join(t.TempDir(), "task-preserved", "widget")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create existing worktree placeholder: %v", err)
	}
	canaryPath := filepath.Join(worktreePath, "uncommitted.txt")
	if err := os.WriteFile(canaryPath, []byte("keep this checkout"), 0o600); err != nil {
		t.Fatalf("write checkout canary: %v", err)
	}
	base := models.PRBase{Target: target, OID: strings.Repeat("f", 40)}
	_, err := manager.recreate(context.Background(), &Worktree{
		ID: "wt-qualified-recreate", TaskID: "task-preserved", SessionID: "session-preserved",
		RepositoryID: "repo-qualified-recreate", RepositoryPath: repoPath,
		Path: worktreePath, Branch: target.HeadBranch, Status: StatusDeleted,
	}, CreateRequest{
		TaskID: "task-preserved", SessionID: "session-preserved", RepositoryID: "repo-qualified-recreate",
		RepositoryPath: repoPath, BaseBranch: target.TargetBranch, CheckoutBranch: target.HeadBranch,
		PRNumber: target.Number, QualifiedPRBase: &base, RemoteSyncHandled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "OID changed") {
		t.Fatalf("recreate() error = %v, want required qualified-base OID mismatch", err)
	}
	if got, readErr := os.ReadFile(canaryPath); readErr != nil || string(got) != "keep this checkout" {
		t.Fatalf("existing checkout changed after failed preflight: contents=%q err=%v", got, readErr)
	}
}

func initQualifiedPRBaseFixture(t *testing.T) (string, models.ComparisonTarget, string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	globalGitConfig := filepath.Join(root, "empty-git-config")
	if err := os.WriteFile(globalGitConfig, nil, 0o600); err != nil {
		t.Fatalf("create isolated Git config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalGitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	upstreamBare := filepath.Join(root, "upstream.git")
	forkBare := filepath.Join(root, "fork.git")
	upstreamSeed := filepath.Join(root, "upstream-seed")
	forkSeed := filepath.Join(root, "fork-seed")

	runGit(t, root, "init", "--bare", "-b", "main", upstreamBare)
	runGit(t, root, "init", "-b", "main", upstreamSeed)
	configurePRBaseTestCommitter(t, upstreamSeed)
	writeRepoFile(t, upstreamSeed, "base.txt", "upstream main\n")
	runGit(t, upstreamSeed, "add", "base.txt")
	runGit(t, upstreamSeed, "commit", "-m", "upstream main")
	runGit(t, upstreamSeed, "remote", "add", "origin", upstreamBare)
	runGit(t, upstreamSeed, "push", "origin", "main")
	runGit(t, upstreamSeed, "checkout", "-b", "release/next")
	writeRepoFile(t, upstreamSeed, "target.txt", "upstream release target\n")
	runGit(t, upstreamSeed, "add", "target.txt")
	runGit(t, upstreamSeed, "commit", "-m", "upstream release target")
	runGit(t, upstreamSeed, "push", "origin", "release/next")
	upstreamBaseOID := strings.TrimSpace(runGit(t, upstreamSeed, "rev-parse", "HEAD"))
	runGit(t, upstreamSeed, "checkout", "-b", "feature/same-name")
	writeRepoFile(t, upstreamSeed, "pr.txt", "pull request head\n")
	runGit(t, upstreamSeed, "add", "pr.txt")
	runGit(t, upstreamSeed, "commit", "-m", "pull request head")
	prHeadOID := strings.TrimSpace(runGit(t, upstreamSeed, "rev-parse", "HEAD"))
	runGit(t, upstreamSeed, "push", "origin", "HEAD:refs/pull/42/head")

	runGit(t, root, "init", "--bare", "-b", "main", forkBare)
	runGit(t, root, "init", "-b", "main", forkSeed)
	configurePRBaseTestCommitter(t, forkSeed)
	writeRepoFile(t, forkSeed, "base.txt", "fork main\n")
	runGit(t, forkSeed, "add", "base.txt")
	runGit(t, forkSeed, "commit", "-m", "fork main")
	runGit(t, forkSeed, "remote", "add", "origin", forkBare)
	runGit(t, forkSeed, "push", "origin", "main")
	forkMainOID := strings.TrimSpace(runGit(t, forkSeed, "rev-parse", "HEAD"))
	runGit(t, forkSeed, "checkout", "-b", "release/next")
	writeRepoFile(t, forkSeed, "target.txt", "fork release branch\n")
	runGit(t, forkSeed, "add", "target.txt")
	runGit(t, forkSeed, "commit", "-m", "fork release branch")
	runGit(t, forkSeed, "push", "origin", "release/next")
	forkTargetOID := strings.TrimSpace(runGit(t, forkSeed, "rev-parse", "HEAD"))

	clonePath := filepath.Join(root, "checkout-main")
	repoPath := filepath.Join(root, "checkout-linked")
	runGit(t, root, "clone", forkBare, clonePath)
	runGit(t, clonePath, "config", "url.file://"+upstreamBare+".insteadOf", "https://github.com/upstream/widget.git")
	runGit(t, clonePath, "config", "remote.origin.pushurl", "https://push.example/fork/widget.git")
	runGit(t, clonePath, "worktree", "add", "-b", "local-checkout", repoPath, "main")
	target := models.ComparisonTarget{
		Version: models.ComparisonTargetVersion, Provider: models.ComparisonTargetProviderGitHub,
		Kind: models.ComparisonTargetKindPullRequest, Number: 42,
		HeadBranch: "feature/same-name", TargetBranch: "release/next",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "fork/widget", ProviderID: "fork-id",
			RemoteURL: "https://github.com/fork/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widget", ProviderID: "upstream-id",
			RemoteURL: "https://github.com/upstream/widget.git",
		},
	}
	if err := target.Validate(); err != nil {
		t.Fatalf("test comparison target invalid: %v", err)
	}
	return repoPath, target, upstreamBaseOID, prHeadOID, forkMainOID, forkTargetOID
}

func configurePRBaseTestCommitter(t *testing.T, repoPath string) {
	t.Helper()
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
}
