package repoclone

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestClone_PreservesNonDefaultRemoteBranches(t *testing.T) {
	t.Parallel()

	originPath := initBareRepoWithReleaseBranch(t)
	targetPath := filepath.Join(t.TempDir(), "clone")

	cloner := NewCloner(Config{}, ProtocolSSH, t.TempDir(), logger.Default())
	if err := cloner.clone(context.Background(), originPath, targetPath, nil); err != nil {
		t.Fatalf("clone() unexpected error: %v", err)
	}

	if !gitRefExists(t, targetPath, "refs/remotes/origin/release") {
		t.Fatal("expected cloned repo to contain origin/release for downstream worktree base branches")
	}
}

func TestWorkspaceRepoPathIsolatesManagedClones(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolSSH, "", logger.Default())
	first, err := cloner.WorkspaceRepoPath("workspace-a", "github", "acme", "private")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath(workspace-a): %v", err)
	}
	second, err := cloner.WorkspaceRepoPath("workspace-b", "github", "acme", "private")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath(workspace-b): %v", err)
	}
	if first == second {
		t.Fatalf("workspace clone paths must differ, both were %q", first)
	}
	if !cloner.ShouldRecloneForWorkspace("workspace-b", first) {
		t.Fatal("workspace-b must not reuse workspace-a's managed clone")
	}
	if cloner.ShouldRecloneForWorkspace("workspace-a", first) {
		t.Fatal("workspace-a should reuse its own managed clone")
	}
	legacy := filepath.Join(basePath, "acme", "private")
	if !cloner.ShouldRecloneForWorkspace("workspace-a", legacy) {
		t.Fatal("legacy shared managed clone must be rematerialized")
	}
}

func TestWorkspaceRepoPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	for _, testCase := range []struct {
		name, workspaceID, owner, repo string
	}{
		{name: "workspace", workspaceID: "../outside", owner: "acme", repo: "private"},
		{name: "owner", workspaceID: "workspace", owner: "../outside", repo: "private"},
		{name: "repo", workspaceID: "workspace", owner: "acme", repo: "../outside"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := cloner.WorkspaceRepoPath(testCase.workspaceID, "github", testCase.owner, testCase.repo); err == nil {
				t.Fatal("WorkspaceRepoPath() expected traversal error")
			}
		})
	}
}

func TestWorkspaceRepoPathSupportsNestedProviderOwner(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	cloner := NewCloner(Config{BasePath: basePath}, ProtocolHTTPS, "", logger.Default())
	path, err := cloner.WorkspaceRepoPath("workspace-a", "gitlab", "group/subgroup", "repository")
	if err != nil {
		t.Fatalf("WorkspaceRepoPath() unexpected error: %v", err)
	}
	want := filepath.Join(basePath, "workspaces", "workspace-a", "gitlab", "group", "subgroup", "repository")
	if path != want {
		t.Fatalf("WorkspaceRepoPath() = %q, want %q", path, want)
	}
}

func TestEnsureWorkspaceClonedUsesSelectedCredentialWithoutAmbientFallback(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	capturePath := filepath.Join(root, "capture")
	gitPath := filepath.Join(binDir, "git")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$KANDEV_TEST_CAPTURE.args"
printf '%s\n' "$GH_TOKEN|$GITHUB_TOKEN|$GIT_CONFIG_GLOBAL|$GIT_CONFIG_NOSYSTEM|$KANDEV_REPOCLONE_GITHUB_USERNAME|$KANDEV_REPOCLONE_GITHUB_TOKEN|$GIT_CONFIG_VALUE_1" > "$KANDEV_TEST_CAPTURE.env"
`
	if err := os.WriteFile(gitPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KANDEV_TEST_CAPTURE", capturePath)
	t.Setenv("GH_TOKEN", "ambient-gh-token")
	t.Setenv("GITHUB_TOKEN", "ambient-github-token")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_0", "!malicious-helper")

	credentials := &recordingCredentialProvider{password: "workspace-token"}
	cloner := NewCloner(Config{BasePath: filepath.Join(root, "repos")}, ProtocolSSH, "", logger.Default())
	cloner.SetGitCredentialProvider(credentials)
	target, err := cloner.EnsureWorkspaceCloned(
		context.Background(), "workspace-a", "github", "git@github.com:acme/private.git", "acme", "private",
	)
	if err != nil {
		t.Fatalf("EnsureWorkspaceCloned(): %v", err)
	}
	if credentials.workspaceID != "workspace-a" {
		t.Fatalf("credential workspace = %q, want workspace-a", credentials.workspaceID)
	}
	if !strings.Contains(target, filepath.Join("workspaces", "workspace-a", "github", "acme", "private")) {
		t.Fatalf("target path %q is not workspace isolated", target)
	}
	args := readTestFile(t, capturePath+".args")
	if !strings.Contains(args, "https://github.com/acme/private.git") {
		t.Fatalf("git args do not contain credential-compatible HTTPS URL: %s", args)
	}
	if strings.Contains(args, "workspace-token") || strings.Contains(args, "ambient-") {
		t.Fatalf("git args leaked credential material: %s", args)
	}
	env := strings.TrimSpace(readTestFile(t, capturePath+".env"))
	wantParts := []string{"", "", os.DevNull, "1", "x-access-token", "workspace-token", gitCredentialHelper}
	if got, want := strings.Split(env, "|"), wantParts; strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("git auth environment = %#v, want %#v", got, want)
	}
	assertUniqueGitConfigEnv(t, cloner.gitCmd(context.Background(), &cloneAuth{
		host: "github.com", username: "x-access-token", password: "workspace-token",
	}).Env)
}

func TestManagedGitCommandExecutesWithCompleteConfigAndNoAmbientAuth(t *testing.T) {
	t.Setenv("GH_TOKEN", "ambient-gh-token")
	t.Setenv("GITHUB_TOKEN", "ambient-github-token")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_0", "!printf 'password=ambient-token\\n'")

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	cmd := cloner.gitCmd(context.Background(), &cloneAuth{
		host: "github.com", username: "workspace-user", password: "workspace-token",
	}, "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=acme/private.git\n\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("managed git credential command failed: %v\n%s", err, out)
	}
	got := string(out)
	if !strings.Contains(got, "username=workspace-user") || !strings.Contains(got, "password=workspace-token") {
		t.Fatalf("managed credential output = %q", got)
	}
	if strings.Contains(got, "ambient-token") || envValue(cmd.Env, "GH_TOKEN") != "" ||
		envValue(cmd.Env, "GITHUB_TOKEN") != "" {
		t.Fatalf("ambient authentication reached managed git: output=%q env=%v", got, cmd.Env)
	}
}

func TestEnsureWorkspaceClonedRequiresExplicitGitHubCredential(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
	_, err := cloner.EnsureWorkspaceCloned(
		context.Background(), "workspace-a", "github", "https://github.com/acme/private.git", "acme", "private",
	)
	if !errors.Is(err, ErrWorkspaceCredentialUnavailable) {
		t.Fatalf("EnsureWorkspaceCloned() error = %v, want credential unavailable", err)
	}
}

func TestWorkspaceCloneAuthPreservesNonGitHubURL(t *testing.T) {
	t.Parallel()

	cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolSSH, "", logger.Default())
	want := "git@ssh.dev.azure.com:v3/acme/Platform/api"
	got, auth, err := cloner.workspaceCloneAuth(
		context.Background(), "workspace-a", "azure_devops", want, "Platform", "api",
	)
	if err != nil {
		t.Fatalf("workspaceCloneAuth() unexpected error: %v", err)
	}
	if got != want || auth != nil {
		t.Fatalf("workspaceCloneAuth() = (%q, %#v), want (%q, nil)", got, auth, want)
	}
}

type recordingCredentialProvider struct {
	workspaceID string
	password    string
}

func (p *recordingCredentialProvider) ResolveGitCredential(
	_ context.Context,
	workspaceID, _, _, _ string,
) (string, string, error) {
	p.workspaceID = workspaceID
	return "x-access-token", p.password, nil
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func assertUniqueGitConfigEnv(t *testing.T, env []string) {
	t.Helper()
	counts := make(map[string]int)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if key == "GIT_CONFIG_COUNT" || strings.HasPrefix(key, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			counts[key]++
		}
	}
	for key, count := range counts {
		if count != 1 {
			t.Fatalf("%s occurs %d times in git environment", key, count)
		}
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
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
