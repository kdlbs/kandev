package executor

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// A comparison base can change while the environment retains the same checkout.
func pinCanonicalWorktreeIdentities(req *LaunchAgentRequest, env *models.TaskEnvironment) {
	if !ownsCanonicalWorktreeEnvironment(req, env) {
		return
	}
	specs := req.Repositories
	topLevel := len(specs) == 0
	if topLevel {
		spec, ok := topLevelLaunchRepoSpec(req)
		if !ok {
			return
		}
		specs = []RepoSpec{spec}
	}
	for i, spec := range specs {
		if spec.CheckoutBranch != "" || canonicalInventoryMatches(spec, env.Repos, true) != 0 ||
			requestedWorktreeSlotCount(spec, specs) != 1 {
			continue
		}
		row := uniqueCanonicalWorktreeSlot(spec, env)
		if row == nil || (spec.WorktreeID != "" && spec.WorktreeID != row.WorktreeID) {
			continue
		}
		specs[i].BranchIdentitySlug = worktree.SanitizeBranchSlug(row.BranchSlug)
		specs[i].WorktreeID = row.WorktreeID
		if specs[i].BranchSlug != "" {
			specs[i].BranchSlug = specs[i].BranchIdentitySlug
		}
	}
	if topLevel {
		req.BranchIdentitySlug = specs[0].BranchIdentitySlug
		req.WorktreeID = specs[0].WorktreeID
		return
	}
	req.Repositories = specs
}

func requestedWorktreeSlotCount(selected RepoSpec, specs []RepoSpec) int {
	count := 0
	for _, spec := range specs {
		if spec.RepositoryID == selected.RepositoryID && spec.WorkspaceRelativePath == selected.WorkspaceRelativePath {
			count++
		}
	}
	return count
}

func uniqueCanonicalWorktreeSlot(spec RepoSpec, env *models.TaskEnvironment) *models.TaskEnvironmentRepo {
	var selected *models.TaskEnvironmentRepo
	for _, row := range env.Repos {
		if row == nil || row.TaskEnvironmentID != env.ID || row.RepositoryID != spec.RepositoryID ||
			row.WorkspaceRelativePath != spec.WorkspaceRelativePath || !completeCanonicalWorktreeIdentity(row) ||
			row.DeletedAt != nil || row.Status != taskEnvironmentRepoStatusActive {
			continue
		}
		if selected != nil {
			return nil
		}
		selected = row
	}
	return selected
}

func ownsCanonicalWorktreeEnvironment(req *LaunchAgentRequest, env *models.TaskEnvironment) bool {
	return req.UseWorktree && env != nil && env.ID != "" && env.TaskID == req.TaskID &&
		env.ExecutorType == string(models.ExecutorTypeWorktree)
}

func completeCanonicalWorktreeIdentity(row *models.TaskEnvironmentRepo) bool {
	return row.WorktreeID != "" && row.WorktreePath != "" && row.WorktreeBranch != "" &&
		row.WorktreeSourceClonePath != "" && row.WorktreeSourceCommonDir != "" &&
		worktree.SanitizeBranchSlug(row.BranchSlug) != ""
}
