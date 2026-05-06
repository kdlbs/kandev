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

// TestDefaultPrepareScripts_FeatureBranchCheckout asserts that both clone-based
// remote scripts include the kandev feature-branch checkout block, that the
// block prefers the remote tip when one exists (resume-after-destroyed-sandbox
// recovery), and that it falls back to creating a fresh local branch.
func TestDefaultPrepareScripts_FeatureBranchCheckout(t *testing.T) {
	cases := []struct {
		executorType string
		want         []string
	}{
		{
			executorType: "local_docker",
			want: []string{
				`if [ -n "{{worktree.branch}}" ] && [ "{{worktree.branch}}" != "{{repository.branch}}" ]; then`,
				`if git fetch --depth=1 origin "{{worktree.branch}}" 2>/dev/null; then`,
				`git checkout -B "{{worktree.branch}}" "origin/{{worktree.branch}}"`,
				`git checkout -b "{{worktree.branch}}"`,
			},
		},
		{
			executorType: "remote_docker",
			want: []string{
				`if [ -n "{{worktree.branch}}" ] && [ "{{worktree.branch}}" != "{{repository.branch}}" ]; then`,
				`if git fetch --depth=1 origin "{{worktree.branch}}" 2>/dev/null; then`,
				`git checkout -B "{{worktree.branch}}" "origin/{{worktree.branch}}"`,
				`git checkout -b "{{worktree.branch}}"`,
			},
		},
		{
			executorType: "sprites",
			want: []string{
				`if [ -n "{{worktree.branch}}" ] && [ "{{worktree.branch}}" != "{{repository.branch}}" ]; then`,
				`if git fetch --depth=1 origin "{{worktree.branch}}" 2>/dev/null; then`,
				`git checkout -B "{{worktree.branch}}" "origin/{{worktree.branch}}"`,
				`git checkout -b "{{worktree.branch}}"`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.executorType, func(t *testing.T) {
			script := DefaultPrepareScript(tc.executorType)
			if script == "" {
				t.Fatalf("DefaultPrepareScript(%q) returned empty", tc.executorType)
			}
			for _, want := range tc.want {
				if !strings.Contains(script, want) {
					t.Errorf("script for %q missing %q", tc.executorType, want)
				}
			}
		})
	}
}
