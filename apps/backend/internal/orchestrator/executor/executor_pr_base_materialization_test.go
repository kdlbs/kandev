package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestLegacyLinkedPRBaseFlowsIntoWorktreeMaterialization(t *testing.T) {
	root := t.TempDir()
	gitConfig := filepath.Join(root, "empty-git-config")
	if err := os.WriteFile(gitConfig, nil, 0o600); err != nil {
		t.Fatalf("create isolated Git config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repoPath, upstreamBaseOID, prHeadOID := initLegacyForkPRMaterializationRepos(t, root)

	target := forkPRComparisonTarget()
	target.HeadBranch = "feature/pr-42"
	target.TargetBranch = "release/next"
	resolver := &recordingPRBaseResolver{result: &models.PRBase{
		Target: *target, OID: upstreamBaseOID,
	}}
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", Name: "widgets", SourceType: sourceTypeLocal,
		LocalPath: repoPath, Provider: "github", ProviderOwner: "fork-owner", ProviderName: "widgets",
		DefaultBranch: "main",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetPRBaseResolver(resolver)
	info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
		CheckoutBranch: "feature/pr-42", Metadata: map[string]interface{}{"pr_number": 42},
	})
	if err != nil {
		t.Fatalf("resolve legacy task repository: %v", err)
	}
	specs := buildRepoSpecs([]*repoInfo{info})
	if len(specs) != 1 || specs[0].QualifiedPRBase == nil || specs[0].QualifiedPRBase.OID != upstreamBaseOID {
		t.Fatalf("resolved repository specs = %#v, want live qualified upstream target", specs)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	mgr, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: filepath.Join(root, "tasks"), BranchPrefix: "kandev/",
	}, nil, log)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	prepared, err := lifecycle.NewWorktreePreparer(mgr, log).Prepare(
		context.Background(), &lifecycle.EnvPrepareRequest{
			TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1", TaskDirName: "task-1",
			RepositoryID: specs[0].RepositoryID, TaskRepositoryID: specs[0].TaskRepositoryID,
			RepositoryPath: specs[0].RepositoryPath, RepoName: specs[0].RepoName,
			UseWorktree: true, BaseBranch: specs[0].BaseBranch, DefaultBranch: specs[0].DefaultBranch,
			CheckoutBranch: specs[0].CheckoutBranch, PRNumber: specs[0].PRNumber,
			QualifiedPRBase: specs[0].QualifiedPRBase, RemoteSyncHandled: true,
		}, nil,
	)
	if err != nil {
		t.Fatalf("prepare legacy fork PR worktree: %v", err)
	}
	if !prepared.Success {
		t.Fatalf("legacy fork PR preparation failed: %s", prepared.ErrorMessage)
	}
	if prepared.BaseBranch != "release/next" {
		t.Fatalf("prepared base = %q, want upstream release/next", prepared.BaseBranch)
	}
	if got := gitOutputInTest(t, prepared.WorkspacePath, "rev-parse", "HEAD"); got != prHeadOID {
		t.Fatalf("prepared HEAD = %q, want PR head %q", got, prHeadOID)
	}
	if got := gitOutputInTest(t, repoPath, "rev-parse", target.ComparisonRef()+"^{commit}"); got != upstreamBaseOID {
		t.Fatalf("qualified target ref = %q, want upstream OID %q", got, upstreamBaseOID)
	}
	if got := gitOutputInTest(t, repoPath, "rev-parse", "refs/remotes/origin/main"); got == "" {
		t.Fatal("origin/main disappeared during qualified materialization")
	}
}

func TestTargetAttachedForkPRBasePreparationEndToEnd(t *testing.T) {
	root := t.TempDir()
	gitConfig := filepath.Join(root, "empty-git-config")
	if err := os.WriteFile(gitConfig, nil, 0o600); err != nil {
		t.Fatalf("create isolated Git config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repoPath, targetOID, forkMainOID, prHeadOID := initTargetAttachedForkPRMaterializationRepos(t, root)
	if targetOID == forkMainOID {
		t.Fatal("upstream and fork main branches unexpectedly share an OID")
	}

	target := forkPRComparisonTarget()
	target.HeadBranch = "feature/pr-42"
	target.TargetBranch = "main"
	resolver := &recordingPRBaseResolver{result: &models.PRBase{
		Target: *target, OID: targetOID,
	}}
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", Name: "widgets", SourceType: sourceTypeLocal,
		LocalPath: repoPath, Provider: "github", ProviderOwner: "upstream", ProviderName: "widgets",
		DefaultBranch: "main",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetPRBaseResolver(resolver)
	taskRepo := &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
		CheckoutBranch: "feature/pr-42", Metadata: map[string]interface{}{"pr_number": 42},
	}
	info, err := exec.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolve target-attached PR task: %v", err)
	}
	retriedInfo, err := exec.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("retry target-attached PR task before preparation: %v", err)
	}
	if retriedInfo.PRBase == nil || info.PRBase == nil || !reflect.DeepEqual(retriedInfo.PRBase, info.PRBase) {
		t.Fatalf("retried PR identity = %#v, want unchanged provider identity %#v", retriedInfo.PRBase, info.PRBase)
	}
	info = retriedInfo
	if info.PRBase == nil || info.PRBase.Target.HeadRepository.Path != "fork-owner/widgets" ||
		info.QualifiedPRBase != nil || info.ComparisonTarget != nil {
		t.Fatalf("resolved PR identity = %#v, want transient fork source without a qualified target", info)
	}
	specs := buildRepoSpecs([]*repoInfo{info})
	if len(specs) != 1 || specs[0].BaseBranch != "main" || specs[0].PRNumber != 42 || specs[0].QualifiedPRBase != nil {
		t.Fatalf("target-attached repository spec = %#v, want ordinary main + PR-head preparation", specs)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	mgr, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: filepath.Join(root, "tasks"), BranchPrefix: "kandev/",
	}, nil, log)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	prepared, err := lifecycle.NewWorktreePreparer(mgr, log).Prepare(context.Background(), &lifecycle.EnvPrepareRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1", TaskDirName: "task-1",
		RepositoryID: specs[0].RepositoryID, TaskRepositoryID: specs[0].TaskRepositoryID,
		RepositoryPath: specs[0].RepositoryPath, RepoName: specs[0].RepoName,
		UseWorktree: true, BaseBranch: specs[0].BaseBranch, DefaultBranch: specs[0].DefaultBranch,
		CheckoutBranch: specs[0].CheckoutBranch, PRNumber: specs[0].PRNumber,
	}, nil)
	if err != nil {
		t.Fatalf("prepare target-attached fork PR worktree: %v", err)
	}
	if !prepared.Success || prepared.BaseBranch != "main" {
		t.Fatalf("prepared target-attached PR worktree = %#v, want successful main-based preparation", prepared)
	}
	if got := gitOutputInTest(t, prepared.WorkspacePath, "rev-parse", "HEAD"); got != prHeadOID {
		t.Fatalf("prepared HEAD = %q, want fork PR head %q", got, prHeadOID)
	}
	if got := gitOutputInTest(t, repoPath, "rev-parse", "refs/remotes/origin/main"); got != targetOID {
		t.Fatalf("origin/main = %q, want attached upstream OID %q", got, targetOID)
	}
	if got := gitOutputInTest(t, repoPath, "config", "--get", "remote.origin.url"); got != filepath.Join(root, "upstream.git") {
		t.Fatalf("origin URL = %q, want unchanged upstream remote", got)
	}
	if got := gitOutputInTest(t, repoPath, "config", "--get", "remote.origin.pushurl"); got != "https://push.example/upstream/widgets.git" {
		t.Fatalf("origin push URL = %q, want unchanged upstream push route", got)
	}
}

func initTargetAttachedForkPRMaterializationRepos(
	t *testing.T, root string,
) (repoPath, upstreamMainOID, forkMainOID, prHeadOID string) {
	t.Helper()
	upstreamBare := filepath.Join(root, "upstream.git")
	forkBare := filepath.Join(root, "fork.git")
	upstreamSeed := filepath.Join(root, "upstream-seed")
	forkSeed := filepath.Join(root, "fork-seed")
	gitCommandInTest(t, root, "init", "--bare", "-b", "main", upstreamBare)
	gitCommandInTest(t, root, "init", "-b", "main", upstreamSeed)
	configureLegacyForkPRCommitter(t, upstreamSeed)
	writeLegacyForkPRFile(t, upstreamSeed, "base.txt", "upstream main\n")
	gitCommandInTest(t, upstreamSeed, "add", "base.txt")
	gitCommandInTest(t, upstreamSeed, "commit", "-m", "upstream main")
	gitCommandInTest(t, upstreamSeed, "remote", "add", "origin", upstreamBare)
	gitCommandInTest(t, upstreamSeed, "push", "origin", "main")
	upstreamMainOID = gitOutputInTest(t, upstreamSeed, "rev-parse", "HEAD")
	gitCommandInTest(t, root, "init", "--bare", "-b", "main", forkBare)
	gitCommandInTest(t, root, "init", "-b", "main", forkSeed)
	configureLegacyForkPRCommitter(t, forkSeed)
	writeLegacyForkPRFile(t, forkSeed, "base.txt", "fork main\n")
	gitCommandInTest(t, forkSeed, "add", "base.txt")
	gitCommandInTest(t, forkSeed, "commit", "-m", "fork main")
	gitCommandInTest(t, forkSeed, "remote", "add", "origin", forkBare)
	gitCommandInTest(t, forkSeed, "push", "origin", "main")
	forkMainOID = gitOutputInTest(t, forkSeed, "rev-parse", "HEAD")
	gitCommandInTest(t, forkSeed, "checkout", "-b", "feature/pr-42")
	writeLegacyForkPRFile(t, forkSeed, "pr.txt", "fork pull request head\n")
	gitCommandInTest(t, forkSeed, "add", "pr.txt")
	gitCommandInTest(t, forkSeed, "commit", "-m", "fork pull request head")
	prHeadOID = gitOutputInTest(t, forkSeed, "rev-parse", "HEAD")
	gitCommandInTest(t, forkSeed, "push", "origin", "feature/pr-42")
	gitCommandInTest(t, forkSeed, "remote", "add", "upstream", upstreamBare)
	gitCommandInTest(t, forkSeed, "push", "upstream", "HEAD:refs/pull/42/head")
	repoPath = filepath.Join(root, "upstream-checkout")
	gitCommandInTest(t, root, "clone", upstreamBare, repoPath)
	gitCommandInTest(t, repoPath, "config", "remote.origin.pushurl", "https://push.example/upstream/widgets.git")
	return repoPath, upstreamMainOID, forkMainOID, prHeadOID
}

func initLegacyForkPRMaterializationRepos(t *testing.T, root string) (repoPath, upstreamBaseOID, prHeadOID string) {
	t.Helper()
	upstreamBare := filepath.Join(root, "upstream.git")
	forkBare := filepath.Join(root, "fork.git")
	upstreamSeed := filepath.Join(root, "upstream-seed")
	forkSeed := filepath.Join(root, "fork-seed")
	gitCommandInTest(t, root, "init", "--bare", "-b", "main", upstreamBare)
	gitCommandInTest(t, root, "init", "-b", "main", upstreamSeed)
	configureLegacyForkPRCommitter(t, upstreamSeed)
	writeLegacyForkPRFile(t, upstreamSeed, "base.txt", "upstream main\n")
	gitCommandInTest(t, upstreamSeed, "add", "base.txt")
	gitCommandInTest(t, upstreamSeed, "commit", "-m", "upstream main")
	gitCommandInTest(t, upstreamSeed, "remote", "add", "origin", upstreamBare)
	gitCommandInTest(t, upstreamSeed, "push", "origin", "main")
	gitCommandInTest(t, upstreamSeed, "checkout", "-b", "release/next")
	writeLegacyForkPRFile(t, upstreamSeed, "target.txt", "upstream release\n")
	gitCommandInTest(t, upstreamSeed, "add", "target.txt")
	gitCommandInTest(t, upstreamSeed, "commit", "-m", "upstream release")
	gitCommandInTest(t, upstreamSeed, "push", "origin", "release/next")
	upstreamBaseOID = gitOutputInTest(t, upstreamSeed, "rev-parse", "HEAD")
	gitCommandInTest(t, upstreamSeed, "checkout", "-b", "feature/pr-42")
	writeLegacyForkPRFile(t, upstreamSeed, "pr.txt", "pull request head\n")
	gitCommandInTest(t, upstreamSeed, "add", "pr.txt")
	gitCommandInTest(t, upstreamSeed, "commit", "-m", "pull request head")
	prHeadOID = gitOutputInTest(t, upstreamSeed, "rev-parse", "HEAD")
	gitCommandInTest(t, upstreamSeed, "push", "origin", "HEAD:refs/pull/42/head")

	gitCommandInTest(t, root, "init", "--bare", "-b", "main", forkBare)
	gitCommandInTest(t, root, "init", "-b", "main", forkSeed)
	configureLegacyForkPRCommitter(t, forkSeed)
	writeLegacyForkPRFile(t, forkSeed, "base.txt", "fork main\n")
	gitCommandInTest(t, forkSeed, "add", "base.txt")
	gitCommandInTest(t, forkSeed, "commit", "-m", "fork main")
	gitCommandInTest(t, forkSeed, "remote", "add", "origin", forkBare)
	gitCommandInTest(t, forkSeed, "push", "origin", "main")
	gitCommandInTest(t, forkSeed, "checkout", "-b", "release/next")
	writeLegacyForkPRFile(t, forkSeed, "target.txt", "fork release\n")
	gitCommandInTest(t, forkSeed, "add", "target.txt")
	gitCommandInTest(t, forkSeed, "commit", "-m", "fork release")
	gitCommandInTest(t, forkSeed, "push", "origin", "release/next")

	repoPath = filepath.Join(root, "fork-checkout")
	gitCommandInTest(t, root, "clone", forkBare, repoPath)
	gitCommandInTest(t, repoPath, "config", "url.file://"+upstreamBare+".insteadOf", "https://github.com/upstream/widgets.git")
	gitCommandInTest(t, repoPath, "config", "remote.origin.pushurl", "https://push.example/fork/widgets.git")
	return repoPath, upstreamBaseOID, prHeadOID
}

func configureLegacyForkPRCommitter(t *testing.T, path string) {
	t.Helper()
	gitCommandInTest(t, path, "config", "user.email", "test@example.com")
	gitCommandInTest(t, path, "config", "user.name", "Test User")
	gitCommandInTest(t, path, "config", "commit.gpgsign", "false")
}

func writeLegacyForkPRFile(t *testing.T, path, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}

func gitCommandInTest(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func gitOutputInTest(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
