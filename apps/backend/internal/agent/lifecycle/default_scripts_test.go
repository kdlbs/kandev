package lifecycle

import (
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
		`git fetch --depth=1 origin "{{worktree.branch}}"`,
		`git checkout -B "{{worktree.branch}}" "origin/{{worktree.branch}}"`,
		`git checkout -b "{{worktree.branch}}"`,
		`|| true`,
	}
	for _, w := range want {
		if !strings.Contains(postlude, w) {
			t.Errorf("postlude missing %q", w)
		}
	}
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
