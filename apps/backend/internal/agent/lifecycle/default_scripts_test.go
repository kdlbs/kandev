package lifecycle

import (
	"strings"
	"testing"
)

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
