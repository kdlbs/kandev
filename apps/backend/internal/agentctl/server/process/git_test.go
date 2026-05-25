package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGitOperatorPush_PreservesExistingUpstream(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)

	runGit(t, repoDir, "checkout", "-b", "feature/pr-branch")
	writeFile(t, repoDir, "feature.txt", "feature branch\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "feature commit")
	runGit(t, repoDir, "push", "-u", "origin", "feature/pr-branch")

	runGit(t, repoDir, "checkout", "main")
	worktreeBase := t.TempDir()
	directDir := filepath.Join(worktreeBase, "wt-direct")
	suffixedDir := filepath.Join(worktreeBase, "wt-sfx")
	runGit(t, repoDir, "worktree", "add", directDir, "feature/pr-branch")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/pr-branch-sfx", suffixedDir, "origin/feature/pr-branch")

	runGit(t, suffixedDir, "branch", "--set-upstream-to=origin/feature/pr-branch", "feature/pr-branch-sfx")
	writeFile(t, suffixedDir, "feature.txt", "feature branch\nlocal change\n")
	runGit(t, suffixedDir, "add", ".")
	runGit(t, suffixedDir, "commit", "-m", "local change")

	before := strings.TrimSpace(runGit(t, suffixedDir, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if before != "origin/feature/pr-branch" {
		t.Fatalf("upstream before push = %q, want %q", before, "origin/feature/pr-branch")
	}

	gitOp := NewGitOperator(suffixedDir, log, nil)
	result, err := gitOp.Push(context.Background(), false, false)
	if err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Push failed: %+v", result)
	}

	after := strings.TrimSpace(runGit(t, suffixedDir, "rev-parse", "--abbrev-ref", "@{upstream}"))
	if after != "origin/feature/pr-branch" {
		t.Fatalf("upstream after push = %q, want %q", after, "origin/feature/pr-branch")
	}
}

func TestParseCommitDiff_PathsWithSpaces(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)

	// Create a file with spaces in its path, commit it, then verify parseCommitDiff
	// extracts the correct unquoted file path.
	dir := filepath.Join(repoDir, "path with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	writeFile(t, dir, "file.md", "hello world\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add spaced path")
	sha := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.ShowCommit(context.Background(), sha)
	if err != nil {
		t.Fatalf("ShowCommit error: %v", err)
	}
	if !result.Success {
		t.Fatalf("ShowCommit failed: %+v", result)
	}

	expectedPath := "path with spaces/file.md"
	if _, exists := result.Files[expectedPath]; !exists {
		keys := make([]string, 0, len(result.Files))
		for k := range result.Files {
			keys = append(keys, k)
		}
		t.Errorf("expected Files to contain key %q, got keys: %v", expectedPath, keys)
	}
}

func TestGitOperatorCreatePR_UsesAzureCLIForAzureRepos(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	runGit(t, repoDir, "checkout", "-b", "feature/azure-pr")
	writeFile(t, repoDir, "azure.txt", "azure repos\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add azure change")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}

	scriptDir := t.TempDir()
	azArgsPath := filepath.Join(scriptDir, "az-args.txt")
	writeExecutable(t, filepath.Join(scriptDir, "git"), fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"remote\" ] && [ \"$2\" = \"get-url\" ] && [ \"$3\" = \"origin\" ]; then\n  printf '%s\\n' 'git@ssh.dev.azure.com:v3/acme/platform/widgets'\n  exit 0\nfi\nexec %q \"$@\"\n", "%s", realGit))
	writeExecutable(t, filepath.Join(scriptDir, "az"), fmt.Sprintf("#!/bin/sh\nprintf '%s\\n' \"$@\" > %q\nprintf '%s\\n' 'WARNING: preview command group' >&2\nprintf 'Creating pull request...\\n'\ncat <<'EOF'\n{\"pullRequestId\":42,\"repository\":{\"remoteUrl\":\"https://dev.azure.com/acme/platform/_git/widgets\"}}\nEOF\n", "%s", azArgsPath, "%s"))
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.CreatePR(context.Background(), "Add Azure support", "PR body", "origin/main", true)
	if err != nil {
		t.Fatalf("CreatePR returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("CreatePR failed: %+v", result)
	}

	if got, want := result.PRURL, "https://dev.azure.com/acme/platform/_git/widgets/pullrequest/42"; got != want {
		t.Fatalf("PRURL = %q, want %q", got, want)
	}

	gotArgs := readScriptArgs(t, azArgsPath)
	wantArgs := []string{
		"repos",
		"pr",
		"create",
		"--organization",
		"https://dev.azure.com/acme",
		"--project",
		"platform",
		"--repository",
		"widgets",
		"--source-branch",
		"feature/azure-pr",
		"--target-branch",
		"main",
		"--title",
		"Add Azure support",
		"--description",
		"PR body",
		"--draft",
		"true",
		"-o",
		"json",
	}
	if strings.Join(gotArgs, "\n") != strings.Join(wantArgs, "\n") {
		t.Fatalf("az args mismatch\n got: %q\nwant: %q", gotArgs, wantArgs)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func TestDetectPRProvider(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   prProvider
	}{
		{name: "azure https", remote: "https://dev.azure.com/acme/platform/_git/widgets", want: prProviderAzureRepos},
		{name: "azure ssh", remote: "git@ssh.dev.azure.com:v3/acme/platform/widgets", want: prProviderAzureRepos},
		{
			name:   "azure ssh url scheme",
			remote: "ssh://ssh.dev.azure.com:22/v3/acme/platform/widgets",
			want:   prProviderAzureRepos,
		},
		{name: "visualstudio", remote: "https://acme.visualstudio.com/platform/_git/widgets", want: prProviderAzureRepos},
		{name: "github", remote: "https://github.com/acme/widgets.git", want: prProviderGitHub},
		{
			name:   "github path must not match azure substring",
			remote: "git@github.com:acme/dev.azure.com-docs.git",
			want:   prProviderGitHub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectPRProvider(tt.remote); got != tt.want {
				t.Fatalf("detectPRProvider(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}

func TestRemoteHostFromURL(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{name: "https", remote: "https://dev.azure.com/acme/platform/_git/widgets", want: "dev.azure.com"},
		{name: "scp", remote: "git@ssh.dev.azure.com:v3/acme/platform/widgets", want: "ssh.dev.azure.com"},
		{
			name:   "ssh url scheme",
			remote: "ssh://ssh.dev.azure.com:22/v3/acme/platform/widgets",
			want:   "ssh.dev.azure.com",
		},
		{name: "github", remote: "https://github.com/acme/widgets.git", want: "github.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remoteHostFromURL(tt.remote); got != tt.want {
				t.Fatalf("remoteHostFromURL(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}

func TestParseAzurePRCreateResponse(t *testing.T) {
	payload := `{"pullRequestId":42,"repository":{"remoteUrl":"https://dev.azure.com/acme/platform/_git/widgets"}}`
	stdout := "Creating pull request...\n" + payload

	response, err := parseAzurePRCreateResponse(stdout)
	if err != nil {
		t.Fatalf("parseAzurePRCreateResponse() error = %v", err)
	}
	if response.PullRequestID != 42 {
		t.Fatalf("PullRequestID = %d, want 42", response.PullRequestID)
	}
}

func TestParseAzurePRCreateResponse_multilineJSON(t *testing.T) {
	stdout := "Creating pull request...\n{\n  \"pullRequestId\": 9,\n  \"repository\": {\n    \"remoteUrl\": \"https://dev.azure.com/acme/platform/_git/widgets\"\n  }\n}\n"

	response, err := parseAzurePRCreateResponse(stdout)
	if err != nil {
		t.Fatalf("parseAzurePRCreateResponse() error = %v", err)
	}
	if response.PullRequestID != 9 {
		t.Fatalf("PullRequestID = %d, want 9", response.PullRequestID)
	}
}

func TestParseAzurePRCreateResponse_bracesInString(t *testing.T) {
	// Braces inside JSON string values must not break line-based parsing.
	stdout := "status line\n" + `{"pullRequestId":7,"repository":{"remoteUrl":"https://dev.azure.com/o/p/_git/r","note":"} not end"}}`

	response, err := parseAzurePRCreateResponse(stdout)
	if err != nil {
		t.Fatalf("parseAzurePRCreateResponse() error = %v", err)
	}
	if response.PullRequestID != 7 {
		t.Fatalf("PullRequestID = %d, want 7", response.PullRequestID)
	}
}

func TestParseAzureRepoInfo(t *testing.T) {
	tests := []struct {
		name     string
		remote   string
		wantOrg  string
		wantProj string
		wantRepo string
	}{
		{
			name:     "dev azure https",
			remote:   "https://dev.azure.com/acme/platform/_git/widgets.git",
			wantOrg:  "https://dev.azure.com/acme",
			wantProj: "platform",
			wantRepo: "widgets",
		},
		{
			name:     "visualstudio https",
			remote:   "https://acme.visualstudio.com/platform/_git/widgets",
			wantOrg:  "https://acme.visualstudio.com",
			wantProj: "platform",
			wantRepo: "widgets",
		},
		{
			name:     "azure ssh",
			remote:   "git@ssh.dev.azure.com:v3/acme/platform/widgets",
			wantOrg:  "https://dev.azure.com/acme",
			wantProj: "platform",
			wantRepo: "widgets",
		},
		{
			name:     "azure ssh url scheme",
			remote:   "ssh://ssh.dev.azure.com:22/v3/acme/platform/widgets",
			wantOrg:  "https://dev.azure.com/acme",
			wantProj: "platform",
			wantRepo: "widgets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := parseAzureRepoInfo(tt.remote)
			if err != nil {
				t.Fatalf("parseAzureRepoInfo(%q) error = %v", tt.remote, err)
			}
			if info.OrganizationURL != tt.wantOrg || info.Project != tt.wantProj || info.Repository != tt.wantRepo {
				t.Fatalf("parseAzureRepoInfo(%q) = %+v, want org=%q project=%q repo=%q", tt.remote, info, tt.wantOrg, tt.wantProj, tt.wantRepo)
			}
		})
	}
}

func TestSanitizeRepositoryArgs(t *testing.T) {
	got := sanitizeRepositoryArgs([]string{
		"pr",
		"create",
		"--title",
		"real title",
		"--body=secret body",
		"--description",
		"secret description",
		"--head",
		"feature/branch",
	})

	want := []string{
		"pr",
		"create",
		"--title",
		"[REDACTED]",
		"--body=[REDACTED]",
		"--description",
		"[REDACTED]",
		"--head",
		"feature/branch",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("sanitizeRepositoryArgs() = %q, want %q", got, want)
	}
}

func TestRedactRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "https credentials",
			url:  "https://token:x-oauth-basic@github.com/acme/widgets.git",
			want: "https://github.com/acme/widgets.git",
		},
		{
			name: "scp style",
			url:  "git@ssh.dev.azure.com:v3/acme/platform/widgets",
			want: "ssh.dev.azure.com:v3/acme/platform/widgets",
		},
		{
			name: "plain https",
			url:  "https://dev.azure.com/acme/platform/_git/widgets",
			want: "https://dev.azure.com/acme/platform/_git/widgets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactRemoteURL(tt.url); got != tt.want {
				t.Fatalf("redactRemoteURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func readScriptArgs(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
