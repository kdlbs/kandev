package repoclone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestProviderRepoPathSeparatesProviderHosts(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{}, ProtocolHTTPS, t.TempDir(), logger.Default())
	gitHubPath, err := cloner.ProviderRepoPath("github", "https://github.com", "group", "project")
	if err != nil {
		t.Fatalf("GitHub ProviderRepoPath: %v", err)
	}
	gitLabPath, err := cloner.ProviderRepoPath("gitlab", "https://gitlab.com", "group", "project")
	if err != nil {
		t.Fatalf("GitLab ProviderRepoPath: %v", err)
	}
	selfManagedPath, err := cloner.ProviderRepoPath("gitlab", "https://gitlab.internal:8443", "group", "project")
	if err != nil {
		t.Fatalf("self-managed GitLab ProviderRepoPath: %v", err)
	}

	if gitHubPath == gitLabPath || gitLabPath == selfManagedPath || gitHubPath == selfManagedPath {
		t.Fatalf("provider paths collided: github=%q gitlab=%q self-managed=%q", gitHubPath, gitLabPath, selfManagedPath)
	}
	if !strings.Contains(selfManagedPath, filepath.Join("gitlab", "gitlab.internal-8443")) {
		t.Fatalf("self-managed path %q does not contain normalized provider and host", selfManagedPath)
	}
}

func TestClone_PreservesNonDefaultRemoteBranches(t *testing.T) {
	t.Parallel()

	originPath := initBareRepoWithReleaseBranch(t)
	targetPath := filepath.Join(t.TempDir(), "clone")

	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	if err := cloner.clone(context.Background(), originPath, targetPath, "", "", "", ""); err != nil {
		t.Fatalf("clone() unexpected error: %v", err)
	}

	if !gitRefExists(t, targetPath, "refs/remotes/origin/release") {
		t.Fatal("expected cloned repo to contain origin/release for downstream worktree base branches")
	}
}

func TestRepoPathConfinesRepositoryToCloneBase(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolHTTPS, "", logger.Default())

	path, err := cloner.RepoPath("group/subgroup", "repository")
	if err != nil {
		t.Fatalf("RepoPath() unexpected error: %v", err)
	}
	want := filepath.Join(basePath, "group", "subgroup", "repository")
	if path != want {
		t.Fatalf("RepoPath() = %q, want %q", path, want)
	}

	for _, test := range []struct {
		name  string
		owner string
		repo  string
	}{
		{name: "owner traversal", owner: "../../outside", repo: "repository"},
		{name: "repository traversal", owner: "group", repo: "../../../outside"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, pathErr := cloner.RepoPath(test.owner, test.repo); pathErr == nil {
				t.Fatal("RepoPath() accepted a path outside the clone base")
			}
		})
	}
}

func TestGitCmdBindsGitLabCredentialToExactOrigin(t *testing.T) {
	cloner := NewCloner(Config{}, ProtocolHTTPS, t.TempDir(), logger.Default())
	cmd, err := cloner.gitCmd(
		context.Background(),
		"https://gitlab.internal/group/repo.git",
		"https://gitlab.internal",
		"workspace-token",
		"clone",
	)
	if err != nil {
		t.Fatalf("gitCmd: %v", err)
	}
	joined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(joined, "KANDEV_GIT_CREDENTIAL_TOKEN=workspace-token") ||
		!strings.Contains(joined, "GIT_CONFIG_KEY_0=credential.https://gitlab.internal.helper") {
		t.Fatalf("credential env = %s", joined)
	}
	if _, err := cloner.gitCmd(
		context.Background(),
		"https://gitlab.com/group/repo.git",
		"https://gitlab.internal",
		"must-not-leak",
		"clone",
	); err == nil {
		t.Fatal("expected cross-host credential binding to fail")
	}
}

func TestGitCmdUsesSSHAuthForMatchingWorkspaceGitLabHost(t *testing.T) {
	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	for _, cloneURL := range []string{
		"git@gitlab.internal:group/repo.git",
		"ssh://git@gitlab.internal:2222/group/repo.git",
	} {
		cmd, err := cloner.gitCmd(
			context.Background(), cloneURL, "https://gitlab.internal:8443", "workspace-token", "clone",
		)
		if err != nil {
			t.Fatalf("gitCmd(%q): %v", cloneURL, err)
		}
		joined := strings.Join(cmd.Env, "\n")
		if strings.Contains(joined, "workspace-token") || strings.Contains(joined, "credential.gitlab") {
			t.Fatalf("SSH clone received HTTP credentials: %s", joined)
		}
	}
}

func TestGitCmdRejectsSSHCloneForDifferentWorkspaceGitLabHost(t *testing.T) {
	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	for _, cloneURL := range []string{
		"git@gitlab.other:group/repo.git",
		"ssh://git@gitlab.other:2222/group/repo.git",
	} {
		if _, err := cloner.gitCmd(
			context.Background(), cloneURL, "https://gitlab.internal", "must-not-leak", "clone",
		); err == nil {
			t.Fatalf("gitCmd accepted mismatched SSH clone %q", cloneURL)
		}
	}
}

func initBareRepoWithReleaseBranch(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	originPath := filepath.Join(root, "origin.git")
	workPath := filepath.Join(root, "work")

	runGit(t, root, "init", "--bare", "-b", "main", originPath)
	runGit(t, root, "clone", originPath, workPath)
	runGit(t, workPath, "config", "user.email", "test@example.com")
	runGit(t, workPath, "config", "user.name", "Test User")

	readmePath := filepath.Join(workPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, workPath, "add", "README.md")
	runGit(t, workPath, "commit", "-m", "main commit")
	runGit(t, workPath, "push", "origin", "main")

	runGit(t, workPath, "checkout", "-b", "release")
	if err := os.WriteFile(readmePath, []byte("release\n"), 0o644); err != nil {
		t.Fatalf("write release README.md: %v", err)
	}
	runGit(t, workPath, "commit", "-am", "release commit")
	runGit(t, workPath, "push", "origin", "release")

	return originPath
}

func gitRefExists(t *testing.T, repoPath, ref string) bool {
	t.Helper()

	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

func runGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return string(output)
}
