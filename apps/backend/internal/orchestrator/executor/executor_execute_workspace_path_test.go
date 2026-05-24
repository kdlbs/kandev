package executor

import "testing"

// Pinpointed tests for computeWorkspacePath. The persisted workspace_path
// becomes agentctl's WorkDir on cold start (GetOrEnsureExecution); it must
// mirror the agent process cwd used on hot start (= cfg.WorkDir, which is
// metadata["worktree_path"] = req.WorkspacePath from the env preparer).
// Otherwise ACP session/load on resume fails with -32002 because the agent's
// jsonl was saved under a different sanitised-cwd folder.
func TestResolveTaskEnvWorkspacePath(t *testing.T) {
	t.Parallel()
	t.Run("multi-repo keeps task root", func(t *testing.T) {
		req := &LaunchAgentRequest{TaskDirName: "do-nothing_mvo"}
		resp := &LaunchAgentResponse{
			// Multi-repo preparer sets WorkspacePath to filepath.Dir(worktrees[0].WorktreePath)
			// = task root; executor_standalone copies that into metadata["worktree_path"],
			// and the lifecycle adapter surfaces it back as resp.WorktreePath.
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

	t.Run("single-repo keeps repo subdir", func(t *testing.T) {
		// Single-repo preparer sets WorkspacePath = wt.Path (with repo subdir);
		// the agent process starts at that cwd. Persisting it as-is keeps create
		// and resume cwd in sync so ACP session/load finds the saved jsonl.
		req := &LaunchAgentRequest{TaskDirName: "fix-thing_abc"}
		resp := &LaunchAgentResponse{
			WorktreePath: "/tmp/tasks/fix-thing_abc/kandev",
		}
		if got := computeWorkspacePath(req, resp); got != "/tmp/tasks/fix-thing_abc/kandev" {
			t.Fatalf("single-repo: want /tmp/tasks/fix-thing_abc/kandev, got %q", got)
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
