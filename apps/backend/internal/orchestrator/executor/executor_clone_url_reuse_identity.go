package executor

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// A fresh resume can materialize the base branch instead of the configured checkout.
func (e *Executor) pinCanonicalCloneURLBaseIdentity(req *LaunchAgentRequest, env *models.TaskEnvironment) {
	if req.UseWorktree || len(req.Repositories) != 0 || req.CheckoutBranch != "" {
		return
	}
	if env.TaskID != req.TaskID || env.ExecutorType != req.ExecutorType {
		return
	}
	if e.capabilities == nil || !e.capabilities.RequiresCloneURL(req.ExecutorType) {
		return
	}
	spec, ok := topLevelLaunchRepoSpec(req)
	if !ok || canonicalInventoryMatches(spec, env.Repos, false) != 0 {
		return
	}
	spec.BranchIdentitySlug = worktree.SanitizeBranchSlug(req.BaseBranch)
	if spec.BranchIdentitySlug == "" {
		return
	}
	matches := matchingCanonicalEnvironmentRepoRows(spec, env.Repos, false)
	if len(matches) != 1 {
		return
	}
	row := matches[0]
	if row.TaskEnvironmentID != env.ID || row.Status != taskEnvironmentRepoStatusActive {
		return
	}
	req.BranchIdentitySlug = worktree.SanitizeBranchSlug(row.BranchSlug)
}
