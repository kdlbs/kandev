package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1
// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
// @covers AC-TASKS-RUNTIME-CLEANUP-001.2
func TestPersistTaskEnvironmentRepos_PreservesPhysicalWorktreeOnInventoryOnlyRefresh(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	const envID = "env-inventory-only"
	repo.taskEnvironmentRepos[envID] = []*models.TaskEnvironmentRepo{
		{
			ID:                "env-inventory-only-repo-main",
			TaskEnvironmentID: envID,
			RepositoryID:      "repo-kandev",
			BranchSlug:        "main",
			WorktreeID:        "wt-existing",
			WorktreePath:      "/tasks/task-1/repo",
			WorktreeBranch:    "feature/task-1",
			Position:          4,
			ErrorMessage:      "stale inventory error",
		},
	}

	repos := environmentReposForLaunch(
		&LaunchAgentRequest{
			RepositoryID:       "repo-kandev",
			RepositoryPath:     "/source/kandev",
			BranchIdentitySlug: "main",
		},
		&LaunchAgentResponse{WorkspacePath: "/source/kandev"},
	)
	exec.persistTaskEnvironmentRepos(context.Background(), envID, repos)

	row := repo.taskEnvironmentRepos[envID][0]
	if row.WorktreeID != "wt-existing" || row.WorktreePath != "/tasks/task-1/repo" || row.WorktreeBranch != "feature/task-1" {
		t.Fatalf("inventory-only refresh changed physical worktree: %+v", row)
	}
	if row.Position != 0 || row.ErrorMessage != "" {
		t.Fatalf("inventory-only refresh did not update inventory metadata: position=%d error=%q", row.Position, row.ErrorMessage)
	}
}

func TestPersistTaskEnvironmentRepos_ReplacesAndDeletesOmittedRows(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	const envID = "env-transition-replace"
	repo.taskEnvironmentRepos[envID] = []*models.TaskEnvironmentRepo{
		{ID: "keep", TaskEnvironmentID: envID, RepositoryID: "repo-1", BranchSlug: "main", WorktreeID: "wt-main", Status: "active"},
		{ID: "remove", TaskEnvironmentID: envID, RepositoryID: "repo-2", BranchSlug: "old", WorktreeID: "wt-old", Status: "active"},
	}

	if err := exec.persistTaskEnvironmentReposForTransition(context.Background(), envID, []*models.TaskEnvironmentRepo{
		{RepositoryID: "repo-1", BranchSlug: "main", WorktreeID: "wt-new", WorktreePath: "/new", WorktreeBranch: "main"},
	}, true); err != nil {
		t.Fatalf("persistTaskEnvironmentReposForTransition: %v", err)
	}

	rows := repo.taskEnvironmentRepos[envID]
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want retained rows including tombstone", len(rows))
	}
	var kept, removed *models.TaskEnvironmentRepo
	for _, row := range rows {
		switch row.ID {
		case "keep":
			kept = row
		case "remove":
			removed = row
		}
	}
	if kept == nil || kept.WorktreeID != "wt-new" {
		t.Fatalf("kept row = %#v, want refreshed physical identity", kept)
	}
	if removed == nil || removed.Status != taskEnvironmentRepoStatusDeleted || removed.DeletedAt == nil {
		t.Fatalf("removed row = %#v, want deleted tombstone", removed)
	}
}

func TestPersistTaskEnvironmentReposForTransitionClearsCompactionMetadataOnReplacement(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	const envID = "env-transition-replace-compacted"
	compactedAt := time.Now().UTC().Add(-time.Hour)
	repo.taskEnvironmentRepos[envID] = []*models.TaskEnvironmentRepo{
		{
			ID: "replace", TaskEnvironmentID: envID, RepositoryID: "repo-1", BranchSlug: "main",
			WorktreeID: "wt-old", WorktreeBranch: "feature/old", WorktreeBranchOwner: "kandev",
			WorktreeIntegrationRef: "main", WorktreeRecoveryHeadSHA: strings.Repeat("a", 40),
			WorktreeBranchCompactedAt: &compactedAt, Status: taskEnvironmentRepoStatusDeleted, DeletedAt: &compactedAt,
		},
	}

	if err := exec.persistTaskEnvironmentReposForTransition(context.Background(), envID, []*models.TaskEnvironmentRepo{
		{
			RepositoryID: "repo-1", BranchSlug: "main", WorktreeID: "wt-new",
			WorktreePath: "/new", WorktreeBranch: "feature/new",
			WorktreeBranchOwner: "kandev", WorktreeIntegrationRef: "main",
		},
	}, true); err != nil {
		t.Fatalf("persistTaskEnvironmentReposForTransition: %v", err)
	}

	row := repo.taskEnvironmentRepos[envID][0]
	if row.Status != taskEnvironmentRepoStatusActive || row.DeletedAt != nil || row.WorktreeID != "wt-new" {
		t.Fatalf("replacement row = %#v, want active replacement worktree", row)
	}
	if row.WorktreeRecoveryHeadSHA != "" || row.WorktreeBranchCompactedAt != nil {
		t.Fatalf("replacement row kept stale compaction metadata: head=%q compacted_at=%v", row.WorktreeRecoveryHeadSHA, row.WorktreeBranchCompactedAt)
	}
}

func TestPersistTaskEnvironmentReposForTransitionClearsMissingIntegrationRef(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	const envID = "env-transition-empty-integration"
	repo.taskEnvironmentRepos[envID] = []*models.TaskEnvironmentRepo{
		{
			ID: "replace", TaskEnvironmentID: envID, RepositoryID: "repo-1", BranchSlug: "main",
			WorktreeID: "wt-old", WorktreeBranch: "feature/old", WorktreeBranchOwner: "kandev",
			WorktreeIntegrationRef: "main", Status: taskEnvironmentRepoStatusActive,
		},
	}

	if err := exec.persistTaskEnvironmentReposForTransition(context.Background(), envID, []*models.TaskEnvironmentRepo{
		{
			RepositoryID: "repo-1", BranchSlug: "main", WorktreeID: "wt-new",
			WorktreePath: "/new", WorktreeBranch: "feature/new",
			WorktreeBranchOwner: "kandev", WorktreeIntegrationRef: "",
		},
	}, true); err != nil {
		t.Fatalf("persistTaskEnvironmentReposForTransition: %v", err)
	}

	row := repo.taskEnvironmentRepos[envID][0]
	if row.WorktreeIntegrationRef != "" {
		t.Fatalf("WorktreeIntegrationRef = %q, want empty so cleanup fails closed", row.WorktreeIntegrationRef)
	}
}

func TestEnvironmentReposForLaunch_PersistsBranchCompactionMetadata(t *testing.T) {
	repos := environmentReposForLaunch(
		&LaunchAgentRequest{},
		&LaunchAgentResponse{Worktrees: []RepoWorktreeResult{{
			RepositoryID: "repo-kandev", WorktreeID: "wt-managed", WorktreeBranch: "feature/task",
			WorktreeBranchOwner: "kandev", WorktreeIntegrationRef: "develop",
		}}},
	)
	if len(repos) != 1 || repos[0].WorktreeBranchOwner != "kandev" || repos[0].WorktreeIntegrationRef != "develop" {
		t.Fatalf("environment repository metadata = %+v", repos)
	}
}
