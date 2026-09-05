package lifecycle

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestKandevBranchCheckoutPostlude_HasInvariantSteps asserts the kandev-
// managed postlude contains the steps needed to land on the session's
// feature branch. The postlude is appended to every user prepare script so
// stale stored scripts (created before the worktree-branch checkout was
// part of the default) still get the checkout.
func TestKandevBranchCheckoutPostlude_HasInvariantSteps(t *testing.T) {
	postlude := KandevBranchCheckoutPostlude()
	// Data placeholders are referenced BARE; the scriptengine providers
	// substitute a self-contained single-quoted token (shellQuote), so a
	// hostile branch name resolves to a quoted literal. The placeholders must
	// NOT be double-quoted here (double quotes would re-expose $(...)).
	want := []string{
		`if [ -d {{workspace.path}}/.git ]`,
		`[ -n {{worktree.branch}} ]`,
		`[ {{worktree.branch}} != {{repository.branch}} ]`,
		`cd {{workspace.path}}`,
		`git rev-parse --verify {{worktree.branch}}`,
		`git fetch --no-tags origin {{worktree.branch}}`,
		`git checkout -b {{worktree.branch}} origin/{{worktree.branch}}`,
		`git checkout -b {{worktree.branch}}`,
		`|| true`,
	}
	for _, w := range want {
		if !strings.Contains(postlude, w) {
			t.Errorf("postlude missing %q", w)
		}
	}
	// Data placeholders must never be double-quoted: double quotes do not stop
	// $(...) / backtick command substitution, which is the RCE we fixed.
	if strings.Contains(postlude, `"{{worktree.branch}}"`) ||
		strings.Contains(postlude, `"{{repository.branch}}"`) ||
		strings.Contains(postlude, `"{{workspace.path}}`) {
		t.Errorf("postlude must not double-quote data placeholders:\n%s", postlude)
	}
	// A depth-limited fetch grafts a history-less tip into the workspace, which
	// leaves the session branch with no merge-base against the base branch and
	// breaks the diff panel and the pull-request flow. Nothing unshallows a
	// workspace afterwards, so the graft is permanent — verify it stays gone.
	if strings.Contains(shellCodeOnly(postlude), "--depth") {
		t.Errorf("postlude must not fetch with --depth (it permanently grafts a shallow tip):\n%s", postlude)
	}
	// The destructive `-B branch origin/branch` form orphaned local commits on
	// resume — verify it does NOT come back.
	forbidden := []string{
		`git checkout -B {{worktree.branch}} origin/{{worktree.branch}}`,
	}
	for _, f := range forbidden {
		if strings.Contains(postlude, f) {
			t.Errorf("postlude must not contain destructive form %q", f)
		}
	}
}

// TestKandevBranchCheckoutPostlude_LandsOnFeatureBranch executes the
// postlude as a real shell script against a temp git repo. It is the
// behaviour test that pairs with the static-content test above:
// it catches regressions where the snippet still parses as bash but
// no longer does the right thing (wrong refspec, missing -B, swallowed
// exit code, etc.).
//
// The three cases mirror the three branches inside the postlude:
//   - remote feature branch exists                → tracks origin/<branch>
//   - no remote, no local                         → creates local branch off HEAD
//   - local branch already checked out, no remote → idempotent (no error, same branch)
func TestKandevBranchCheckoutPostlude_LandsOnFeatureBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	cases := []struct {
		name        string
		seedRemote  bool   // create a "feature" branch on origin before running
		seedLocal   bool   // pre-create the local branch (idempotency check)
		featureName string // branch name passed to the postlude
		baseName    string // base branch name (must match repository.branch)
		want        string // expected current branch after running
	}{
		{
			name:        "remote tip exists",
			seedRemote:  true,
			featureName: "feature/from-remote",
			baseName:    "main",
			want:        "feature/from-remote",
		},
		{
			name:        "no remote, no local",
			seedRemote:  false,
			featureName: "feature/from-scratch",
			baseName:    "main",
			want:        "feature/from-scratch",
		},
		{
			name:        "local already checked out",
			seedLocal:   true,
			featureName: "feature/already-here",
			baseName:    "main",
			want:        "feature/already-here",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace, originDir := setupPostludeRepo(t, tc.baseName)
			if tc.seedRemote {
				seedOriginBranch(t, originDir, tc.featureName)
			}
			if tc.seedLocal {
				runIn(t, workspace, "git", "checkout", "-b", tc.featureName)
			}

			script := strings.NewReplacer(
				"{{workspace.path}}", workspace,
				"{{worktree.branch}}", tc.featureName,
				"{{repository.branch}}", tc.baseName,
			).Replace(KandevBranchCheckoutPostlude())

			cmd := exec.Command("bash", "-e", "-c", script)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("bash -e postlude failed: %v\n%s", err, out)
			}

			gotBranch := strings.TrimSpace(string(runIn(t, workspace, "git", "branch", "--show-current")))
			if gotBranch != tc.want {
				t.Fatalf("after postlude branch = %q, want %q\nscript output:\n%s", gotBranch, tc.want, out)
			}
		})
	}
}

// setupPostludeRepo creates a fake origin (bare repo with one commit on
// baseBranch) and a workspace cloned from it, then leaves the workspace on
// baseBranch. Returns workspace path + origin (bare) path.
func setupPostludeRepo(t *testing.T, baseBranch string) (workspace, origin string) {
	t.Helper()
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	workspace = filepath.Join(root, "workspace")

	runIn(t, root, "git", "init", "--quiet", "--bare", "--initial-branch="+baseBranch, origin)
	runIn(t, root, "git", "init", "--quiet", "--initial-branch="+baseBranch, seed)
	runIn(t, seed, "git", "config", "user.email", "test@example.com")
	runIn(t, seed, "git", "config", "user.name", "Test")
	runIn(t, seed, "git", "commit", "--allow-empty", "-m", "init")
	runIn(t, seed, "git", "remote", "add", "origin", origin)
	runIn(t, seed, "git", "push", "--quiet", "origin", baseBranch)

	runIn(t, root, "git", "clone", "--quiet", "--branch", baseBranch, origin, workspace)
	runIn(t, workspace, "git", "config", "user.email", "test@example.com")
	runIn(t, workspace, "git", "config", "user.name", "Test")
	return workspace, origin
}

// seedOriginBranch creates `branch` on `origin` (the bare repo) so the
// postlude's `git fetch origin <branch>` succeeds. Uses a temporary clone of
// origin to push the new branch in.
func seedOriginBranch(t *testing.T, origin, branch string) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "seed-origin-branch")
	runIn(t, "", "git", "clone", "--quiet", origin, tmp)
	runIn(t, tmp, "git", "config", "user.email", "test@example.com")
	runIn(t, tmp, "git", "config", "user.name", "Test")
	runIn(t, tmp, "git", "checkout", "-b", branch)
	runIn(t, tmp, "git", "commit", "--allow-empty", "-m", "feature")
	runIn(t, tmp, "git", "push", "--quiet", "origin", branch)
}

// runIn runs cmd with args in dir (or current dir when dir == "") and fails
// the test on non-zero exit. Returns stdout for callers that need it.
func runIn(t *testing.T, dir string, cmd string, args ...string) []byte {
	t.Helper()
	c := exec.Command(cmd, args...)
	if dir != "" {
		c.Dir = dir
	}
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", cmd, args, err, out)
	}
	return out
}

// TestDefaultPrepareScripts_NoInlineFeatureBranchCheckout asserts that the
// clone-based remote default scripts no longer carry an inline worktree-
// branch checkout. The checkout is owned exclusively by the postlude
// (KandevBranchCheckoutPostlude) so old stored profiles and the current
// default can never disagree about how the feature branch is materialised.
func TestDefaultPrepareScripts_NoInlineFeatureBranchCheckout(t *testing.T) {
	executors := []string{"local_docker", "remote_docker", "sprites"}
	forbidden := []string{
		`if [ -n {{worktree.branch}} ] && [ {{worktree.branch}} != {{repository.branch}} ]; then`,
		`git checkout -B {{worktree.branch}} origin/{{worktree.branch}}`,
	}

	for _, executorType := range executors {
		t.Run(executorType, func(t *testing.T) {
			script := DefaultPrepareScript(executorType)
			if script == "" {
				t.Fatalf("DefaultPrepareScript(%q) returned empty", executorType)
			}
			for _, bad := range forbidden {
				if strings.Contains(script, bad) {
					t.Errorf("script for %q must not contain inline checkout %q (postlude owns it)", executorType, bad)
				}
			}
		})
	}
}

func TestDefaultPrepareScript_SSHMaterializesPrimaryWorkspace(t *testing.T) {
	script := DefaultPrepareScript(executorTypeSSH)
	if script == "" {
		t.Fatal("DefaultPrepareScript(\"ssh\") returned empty")
	}
	for _, want := range []string{
		"{{workspace.path}}",
		"{{repository.clone_url}}",
		"{{repository.branch}}",
		"{{repository.setup_script}}",
		"git init",
		"fetch --no-tags",
		"remote.origin.url",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("SSH default prepare script missing %q", want)
		}
	}
	if strings.Contains(script, "git checkout -B {{worktree.branch}} origin/{{worktree.branch}}") {
		t.Fatal("SSH default prepare script must leave feature-branch selection to the postlude")
	}
}

// TestKandevBranchCheckoutPostlude_PreservesHistoryForRemoteBranch is the
// regression for the shallow graft. When the session branch already exists on
// origin (resume, re-run, or a fresh sandbox provisioned for an existing task)
// the postlude fetches it. A depth-limited fetch there wrote .git/shallow even
// into a full clone, after which the session branch had no common ancestor
// with the base branch: "git merge-base" fails and "git diff <base>...HEAD" is
// meaningless. Those are exactly what the diff panel and the pull-request flow
// run, and nothing in the backend ever unshallows a workspace afterwards.
func TestKandevBranchCheckoutPostlude_PreservesHistoryForRemoteBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	const (
		baseBranch    = "main"
		featureBranch = "feature/from-remote"
	)

	workspace, originDir := setupPostludeRepo(t, baseBranch)
	seedOriginBranch(t, originDir, featureBranch)

	script := strings.NewReplacer(
		"{{workspace.path}}", workspace,
		"{{worktree.branch}}", featureBranch,
		"{{repository.branch}}", baseBranch,
	).Replace(KandevBranchCheckoutPostlude())

	if out, err := exec.Command("bash", "-e", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("bash -e postlude failed: %v\n%s", err, out)
	}

	shallow := strings.TrimSpace(string(runIn(t, workspace, "git", "rev-parse", "--is-shallow-repository")))
	if shallow != "false" {
		t.Errorf("workspace is shallow after the postlude (--is-shallow-repository = %q)", shallow)
	}

	mergeBase := exec.Command("git", "merge-base", baseBranch, "HEAD")
	mergeBase.Dir = workspace
	out, err := mergeBase.CombinedOutput()
	if err != nil {
		t.Fatalf("git merge-base %s HEAD failed after the postlude: %v\n%s", baseBranch, err, out)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatalf("git merge-base %s HEAD returned no commit", baseBranch)
	}
}

// TestDefaultPrepareScript_SpritesMaterializesWithoutTruncatingHistory pins the
// properties the Sprites default regressed on: it must not clone into a
// directory kandev may already have written into, it must not truncate the
// history the pull-request flow needs, and it must never print the clone URL,
// which carries an injected access token, into the streamed prepare output.
func TestDefaultPrepareScript_SpritesMaterializesWithoutTruncatingHistory(t *testing.T) {
	script := DefaultPrepareScript("sprites")
	if script == "" {
		t.Fatal(`DefaultPrepareScript("sprites") returned empty`)
	}

	for _, want := range []string{
		"{{workspace.path}}",
		"{{repository.clone_url}}",
		"{{repository.branch}}",
		"{{repository.setup_script}}",
		"{{kandev.agentctl.install}}",
		"{{kandev.agentctl.start}}",
		"git init",
		"fetch --filter=blob:none --no-tags",
		"remote.origin.url",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("Sprites default prepare script missing %q", want)
		}
	}

	// Each of these shipped in the default and each broke a real launch path.
	forbidden := map[string]string{
		"--depth":            "truncated history breaks merge-base and pull-request diffs",
		"git clone":          "the workspace is not guaranteed to be empty when prepare runs",
		"ssh-keyscan":        "all transports are HTTPS + token; a DNS blip killed the launch under set -e",
		"printf 'Cloning %s": "leaked the token-bearing clone URL into the streamed task output",
		"pnpm-linux-x64":     "a hardcoded version ignores each repository's packageManager pin",
	}
	code := shellCodeOnly(script)
	for bad, why := range forbidden {
		if strings.Contains(code, bad) {
			t.Errorf("Sprites default prepare script must not contain %q: %s", bad, why)
		}
	}
}

// shellCodeOnly drops whole-line "#" comments so a forbidden-substring check
// tests what the script RUNS rather than what its comments say about it. The
// comments deliberately name the constructs that were removed (--depth,
// git clone, ssh-keyscan), and matching those would make the check assert the
// opposite of its intent.
func shellCodeOnly(script string) string {
	lines := strings.Split(script, "\n")
	code := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		code = append(code, line)
	}
	return strings.Join(code, "\n")
}
