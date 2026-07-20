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
	writeExecutable(t, filepath.Join(scriptDir, "az"), fmt.Sprintf(`#!/bin/sh
if [ "$1" = "extension" ] && [ "$2" = "show" ]; then
  exit 0
fi
printf '%%s\n' "$@" > %q
printf 'WARNING: preview command group\n' >&2
printf 'Creating pull request...\n'
cat <<'EOF'
{"pullRequestId":42,"repository":{"remoteUrl":"https://dev.azure.com/acme/platform/_git/widgets"}}
EOF
`, azArgsPath))
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

func writeGitRemoteWrapper(t *testing.T, scriptDir, originURL string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("find git: %v", err)
	}
	writeExecutable(
		t,
		filepath.Join(scriptDir, "git"),
		fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"remote\" ] && [ \"$2\" = \"get-url\" ] && [ \"$3\" = \"origin\" ]; then\n  printf '%%s\\n' %q\n  exit 0\nfi\nexec %q \"$@\"\n", originURL, realGit),
	)
}

func TestGitOperatorCreatePR_GitLabUsesGLab(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	runGit(t, repoDir, "checkout", "-b", "feature/gitlab-pr")
	writeFile(t, repoDir, "gitlab.txt", "gitlab\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add gitlab change")

	scriptDir := t.TempDir()
	writeGitRemoteWrapper(t, scriptDir, "git@gitlab.com:acme/widgets.git")
	writeExecutable(t, filepath.Join(scriptDir, "glab"), `#!/bin/sh
if [ "$1" = "mr" ] && [ "$2" = "list" ]; then
  printf '[]\n'
  exit 0
fi
printf 'https://gitlab.com/acme/widgets/-/merge_requests/42\n'
`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.CreatePR(context.Background(), "Title", "Body", "main", false)
	if err != nil {
		t.Fatalf("CreatePR returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("CreatePR should succeed for GitLab remote: %+v", result)
	}
	if result.PRURL != "https://gitlab.com/acme/widgets/-/merge_requests/42" {
		t.Fatalf("PRURL = %q", result.PRURL)
	}
	if result.Provider != "gitlab" {
		t.Fatalf("Provider = %q, want gitlab", result.Provider)
	}
}

func TestGitOperatorCreatePR_AzureMissingCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	runGit(t, repoDir, "checkout", "-b", "feature/azure-missing-cli")
	writeFile(t, repoDir, "azure.txt", "azure\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add azure change")

	scriptDir := t.TempDir()
	writeGitRemoteWrapper(t, scriptDir, "git@ssh.dev.azure.com:v3/acme/platform/widgets")
	// Isolate PATH so CI's preinstalled az is not visible to LookPath.
	t.Setenv("PATH", scriptDir)

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.CreatePR(context.Background(), "Title", "Body", "main", false)
	if err != nil {
		t.Fatalf("CreatePR returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("CreatePR should fail without az: %+v", result)
	}
	if !result.BranchPushed || result.Error != "branch was pushed; retry pull request creation" {
		t.Fatalf("partial result = %+v", result)
	}
}

func TestGitOperatorCreatePR_AzureMissingExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}

	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	runGit(t, repoDir, "checkout", "-b", "feature/azure-missing-ext")
	writeFile(t, repoDir, "azure.txt", "azure\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add azure change")

	scriptDir := t.TempDir()
	writeGitRemoteWrapper(t, scriptDir, "git@ssh.dev.azure.com:v3/acme/platform/widgets")
	writeExecutable(t, filepath.Join(scriptDir, "az"), "#!/bin/sh\nif [ \"$1\" = \"extension\" ] && [ \"$2\" = \"show\" ]; then exit 1; fi\nexit 0\n")
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.CreatePR(context.Background(), "Title", "Body", "main", false)
	if err != nil {
		t.Fatalf("CreatePR returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("CreatePR should fail without azure-devops extension: %+v", result)
	}
	if !result.BranchPushed || result.Error != "branch was pushed; retry pull request creation" {
		t.Fatalf("partial result = %+v", result)
	}
}

func TestEnsureAzureDevOpsCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell wrapper test is Unix-only")
	}

	scriptDir := t.TempDir()
	writeExecutable(t, filepath.Join(scriptDir, "az"), "#!/bin/sh\nif [ \"$1\" = \"extension\" ] && [ \"$2\" = \"show\" ]; then exit 0; fi\nexit 1\n")
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := ensureAzureDevOpsCLI(context.Background()); err != nil {
		t.Fatalf("ensureAzureDevOpsCLI() error = %v", err)
	}

	t.Setenv("PATH", "")
	if err := ensureAzureDevOpsCLI(context.Background()); err == nil {
		t.Fatal("expected error when az is missing")
	} else if err.Error() != errAzureCLIMissing {
		t.Fatalf("error = %q, want %q", err.Error(), errAzureCLIMissing)
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
		{name: "gitlab https", remote: "https://gitlab.com/acme/widgets.git", want: prProviderGitLab},
		{name: "gitlab ssh", remote: "git@gitlab.com:acme/widgets.git", want: prProviderGitLab},
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

func TestDetectPRProviderSelfManagedGitLabRequiresMatchingConfiguredHost(t *testing.T) {
	t.Setenv("KANDEV_GITLAB_HOST", "http://gitlab.internal:8080")
	if got := detectPRProvider("git@gitlab.internal:acme/widgets.git"); got != prProviderGitLab {
		t.Fatalf("matching self-managed provider = %q, want gitlab", got)
	}
	if got := detectPRProvider("git@other.internal:acme/widgets.git"); got == prProviderGitLab {
		t.Fatalf("mismatched provider = %q, must not be gitlab", got)
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

// fileEntryDiff returns the diff string and skip reason for a file entry from a
// CumulativeDiffResult/CommitDiffResult Files map.
func fileEntryDiff(t *testing.T, files map[string]interface{}, path string) (diff, skipReason string) {
	t.Helper()
	raw, ok := files[path]
	if !ok {
		t.Fatalf("Files missing key %q; keys=%v", path, fileKeys(files))
	}
	entry, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("Files[%q] is %T, want map[string]interface{}", path, raw)
	}
	diff, _ = entry["diff"].(string)
	skipReason, _ = entry["diff_skip_reason"].(string)
	return diff, skipReason
}

func fileKeys(files map[string]interface{}) []string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	return keys
}

// TestGetCumulativeDiff_TruncatesLargeFile verifies a single file whose diff
// exceeds maxDiffOutputSize is truncated and flagged.
func TestGetCumulativeDiff_TruncatesLargeFile(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// README.md is already tracked; overwrite it with >256 KB of new content so
	// `git diff <base>` produces a single large per-file diff.
	large := strings.Repeat("an additional line of content for the big diff\n", 8000) // ~376 KB
	writeFile(t, repoDir, "README.md", large)

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.GetCumulativeDiff(context.Background(), base)
	if err != nil {
		t.Fatalf("GetCumulativeDiff error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetCumulativeDiff failed: %+v", result)
	}

	diff, skipReason := fileEntryDiff(t, result.Files, "README.md")
	if skipReason != diffSkipReasonTruncated {
		t.Errorf("diff_skip_reason = %q, want %q", skipReason, diffSkipReasonTruncated)
	}
	if len(diff) > maxDiffOutputSize {
		t.Errorf("truncated diff len = %d, want <= %d", len(diff), maxDiffOutputSize)
	}
}

// TestGetCumulativeDiff_BudgetExceeded verifies that once the cumulative diff
// budget is crossed, later files get an empty diff flagged budget_exceeded.
func TestGetCumulativeDiff_BudgetExceeded(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// 20 files of ~200 KB each (~4 MB total) so the 2 MB budget trips partway.
	body := strings.Repeat("line of content for the budget test\n", 5700) // ~200 KB
	for i := 0; i < 20; i++ {
		writeFile(t, repoDir, fmt.Sprintf("file_%02d.txt", i), body)
	}
	runGit(t, repoDir, "add", ".")

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.GetCumulativeDiff(context.Background(), base)
	if err != nil {
		t.Fatalf("GetCumulativeDiff error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetCumulativeDiff failed: %+v", result)
	}

	budgetExceeded := 0
	for path := range result.Files {
		diff, skipReason := fileEntryDiff(t, result.Files, path)
		if skipReason == diffSkipReasonBudgetExceeded {
			budgetExceeded++
			if diff != "" {
				t.Errorf("budget_exceeded file %q has non-empty diff (len %d)", path, len(diff))
			}
		}
	}
	if budgetExceeded == 0 {
		t.Errorf("expected at least one budget_exceeded file, got none across %d files", len(result.Files))
	}
}

// TestGetCumulativeDiff_CapsFileCount verifies the file-count cap drops excess
// entries and reports the count.
func TestGetCumulativeDiff_CapsFileCount(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	const totalFiles = maxCumulativeDiffFiles + 100
	for i := 0; i < totalFiles; i++ {
		writeFile(t, repoDir, fmt.Sprintf("f%04d.txt", i), "small change\n")
	}
	runGit(t, repoDir, "add", ".")

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.GetCumulativeDiff(context.Background(), base)
	if err != nil {
		t.Fatalf("GetCumulativeDiff error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetCumulativeDiff failed: %+v", result)
	}

	if len(result.Files) != maxCumulativeDiffFiles {
		t.Errorf("len(Files) = %d, want %d", len(result.Files), maxCumulativeDiffFiles)
	}
	if result.TruncatedFilesCount != totalFiles-maxCumulativeDiffFiles {
		t.Errorf("TruncatedFilesCount = %d, want %d", result.TruncatedFilesCount, totalFiles-maxCumulativeDiffFiles)
	}
}

// TestShowCommit_NotCapped is a regression test: ShowCommit must return the full
// diff for a large file (no per-file truncation), unlike GetCumulativeDiff.
func TestShowCommit_NotCapped(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	defer cleanup()

	log := newTestLogger(t)

	large := strings.Repeat("an additional line of content for the big diff\n", 8000) // ~376 KB
	writeFile(t, repoDir, "big.txt", large)
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add big file")
	sha := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	gitOp := NewGitOperator(repoDir, log, nil)
	result, err := gitOp.ShowCommit(context.Background(), sha)
	if err != nil {
		t.Fatalf("ShowCommit error: %v", err)
	}
	if !result.Success {
		t.Fatalf("ShowCommit failed: %+v", result)
	}

	diff, skipReason := fileEntryDiff(t, result.Files, "big.txt")
	if skipReason != "" {
		t.Errorf("ShowCommit diff_skip_reason = %q, want empty (uncapped)", skipReason)
	}
	if len(diff) <= maxDiffOutputSize {
		t.Errorf("ShowCommit diff len = %d, want > %d (full, uncapped)", len(diff), maxDiffOutputSize)
	}
}
