package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const windowsGeneratedBranchMarker = "ensure session feature branch is checked out"

func resolveWindowsPrepareForTest(t *testing.T, metadata map[string]interface{}) string {
	t.Helper()
	base := map[string]interface{}{
		"repository_clone_url":    "https://github.com/acme/widget.git",
		MetadataKeyBaseBranch:     "main",
		"repository_setup_script": "Write-Output setup-marker",
	}
	for key, value := range metadata {
		base[key] = value
	}
	r := NewSSHExecutor(nil, nil, nil, newTestLogger())
	prepare, err := r.resolveWindowsPrepareScript(&ExecutorCreateRequest{Metadata: base}, `C:\k\tasks\task-1`)
	if err != nil {
		t.Fatalf("resolve Windows prepare: %v", err)
	}
	return prepare
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.4
func TestSSHWindowsPrepareMaterializesSelectedPullRequest(t *testing.T) {
	prepare := resolveWindowsPrepareForTest(t, map[string]interface{}{
		metadataCheckoutBranch: "contributor-fix",
		metadataCheckoutRef:    "refs/pull/7/head",
	})
	for _, want := range []string{
		"$__kCheckoutBranch = 'contributor-fix'",
		"$__kCheckoutRef = 'refs/pull/7/head'",
		"':refs/kandev/selected-checkout'",
		"kandev: selected checkout conflicts with an existing local branch",
	} {
		if !strings.Contains(prepare, want) {
			t.Fatalf("Windows prepare missing %q:\n%s", want, prepare)
		}
	}
	if strings.Contains(prepare, windowsGeneratedBranchMarker) {
		t.Fatal("an explicit selection must not fall back to the generated-branch postlude")
	}
	materialize := strings.Index(prepare, "materialize explicit checkout")
	scrub := strings.Index(prepare, "Env:GITHUB_TOKEN")
	setup := strings.Index(prepare, "Write-Output setup-marker")
	if materialize < 0 || scrub < materialize || setup < scrub {
		t.Fatalf("want checkout, then credential scrub, then repository setup; got indexes %d, %d, %d", materialize, scrub, setup)
	}
}

// The scrub can be the last statement of a prepare script when the repository
// has no setup script. The loader then reads its $? as the script status, and
// Remove-Item -ErrorAction SilentlyContinue leaves $? false for an absent
// variable, so the scrub must clear variables with a statement that succeeds.
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.4
func TestSSHWindowsPullRequestScrubSucceedsForAbsentVariables(t *testing.T) {
	prepare := resolveWindowsPrepareForTest(t, map[string]interface{}{
		metadataCheckoutBranch:    "contributor-fix",
		metadataCheckoutRef:       "refs/pull/7/head",
		"repository_setup_script": "",
	})
	if strings.Contains(prepare, "Remove-Item -LiteralPath 'Env:") {
		t.Fatalf("the credential scrub must not use Remove-Item, which leaves $? false for an absent variable:\n%s", prepare)
	}
	scrub := windowsSelectedCheckoutCredentialScrub(map[string]interface{}{metadataCheckoutRef: "refs/pull/7/head"})
	if !strings.HasPrefix(scrub, "Set-Item -LiteralPath ") || !strings.HasSuffix(scrub, " -Value $null") {
		t.Fatalf("scrub = %q, want one Set-Item clearing every variable", scrub)
	}
	for _, name := range strings.Fields(strings.TrimPrefix(selectedCheckoutCredentialScrubCommands, "unset ")) {
		if !strings.Contains(scrub, powerShellQuote("Env:"+name)) {
			t.Fatalf("scrub does not clear %s: %q", name, scrub)
		}
	}
	if !strings.Contains(prepare, scrub) {
		t.Fatalf("a selected pull request without a setup script still scrubs credentials:\n%s", prepare)
	}
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.1
func TestSSHWindowsPrepareMaterializesSelectedBranchStrictly(t *testing.T) {
	prepare := resolveWindowsPrepareForTest(t, map[string]interface{}{
		metadataCheckoutBranch: "release/2.0",
		metadataCheckoutRef:    "refs/heads/release/2.0",
	})
	if !strings.Contains(prepare, "$__kCheckoutRef = 'refs/heads/release/2.0'") || strings.Contains(prepare, windowsGeneratedBranchMarker) {
		t.Fatalf("a selected branch must use the strict checkout:\n%s", prepare)
	}
	if strings.Contains(prepare, "Env:GITHUB_TOKEN") {
		t.Fatal("a selected branch of the base repository keeps its credentials")
	}
}

func TestSSHWindowsPrepareKeepsGeneratedBranchWithoutSelection(t *testing.T) {
	prepare := resolveWindowsPrepareForTest(t, map[string]interface{}{MetadataKeyWorktreeBranch: "feature/task"})
	if !strings.Contains(prepare, windowsGeneratedBranchMarker) || strings.Contains(prepare, "materialize explicit checkout") {
		t.Fatalf("no selection keeps the generated-branch postlude:\n%s", prepare)
	}
}

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-002.4
func TestSSHWindowsExecPreparesSelectedPullRequest(t *testing.T) {
	requireWindowsExecution(t)
	client := newLocalWindowsSSHServer(t).dial(t)
	root := filepath.Join(t.TempDir(), "kandev pr root")
	originURL := newWindowsGitOrigin(t, root)
	seed := filepath.Join(root, "seed")
	runWindowsTestGit(t, "-C", seed, "checkout", "-q", "-b", "fork-work")
	if err := os.WriteFile(filepath.Join(seed, "pr.txt"), []byte("pull request\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runWindowsTestGit(t, "-C", seed, "add", "pr.txt")
	runWindowsTestGit(t, "-C", seed, "commit", "-q", "-m", "pull request change")
	prHead := runWindowsTestGit(t, "-C", seed, "rev-parse", "HEAD")
	runWindowsTestGit(t, "-C", seed, "push", "-q", filepath.Join(root, "origin.git"), "HEAD:refs/pull/7/head")

	r := NewSSHExecutor(nil, nil, nil, newTestLogger())
	windows := SSHRemotePlatform{GOOS: "windows", GOARCH: "amd64"}
	const recordSetupHead = "Set-Content -LiteralPath (Join-Path (Get-Location) 'setup-head.txt') -Value (& git rev-parse HEAD)"
	request := func(shell, ref, setup string) *ExecutorCreateRequest {
		return &ExecutorCreateRequest{Metadata: map[string]interface{}{
			MetadataKeySSHShell: shell, "repository_clone_url": originURL, MetadataKeyBaseBranch: "main",
			metadataCheckoutBranch: "pr-7", metadataCheckoutRef: ref, "repository_setup_script": setup,
		}}
	}
	for _, shell := range windowsTestShells() {
		taskDir := filepath.Join(root, "tasks", "task-pr-"+shell)
		if err := os.MkdirAll(taskDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := r.runPrepareScript(context.Background(), client, taskDir, request(shell, "refs/pull/7/head", recordSetupHead), windows, ""); err != nil {
			t.Fatalf("[%s] prepare selected PR: %v", shell, err)
		}
		if branch := runWindowsTestGit(t, "-C", taskDir, "branch", "--show-current"); branch != "pr-7" {
			t.Fatalf("[%s] branch = %q, want pr-7", shell, branch)
		}
		if head := runWindowsTestGit(t, "-C", taskDir, "rev-parse", "HEAD"); head != prHead {
			t.Fatalf("[%s] HEAD = %q, want the pull request head %q", shell, head, prHead)
		}
		setupHead, err := os.ReadFile(filepath.Join(taskDir, "setup-head.txt"))
		if err != nil || strings.TrimSpace(string(setupHead)) != prHead {
			t.Fatalf("[%s] repository setup saw HEAD %q (%v), want the selected head", shell, setupHead, err)
		}

		noSetupDir := filepath.Join(root, "tasks", "task-pr-no-setup-"+shell)
		if err := os.MkdirAll(noSetupDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := r.runPrepareScript(context.Background(), client, noSetupDir, request(shell, "refs/pull/7/head", ""), windows, ""); err != nil {
			t.Fatalf("[%s] prepare selected PR without a setup script: %v", shell, err)
		}
		if head := runWindowsTestGit(t, "-C", noSetupDir, "rev-parse", "HEAD"); head != prHead {
			t.Fatalf("[%s] without a setup script HEAD = %q, want %q", shell, head, prHead)
		}

		missingDir := filepath.Join(root, "tasks", "task-missing-"+shell)
		if err := os.MkdirAll(missingDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := r.runPrepareScript(context.Background(), client, missingDir, request(shell, "refs/pull/99/head", recordSetupHead), windows, ""); err == nil {
			t.Fatalf("[%s] a missing pull request ref must fail preparation", shell)
		}
	}
}

// A Windows prepare script receives the same upstream branch name as a POSIX
// one, so an origin-qualified base branch clones the upstream branch.
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.1
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.4
func TestSSHWindowsRepositoryProviderNormalizesOriginBranch(t *testing.T) {
	cases := []struct{ input, want string }{
		{"main", "main"},
		{"feature/login", "feature/login"},
		{"origin/main", "main"},
		{"origin/feature/login", "feature/login"},
		{"refs/remotes/origin/main", "main"},
		{"refs/heads/main", "main"},
		{"upstream/main", "upstream/main"},
		{"refs/heads/origin/topic", "origin/topic"},
		{"origin/origin/topic", "origin/topic"},
		{"origin/O’Brien", "O’Brien"},
	}
	for _, tc := range cases {
		for _, key := range []string{MetadataKeyBaseBranch, "repository_branch"} {
			t.Run(key+"/"+tc.input, func(t *testing.T) {
				got := windowsRepositoryProvider(map[string]interface{}{key: tc.input})()["repository.branch"]
				if want := powerShellQuote(tc.want); got != want {
					t.Fatalf("repository.branch = %q, want %q", got, want)
				}
			})
		}
	}
}
