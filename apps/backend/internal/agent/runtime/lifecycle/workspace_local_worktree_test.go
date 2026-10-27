package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func localWorktreeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.test"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestProvisionLocalPoolWorktreesCreatesWorktreeAndKeepsRemoteRepos(t *testing.T) {
	repo := t.TempDir()
	localWorktreeGit(t, repo, "init", "-q", "-b", "main")
	localWorktreeGit(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	workspace := poolWorkspace(t, "env-a", 1)
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{
		{RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/kandev.git", BaseBranch: "main"},
		{RepoName: "other", RepositoryURL: "https://example.test/other.git", BaseBranch: "main"},
	}}
	projection, err := remoteWorkspaceProjectionFromLaunch(req, true)
	if err != nil || len(projection) != 2 {
		t.Fatalf("projection: %v %d", err, len(projection))
	}
	instance := &ExecutorInstance{WorkspacePath: workspace, LocalWorktreeBranch: "kandev/pool/env-a"}
	remaining, err := provisionLocalPoolWorktrees(context.Background(), req, instance, projection)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || !strings.HasPrefix(remaining[0].Destination, "other-") {
		t.Fatalf("remaining projection = %+v", remaining)
	}
	wt := filepath.Join(workspace, projection[0].Destination)
	if got := localWorktreeGit(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); got != "kandev/pool/env-a-g1" {
		t.Fatalf("worktree branch = %q", got)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Fatal(err)
	}
	again, err := provisionLocalPoolWorktrees(context.Background(), req, instance, projection)
	if err != nil || len(again) != 1 {
		t.Fatalf("second call must reuse the worktree: %v %d", err, len(again))
	}
}

func TestProvisionLocalPoolWorktreesIsNoOpOffLocalNodes(t *testing.T) {
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{{RepoName: "kandev", RepositoryPath: "/x", RepositoryURL: "https://example.test/k.git", BaseBranch: "main"}}}
	projection, _ := remoteWorkspaceProjectionFromLaunch(req, true)
	got, err := provisionLocalPoolWorktrees(context.Background(), req, &ExecutorInstance{WorkspacePath: t.TempDir()}, projection)
	if err != nil || len(got) != 1 {
		t.Fatalf("remote node must keep materialization: %v %d", err, len(got))
	}
}

func TestProvisionLocalPoolWorktreesFailsClosedOnMissingBaseBranch(t *testing.T) {
	repo := t.TempDir()
	localWorktreeGit(t, repo, "init", "-q", "-b", "main")
	localWorktreeGit(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{{RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/k.git", BaseBranch: "nope"}}}
	projection, _ := remoteWorkspaceProjectionFromLaunch(req, true)
	if _, err := provisionLocalPoolWorktrees(context.Background(), req, &ExecutorInstance{WorkspacePath: t.TempDir(), LocalWorktreeBranch: "kandev/pool/e"}, projection); err == nil {
		t.Fatal("expected an error for an unknown base branch")
	}
}

func localWorktreeRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	localWorktreeGit(t, repo, "init", "-q", "-b", "main")
	localWorktreeGit(t, repo, "commit", "-q", "--allow-empty", "-m", "base")
	return repo
}

// poolWorkspace returns <root>/environments/<env>/g<generation>.
func poolWorkspace(t *testing.T, env string, generation int) string {
	t.Helper()
	ws := filepath.Join(t.TempDir(), "environments", env, "g"+strconv.Itoa(generation))
	if err := os.MkdirAll(ws, 0o750); err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestProvisionLocalPoolWorktreesRefusesUnsupportedOptionsBeforeCreatingAnything(t *testing.T) {
	repo := localWorktreeRepo(t)
	cases := map[string]RepoLaunchSpec{
		"checkout branch": {CheckoutBranch: "feature/x"},
		"pull request":    {PRNumber: 7},
		"qualified base":  {QualifiedPRBase: &models.PRBase{}},
		"contribution":    {RemoteContribution: &models.RemoteContribution{}},
		"sparse checkout": {CheckoutOptions: &models.RepositoryCheckoutOptions{DownloadMode: models.DownloadStandard, SparseDirectories: []string{"a"}}},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			extra.RepoName, extra.RepositoryPath, extra.RepositoryURL, extra.BaseBranch = "kandev", repo, "https://example.test/k.git", "main"
			req := &LaunchRequest{Repositories: []RepoLaunchSpec{extra}}
			projection, err := remoteWorkspaceProjectionFromLaunch(req, true)
			if err != nil {
				t.Fatal(err)
			}
			ws := poolWorkspace(t, "env-a", 1)
			instance := &ExecutorInstance{WorkspacePath: ws, LocalWorktreeBranch: "kandev/pool/env-a"}
			_, err = provisionLocalPoolWorktrees(context.Background(), req, instance, projection)
			if err == nil || !strings.Contains(err.Error(), "unsupported option: "+name) {
				t.Fatalf("expected an unsupported-option refusal, got %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(ws, projection[0].Destination)); statErr == nil {
				t.Fatal("a worktree was created before the refusal")
			}
		})
	}
}

func TestProvisionLocalPoolWorktreesUsesPerGenerationBranchAndDeletesItOnRollback(t *testing.T) {
	repo := localWorktreeRepo(t)
	localWorktreeGit(t, repo, "branch", "kandev/pool/env-a")
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{
		{RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/k.git", BaseBranch: "main"},
		{RepoName: "second", RepositoryPath: repo, RepositoryURL: "https://example.test/s.git", BaseBranch: "nope"},
	}}
	projection, _ := remoteWorkspaceProjectionFromLaunch(req, true)
	instance := &ExecutorInstance{WorkspacePath: poolWorkspace(t, "env-a", 2), LocalWorktreeBranch: "kandev/pool/env-a"}
	if _, err := provisionLocalPoolWorktrees(context.Background(), req, instance, projection); err == nil {
		t.Fatal("expected the second repository to fail")
	}
	branches := localWorktreeGit(t, repo, "branch", "--list", "kandev/pool/*")
	if strings.Contains(branches, "env-a-g2") {
		t.Fatalf("rollback left the created branch behind: %q", branches)
	}
	if !strings.Contains(branches, "kandev/pool/env-a") {
		t.Fatalf("a pre-existing branch must survive: %q", branches)
	}
	if _, err := os.Stat(filepath.Join(instance.WorkspacePath, projection[0].Destination)); err == nil {
		t.Fatal("rollback left the worktree behind")
	}
}

func TestProvisionLocalPoolWorktreesRejectsForeignGitFile(t *testing.T) {
	repo, other := localWorktreeRepo(t), localWorktreeRepo(t)
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{{RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/k.git", BaseBranch: "main"}}}
	projection, _ := remoteWorkspaceProjectionFromLaunch(req, true)
	ws := poolWorkspace(t, "env-a", 1)
	instance := &ExecutorInstance{WorkspacePath: ws, LocalWorktreeBranch: "kandev/pool/env-a"}
	// A worktree of a different repository sits at the expected path.
	localWorktreeGit(t, other, "worktree", "add", "-b", "kandev/pool/env-a-g1", filepath.Join(ws, projection[0].Destination), "main")
	if _, err := provisionLocalPoolWorktrees(context.Background(), req, instance, projection); err == nil {
		t.Fatal("a worktree of another repository must not be reused")
	}
}

func TestCreatedNomadReconcileProjectionAcceptsLocalPoolWorktreeByRepositoryAndBranch(t *testing.T) {
	repo := localWorktreeRepo(t)
	ws := poolWorkspace(t, "env-a", 1)
	spec := RepoLaunchSpec{RepoName: "kandev", BaseBranch: "main"}
	dest, err := remoteWorkspaceRepositoryDestination(spec)
	if err != nil {
		t.Fatal(err)
	}
	localWorktreeGit(t, repo, "worktree", "add", "-b", "kandev/pool/env-a-g1", filepath.Join(ws, dest), "main")
	info := &WorkspaceInfo{WorkspacePath: ws, WorkspaceRepositories: []WorkspaceRepositorySpec{
		{RepositoryID: "r", RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/k.git", BaseBranch: "main"},
	}}
	got, err := createdNomadReconcileProjection(info)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a verified local worktree must not be sent to agentctl materialization: %+v", got)
	}
	localWorktreeGit(t, filepath.Join(ws, dest), "checkout", "-q", "-b", "other")
	if _, err := createdNomadReconcileProjection(info); err == nil {
		t.Fatal("a worktree on the wrong branch must fail closed")
	}
}

func TestProvisionLocalPoolWorktreesLeftoverEnvironmentBranchDoesNotBlockNextGeneration(t *testing.T) {
	repo := localWorktreeRepo(t)
	localWorktreeGit(t, repo, "branch", "kandev/pool/env-a")
	req := &LaunchRequest{Repositories: []RepoLaunchSpec{{RepoName: "kandev", RepositoryPath: repo, RepositoryURL: "https://example.test/k.git", BaseBranch: "main"}}}
	projection, _ := remoteWorkspaceProjectionFromLaunch(req, true)
	ws := poolWorkspace(t, "env-a", 2)
	instance := &ExecutorInstance{WorkspacePath: ws, LocalWorktreeBranch: "kandev/pool/env-a"}
	if _, err := provisionLocalPoolWorktrees(context.Background(), req, instance, projection); err != nil {
		t.Fatal(err)
	}
	if got := localWorktreeGit(t, filepath.Join(ws, projection[0].Destination), "rev-parse", "--abbrev-ref", "HEAD"); got != "kandev/pool/env-a-g2" {
		t.Fatalf("worktree branch = %q", got)
	}
}
