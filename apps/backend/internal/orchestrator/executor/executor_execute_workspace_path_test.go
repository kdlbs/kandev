package executor

import "testing"

// Pinpointed tests for computeWorkspacePath. The persisted workspace_path
// becomes agentctl's WorkDir on cold start (GetOrEnsureExecution); pointing it
// at the wrong directory disables the per-repo tracker fan-out and silently
// drops the Changes panel back to single-repo mode.
func TestResolveTaskEnvWorkspacePath(t *testing.T) {
	t.Parallel()
	t.Run("multi-repo keeps task root", func(t *testing.T) {
		req := &LaunchAgentRequest{TaskDirName: "do-nothing_mvo"}
		resp := &LaunchAgentResponse{
			// Lifecycle adapter mirrors agentctl WorkDir (task root) into
			// the legacy WorktreePath field for multi-repo launches.
			WorktreePath: "/tmp/tasks/do-nothing_mvo",
			Worktrees: []RepoWorktreeResult{
				{WorktreePath: "/tmp/tasks/do-nothing_mvo/kandev"},
				{WorktreePath: "/tmp/tasks/do-nothing_mvo/thm"},
			},
		}
		if got := computeWorkspacePath(req, resp); got != "/tmp/tasks/do-nothing_mvo" {
			t.Fatalf("multi-repo: want /tmp/tasks/do-nothing_mvo, got %q", got)
		}
	})

	t.Run("single-repo derives task root from repo subdir", func(t *testing.T) {
		req := &LaunchAgentRequest{TaskDirName: "fix-thing_abc"}
		resp := &LaunchAgentResponse{
			WorktreePath: "/tmp/tasks/fix-thing_abc/kandev",
		}
		if got := computeWorkspacePath(req, resp); got != "/tmp/tasks/fix-thing_abc" {
			t.Fatalf("single-repo: want /tmp/tasks/fix-thing_abc, got %q", got)
		}
	})

	t.Run("non-task-dir mode passes worktree path through", func(t *testing.T) {
		req := &LaunchAgentRequest{} // TaskDirName empty
		resp := &LaunchAgentResponse{WorktreePath: "/legacy/worktrees/xyz"}
		if got := computeWorkspacePath(req, resp); got != "/legacy/worktrees/xyz" {
			t.Fatalf("non-task-dir: want /legacy/worktrees/xyz, got %q", got)
		}
	})

	t.Run("no worktree falls back to repository path", func(t *testing.T) {
		req := &LaunchAgentRequest{RepositoryPath: "/repos/myrepo"}
		resp := &LaunchAgentResponse{} // no WorktreePath
		if got := computeWorkspacePath(req, resp); got != "/repos/myrepo" {
			t.Fatalf("fallback: want /repos/myrepo, got %q", got)
		}
	})
}
