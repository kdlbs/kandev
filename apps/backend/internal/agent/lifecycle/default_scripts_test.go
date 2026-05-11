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
	want := []string{
		`if [ -d "{{workspace.path}}/.git" ]`,
		`[ -n "{{worktree.branch}}" ]`,
		`[ "{{worktree.branch}}" != "{{repository.branch}}" ]`,
		`cd "{{workspace.path}}"`,
		`git rev-parse --verify "{{worktree.branch}}"`,
		`git fetch --depth=1 origin "{{worktree.branch}}"`,
		`git checkout -b "{{worktree.branch}}" "origin/{{worktree.branch}}"`,
		`git checkout -b "{{worktree.branch}}"`,
		`|| true`,
	}
	for _, w := range want {
		if !strings.Contains(postlude, w) {
			t.Errorf("postlude missing %q", w)
		}
	}
	// The destructive `-B "branch" "origin/branch"` form orphaned local
	// commits on resume — verify it does NOT come back.
	forbidden := []string{
		`git checkout -B "{{worktree.branch}}" "origin/{{worktree.branch}}"`,
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
		`if [ -n "{{worktree.branch}}" ] && [ "{{worktree.branch}}" != "{{repository.branch}}" ]; then`,
		`git checkout -B "{{worktree.branch}}" "origin/{{worktree.branch}}"`,
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
