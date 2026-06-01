package orchestrator

import "testing"

// TestIsMultiBranchSubdir guards the push-event-to-repo resolution. Multi-
// branch tasks lay sibling worktrees out as `<repo.Name>-<branch-slug>/`.
// When agentctl's per-repo tracker emits a push event tagged with that
// subdir name, the orchestrator must recognize it as belonging to the
// underlying repository — otherwise `resolvePushRepo` returns empty,
// detect-and-associate short-circuits, and the secondary PR never lands
// in github_task_prs.
func TestIsMultiBranchSubdir(t *testing.T) {
	cases := []struct {
		subdir   string
		repoName string
		want     bool
	}{
		{"kandev", "kandev", false},                  // exact match handled elsewhere
		{"kandev-feature-x", "kandev", true},         // sibling worktree
		{"kandev-branch-2", "kandev", true},          // auto-named secondary
		{"kandev-feature/fibo-iter", "kandev", true}, // unusual slug forms
		{"kandev2", "kandev", false},                 // not a sibling (no separator)
		{"other-repo", "kandev", false},              // unrelated
		{"", "kandev", false},                        // empty event tag
		{"kandev-x", "", false},                      // empty repo name
		{"kandev-", "kandev", false},                 // separator only, no slug
	}
	for _, c := range cases {
		if got := isMultiBranchSubdir(c.subdir, c.repoName); got != c.want {
			t.Errorf("isMultiBranchSubdir(%q, %q) = %v, want %v", c.subdir, c.repoName, got, c.want)
		}
	}
}
