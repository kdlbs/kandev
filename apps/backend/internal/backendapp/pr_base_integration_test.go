package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	githubpkg "github.com/kandev/kandev/internal/github"
	executorpkg "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestForkPRBasePreparationEndToEnd(t *testing.T) {
	root := t.TempDir()
	gitConfig := filepath.Join(root, "empty-git-config")
	if err := os.WriteFile(gitConfig, nil, 0o600); err != nil {
		t.Fatalf("create isolated Git config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repoPath, upstreamBaseOID, prHeadOID, forkMainOID, forkTargetOID := initForkPRLaunchFixture(t, root)

	providerPR := &githubpkg.PR{
		Number: 42, RepoOwner: "upstream", RepoName: "widget", BaseRepoOwner: "upstream", BaseRepoName: "widget",
		HeadRepoOwner: "fork-owner", HeadRepoName: "widget", HeadBranch: "feature/pr-42",
		BaseBranch: "release/next", BaseSHA: upstreamBaseOID,
	}
	base, err := githubPRBaseFromPR(providerPR, "upstream", "widget", 42, "feature/pr-42")
	if err != nil {
		t.Fatalf("convert provider PR response: %v", err)
	}
	if base.OID != upstreamBaseOID {
		t.Fatalf("provider target OID = %q, want upstream %q", base.OID, upstreamBaseOID)
	}

	taskRepositoryMetadata := map[string]interface{}{}
	if err := models.PutComparisonTarget(taskRepositoryMetadata, &base.Target); err != nil {
		t.Fatalf("store task comparison target: %v", err)
	}
	storedTarget, found, err := models.LoadComparisonTarget(taskRepositoryMetadata)
	if err != nil || !found || !storedTarget.Equal(base.Target) {
		t.Fatalf("stored comparison target = %#v, found=%v, err=%v", storedTarget, found, err)
	}

	launchRequest := buildLifecycleLaunchRequest(&executorpkg.LaunchAgentRequest{
		TaskID: "task-fork-pr", WorkspaceID: "workspace-fork-pr", SessionID: "session-fork-pr",
		TaskDirName: "task-fork-pr", RepoName: "widget", UseWorktree: true,
		Repositories: []executorpkg.RepoSpec{{
			TaskRepositoryID: "task-repo-fork-pr", RepositoryID: "repo-fork-pr", RepositoryPath: repoPath,
			RepoName: "widget", BaseBranch: base.Target.TargetBranch, DefaultBranch: "main",
			CheckoutBranch: base.Target.HeadBranch, PRNumber: base.Target.Number,
			ComparisonTarget: &base.Target, QualifiedPRBase: &base, RemoteSyncHandled: true,
		}},
	}, filepath.Join(root, "workspace"), "")
	if len(launchRequest.Repositories) != 1 {
		t.Fatalf("lifecycle launch repository count = %d, want 1", len(launchRequest.Repositories))
	}
	repoSpec := launchRequest.Repositories[0]
	if repoSpec.QualifiedPRBase == nil || !reflect.DeepEqual(*repoSpec.QualifiedPRBase, base) ||
		repoSpec.ComparisonTarget == nil || !repoSpec.ComparisonTarget.Equal(base.Target) || repoSpec.PRNumber != 42 {
		t.Fatalf("lifecycle launch repository lost provider identity: %#v", repoSpec)
	}

	worktreeConfig := worktree.Config{
		Enabled: true, TasksBasePath: filepath.Join(root, "tasks"), BranchPrefix: "kandev/",
	}
	mgr, err := worktree.NewManager(worktreeConfig, nil, newTestLogger())
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	preparer := lifecycle.NewWorktreePreparer(mgr, newTestLogger())
	prepared, err := preparer.Prepare(context.Background(), &lifecycle.EnvPrepareRequest{
		TaskID: launchRequest.TaskID, WorkspaceID: launchRequest.WorkspaceID, SessionID: launchRequest.SessionID,
		RepositoryID: repoSpec.RepositoryID, TaskRepositoryID: repoSpec.TaskRepositoryID,
		RepositoryPath: repoSpec.RepositoryPath, UseWorktree: true,
		TaskDirName: launchRequest.TaskDirName, RepoName: repoSpec.RepoName,
		BaseBranch: repoSpec.BaseBranch, DefaultBranch: repoSpec.DefaultBranch,
		CheckoutBranch: repoSpec.CheckoutBranch, PRNumber: repoSpec.PRNumber,
		QualifiedPRBase:   repoSpec.QualifiedPRBase,
		RemoteSyncHandled: repoSpec.RemoteSyncHandled,
	}, nil)
	if err != nil {
		t.Fatalf("prepare fork PR worktree: %v", err)
	}
	if !prepared.Success {
		t.Fatalf("fork PR preparation failed: %s", prepared.ErrorMessage)
	}
	if prepared.BaseBranch != "release/next" || prepared.RequestedBaseBranch != "release/next" {
		t.Fatalf("prepared base = %q (requested %q), want release/next", prepared.BaseBranch, prepared.RequestedBaseBranch)
	}
	if got := strings.TrimSpace(runGit(t, prepared.WorkspacePath, "rev-parse", "HEAD")); got != prHeadOID {
		t.Fatalf("prepared HEAD = %q, want PR head %q", got, prHeadOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", base.Target.ComparisonRef()+"^{commit}")); got != upstreamBaseOID {
		t.Fatalf("qualified target ref = %q, want upstream OID %q", got, upstreamBaseOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/remotes/origin/main")); got != forkMainOID {
		t.Fatalf("origin/main = %q, want unchanged fork OID %q", got, forkMainOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "rev-parse", "refs/remotes/origin/release/next")); got != forkTargetOID {
		t.Fatalf("origin/release/next = %q, want unchanged fork OID %q", got, forkTargetOID)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "config", "--get", "remote.origin.url")); got != filepath.Join(root, "fork.git") {
		t.Fatalf("origin URL = %q, want fork remote", got)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "config", "--get", "remote.origin.pushurl")); got != "https://push.example/fork/widget.git" {
		t.Fatalf("origin push URL = %q, want unchanged fork push route", got)
	}
	if got := strings.TrimSpace(runGit(t, repoPath, "config", "--get", "remote."+base.Target.ComparisonRemoteName()+".pushurl")); got != "DISABLED" {
		t.Fatalf("comparison remote push URL = %q, want disabled", got)
	}
	finalTarget, found, err := models.LoadComparisonTarget(taskRepositoryMetadata)
	if err != nil || !found || !finalTarget.Equal(base.Target) {
		t.Fatalf("task comparison metadata changed during preparation: target=%#v found=%v err=%v", finalTarget, found, err)
	}
}

func initForkPRLaunchFixture(t *testing.T, root string) (repoPath, upstreamBaseOID, prHeadOID, forkMainOID, forkTargetOID string) {
	t.Helper()
	upstreamBare := filepath.Join(root, "upstream.git")
	forkBare := filepath.Join(root, "fork.git")
	upstreamSeed := filepath.Join(root, "upstream-seed")
	forkSeed := filepath.Join(root, "fork-seed")
	runGit(t, root, "init", "--bare", "-b", "main", upstreamBare)
	runGit(t, root, "init", "-b", "main", upstreamSeed)
	configureForkPRLaunchCommitter(t, upstreamSeed)
	writeForkPRLaunchFile(t, upstreamSeed, "base.txt", "upstream main\n")
	runGit(t, upstreamSeed, "add", "base.txt")
	runGit(t, upstreamSeed, "commit", "-m", "upstream main")
	runGit(t, upstreamSeed, "remote", "add", "origin", upstreamBare)
	runGit(t, upstreamSeed, "push", "origin", "main")
	runGit(t, upstreamSeed, "checkout", "-b", "release/next")
	writeForkPRLaunchFile(t, upstreamSeed, "target.txt", "upstream release\n")
	runGit(t, upstreamSeed, "add", "target.txt")
	runGit(t, upstreamSeed, "commit", "-m", "upstream release")
	runGit(t, upstreamSeed, "push", "origin", "release/next")
	upstreamBaseOID = strings.TrimSpace(runGit(t, upstreamSeed, "rev-parse", "HEAD"))
	runGit(t, upstreamSeed, "checkout", "-b", "feature/pr-42")
	writeForkPRLaunchFile(t, upstreamSeed, "pr.txt", "pull request head\n")
	runGit(t, upstreamSeed, "add", "pr.txt")
	runGit(t, upstreamSeed, "commit", "-m", "pull request head")
	prHeadOID = strings.TrimSpace(runGit(t, upstreamSeed, "rev-parse", "HEAD"))
	runGit(t, upstreamSeed, "push", "origin", "HEAD:refs/pull/42/head")

	runGit(t, root, "init", "--bare", "-b", "main", forkBare)
	runGit(t, root, "init", "-b", "main", forkSeed)
	configureForkPRLaunchCommitter(t, forkSeed)
	writeForkPRLaunchFile(t, forkSeed, "base.txt", "fork main\n")
	runGit(t, forkSeed, "add", "base.txt")
	runGit(t, forkSeed, "commit", "-m", "fork main")
	runGit(t, forkSeed, "remote", "add", "origin", forkBare)
	runGit(t, forkSeed, "push", "origin", "main")
	forkMainOID = strings.TrimSpace(runGit(t, forkSeed, "rev-parse", "HEAD"))
	runGit(t, forkSeed, "checkout", "-b", "release/next")
	writeForkPRLaunchFile(t, forkSeed, "target.txt", "fork release\n")
	runGit(t, forkSeed, "add", "target.txt")
	runGit(t, forkSeed, "commit", "-m", "fork release")
	runGit(t, forkSeed, "push", "origin", "release/next")
	forkTargetOID = strings.TrimSpace(runGit(t, forkSeed, "rev-parse", "HEAD"))

	repoPath = filepath.Join(root, "fork-checkout")
	runGit(t, root, "clone", forkBare, repoPath)
	runGit(t, repoPath, "config", "url.file://"+upstreamBare+".insteadOf", "https://github.com/upstream/widget.git")
	runGit(t, repoPath, "config", "remote.origin.pushurl", "https://push.example/fork/widget.git")
	return repoPath, upstreamBaseOID, prHeadOID, forkMainOID, forkTargetOID
}

func configureForkPRLaunchCommitter(t *testing.T, repoPath string) {
	t.Helper()
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
}

func writeForkPRLaunchFile(t *testing.T, repoPath, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repoPath, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
