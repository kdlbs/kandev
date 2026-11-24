package service

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestActiveInheritedWorktreeMatchesUsesLegacyRowWhenScopedRowsAreInactive(t *testing.T) {
	statuses := []string{"failed", "deleted", models.TaskEnvironmentRepoStatusDetached}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			legacy := &models.TaskEnvironmentRepo{
				ID: "legacy", TaskEnvironmentID: "env-parent", RepositoryID: "repo-parent",
				WorktreeID: "worktree-legacy", Status: "active",
			}
			inactive := &models.TaskEnvironmentRepo{
				ID: "inactive", TaskEnvironmentID: "env-parent", RepositoryID: "repo-parent",
				BranchSlug: "feature-stale", WorktreeID: "worktree-inactive", Status: status,
			}

			matches := activeInheritedWorktreeMatches(
				TaskRepositoryInput{RepositoryID: "repo-parent", InheritedWorkspaceSlot: true},
				worktree.BranchIdentityPlan{IdentitySlug: "main"},
				"env-parent",
				[]*models.TaskEnvironmentRepo{legacy, inactive},
			)
			if len(matches) != 1 || matches[0] != legacy {
				t.Fatalf("matches = %#v, want only the live legacy row", matches)
			}
		})
	}
}

func TestActiveInheritedWorktreeMatchesDoesNotUseLegacySlotWhenAnotherLiveScopedSlotExists(t *testing.T) {
	legacy := &models.TaskEnvironmentRepo{
		ID: "legacy", TaskEnvironmentID: "env-parent", RepositoryID: "repo-parent",
		WorktreeID: "worktree-legacy", Status: "active",
	}
	scoped := &models.TaskEnvironmentRepo{
		ID: "scoped", TaskEnvironmentID: "env-parent", RepositoryID: "repo-parent",
		WorkspaceRelativePath: ".other", BranchSlug: "feature-other",
		WorktreeID: "worktree-scoped", Status: "active",
	}

	matches := activeInheritedWorktreeMatches(
		TaskRepositoryInput{RepositoryID: "repo-parent", InheritedWorkspaceSlot: true},
		worktree.BranchIdentityPlan{IdentitySlug: "main"},
		"env-parent",
		[]*models.TaskEnvironmentRepo{legacy, scoped},
	)
	if len(matches) != 0 {
		t.Fatalf("matches = %#v, want no fallback to a legacy slot when another scoped slot is live", matches)
	}
}
