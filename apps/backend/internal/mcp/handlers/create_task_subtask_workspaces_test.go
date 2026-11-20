package handlers

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.3 AC-TASKS-MCP-WORKSPACE-MODE-004.6
func TestMCPSubtaskFreshWorkspaceRepositorySelection(t *testing.T) {
	f := newSelfPlacementFixture(t)
	repositories := []*models.Repository{
		{ID: "parent-repo", WorkspaceID: f.workspace.ID, Name: "Parent repo", DefaultBranch: "main"},
		{ID: "second-parent-repo", WorkspaceID: f.workspace.ID, Name: "Second parent repo", DefaultBranch: "develop"},
	}
	for _, repository := range repositories {
		require.NoError(t, f.repo.CreateRepository(f.ctx, repository))
	}
	for _, taskRepository := range []*models.TaskRepository{
		{TaskID: f.parent.ID, RepositoryID: repositories[0].ID, BaseBranch: "", CheckoutBranch: "feature/parent-a", Position: 0},
		{TaskID: f.parent.ID, RepositoryID: repositories[0].ID, BaseBranch: "main", CheckoutBranch: "feature/parent-b", Position: 1},
		{TaskID: f.parent.ID, RepositoryID: repositories[0].ID, BaseBranch: "release/v1", CheckoutBranch: "feature/parent-release", Position: 2},
		{TaskID: f.parent.ID, RepositoryID: repositories[1].ID, BaseBranch: "", CheckoutBranch: "feature/second", Position: 3},
	} {
		require.NoError(t, f.repo.CreateTaskRepository(f.ctx, taskRepository))
	}
	f.parent.Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{"default_child_workspace": mcpWorkspaceModeInheritParent},
	}
	require.NoError(t, f.repo.UpdateTask(f.ctx, f.parent))

	for _, tc := range []struct {
		workspaceMode string
		baseBranch    string
		wantBases     []string
		wantRepos     []string
	}{
		{wantBases: []string{"", "release/v1", ""}, wantRepos: []string{"parent-repo", "parent-repo", "second-parent-repo"}},
		{workspaceMode: mcpWorkspaceModeNewWorkspace, wantBases: []string{"", "release/v1", ""}, wantRepos: []string{"parent-repo", "parent-repo", "second-parent-repo"}},
		{baseBranch: "feature/override", wantBases: []string{"feature/override", "feature/override"}, wantRepos: []string{"parent-repo", "second-parent-repo"}},
	} {
		payload := selfPlacementCreatePayload(f, f.child.ID)
		if tc.workspaceMode != "" {
			payload["workspace_mode"] = tc.workspaceMode
		}
		if tc.baseBranch != "" {
			payload["base_branch"] = tc.baseBranch
		}
		created := selfPlacementResult(t, f.create(t, f.caller(), payload))
		child, err := f.svc.GetTask(f.ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, f.parent.ID, child.ParentID)
		require.Equal(t, mcpWorkspaceModeNewWorkspace, child.Metadata["workspace"].(map[string]interface{})["mode"])
		require.Len(t, child.Repositories, len(tc.wantBases), "fresh selections must group by repository and effective base branch")
		for repoIndex, wantBase := range tc.wantBases {
			require.Equal(t, tc.wantRepos[repoIndex], child.Repositories[repoIndex].RepositoryID)
			require.Equal(t, wantBase, child.Repositories[repoIndex].BaseBranch)
			require.Empty(t, child.Repositories[repoIndex].CheckoutBranch)
			if tc.baseBranch != "" {
				selection, found, selectionErr := models.LoadBaseBranchSelection(child.Repositories[repoIndex].Metadata)
				require.NoError(t, selectionErr)
				require.True(t, found)
				require.Equal(t, models.BaseBranchSelectionSourceExplicit, selection.Source)
				require.Equal(t, tc.baseBranch, selection.Branch)
				require.Equal(t, models.BaseBranchFallbackPolicyStrict,
					models.ResolveAttachmentBaseBranchFallbackPolicy(child.Repositories[repoIndex].Metadata, child.Repositories[repoIndex].BaseBranch))
			}
		}
	}
}

func TestMCPSubtaskSharedWorkspaceRepositorySlugCollisionUsesCanonicalSlots(t *testing.T) {
	f := newSelfPlacementFixture(t)
	repository := &models.Repository{
		ID: "slug-collision-repo", WorkspaceID: f.workspace.ID, Name: "Slug collision repo", DefaultBranch: "main",
	}
	require.NoError(t, f.repo.CreateRepository(f.ctx, repository))
	branches := []string{"feature/demo", "feature-demo"}
	inputs := make([]worktree.BranchIdentityInput, len(branches))
	for position, branch := range branches {
		inputs[position] = worktree.BranchIdentityInput{
			RepositoryID: repository.ID, BaseBranch: "main", CheckoutBranch: branch,
			DefaultBranch: repository.DefaultBranch, Position: position,
		}
		require.NoError(t, f.repo.CreateTaskRepository(f.ctx, &models.TaskRepository{
			TaskID: f.parent.ID, RepositoryID: repository.ID, BaseBranch: "main",
			CheckoutBranch: branch, Position: position,
		}))
	}
	plans := worktree.BuildBranchIdentityPlans(inputs)
	environment := &models.TaskEnvironment{
		ID: "slug-collision-environment", TaskID: f.parent.ID,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating,
		WorkspacePath: "/tmp/slug-collision-parent",
	}
	require.NoError(t, f.repo.CreateTaskEnvironment(f.ctx, environment))
	for index, branch := range branches {
		require.NoError(t, f.repo.CreateTaskEnvironmentRepo(f.ctx, &models.TaskEnvironmentRepo{
			ID: "slug-collision-row-" + string(rune('a'+index)), TaskEnvironmentID: environment.ID,
			RepositoryID: repository.ID, BranchSlug: plans[index].IdentitySlug,
			WorktreeID: "slug-collision-worktree-" + string(rune('a'+index)), WorktreeBranch: branch,
			Status: models.TaskEnvironmentRepoStatusActive,
		}))
	}
	environment.Status = models.TaskEnvironmentStatusReady
	require.NoError(t, f.repo.UpdateTaskEnvironment(f.ctx, environment))

	payload := selfPlacementCreatePayload(f, f.child.ID)
	payload["workspace_mode"] = mcpWorkspaceModeInheritParent
	created := selfPlacementResult(t, f.create(t, f.caller(), payload))
	child, err := f.svc.GetTask(f.ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, child.Repositories, 2)
	require.Equal(t, "feature/demo", child.Repositories[0].CheckoutBranch)
	require.Equal(t, "feature-demo", child.Repositories[1].CheckoutBranch)
}

func TestMCPSubtaskSharedWorkspaceAcceptsSingleLegacyUnscopedSlot(t *testing.T) {
	f := newSelfPlacementFixture(t)
	repository := &models.Repository{
		ID: "legacy-unscoped-repo", WorkspaceID: f.workspace.ID, Name: "Legacy repo", DefaultBranch: "main",
	}
	require.NoError(t, f.repo.CreateRepository(f.ctx, repository))
	require.NoError(t, f.repo.CreateTaskRepository(f.ctx, &models.TaskRepository{
		TaskID: f.parent.ID, RepositoryID: repository.ID, BaseBranch: "main",
	}))
	environment := &models.TaskEnvironment{
		ID: "legacy-unscoped-environment", TaskID: f.parent.ID,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating,
		WorkspacePath: "/tmp/legacy-unscoped-parent",
	}
	require.NoError(t, f.repo.CreateTaskEnvironment(f.ctx, environment))
	require.NoError(t, f.repo.CreateTaskEnvironmentRepo(f.ctx, &models.TaskEnvironmentRepo{
		ID: "legacy-unscoped-row", TaskEnvironmentID: environment.ID, RepositoryID: repository.ID,
		WorktreeID: "legacy-unscoped-worktree", Status: models.TaskEnvironmentRepoStatusActive,
	}))
	environment.Status = models.TaskEnvironmentStatusReady
	require.NoError(t, f.repo.UpdateTaskEnvironment(f.ctx, environment))

	payload := selfPlacementCreatePayload(f, f.child.ID)
	payload["workspace_mode"] = mcpWorkspaceModeInheritParent
	created := selfPlacementResult(t, f.create(t, f.caller(), payload))
	child, err := f.svc.GetTask(f.ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, child.Repositories, 1)
	require.Equal(t, repository.ID, child.Repositories[0].RepositoryID)
}

// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.8 AC-TASKS-MCP-WORKSPACE-MODE-004.9
func TestMCPSubtaskSharedWorkspaceRepositorySelection(t *testing.T) {
	f := newSelfPlacementFixture(t)
	repository := &models.Repository{
		ID: "shared-repo", WorkspaceID: f.workspace.ID, Name: "Shared repo", DefaultBranch: "main",
	}
	require.NoError(t, f.repo.CreateRepository(f.ctx, repository))
	for _, taskRepository := range []*models.TaskRepository{
		{TaskID: f.parent.ID, RepositoryID: repository.ID, BaseBranch: "main", Position: 0},
		{TaskID: f.parent.ID, RepositoryID: repository.ID, BaseBranch: "main", CheckoutBranch: "feature/adopted", WorkspaceRelativePath: ".claude/worktrees/feature-adopted", Position: 1},
	} {
		require.NoError(t, f.repo.CreateTaskRepository(f.ctx, taskRepository))
	}
	environment := &models.TaskEnvironment{
		ID: "parent-worktree-environment", TaskID: f.parent.ID,
		ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating,
		WorkspacePath: "/tmp/parent-worktree",
	}
	require.NoError(t, f.repo.CreateTaskEnvironment(f.ctx, environment))
	for _, repositoryState := range []*models.TaskEnvironmentRepo{
		{TaskEnvironmentID: environment.ID, RepositoryID: repository.ID, BranchSlug: "main", WorktreeID: "parent-primary", Status: models.TaskEnvironmentRepoStatusActive},
		{TaskEnvironmentID: environment.ID, RepositoryID: repository.ID, BranchSlug: "feature-adopted", WorkspaceRelativePath: ".claude/worktrees/feature-adopted", WorktreeID: "parent-adopted", Status: models.TaskEnvironmentRepoStatusActive},
		{TaskEnvironmentID: environment.ID, RepositoryID: repository.ID, BranchSlug: "retired", WorktreeID: "retired-worktree", Status: "failed"},
	} {
		require.NoError(t, f.repo.CreateTaskEnvironmentRepo(f.ctx, repositoryState))
	}
	environment.Status = models.TaskEnvironmentStatusReady
	require.NoError(t, f.repo.UpdateTaskEnvironment(f.ctx, environment))

	payload := selfPlacementCreatePayload(f, f.child.ID)
	payload["workspace_mode"] = mcpWorkspaceModeInheritParent
	payload["external_id"] = "shared-checkout-retry"
	created := selfPlacementResult(t, f.create(t, f.caller(), payload))
	child, err := f.svc.GetTask(f.ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, f.parent.ID, child.ParentID)
	require.Len(t, child.Repositories, 2)
	require.Equal(t, repository.ID, child.Repositories[0].RepositoryID)
	require.Equal(t, "main", child.Repositories[0].BaseBranch)
	require.Equal(t, "main", child.Repositories[1].BaseBranch)
	require.Equal(t, "feature/adopted", child.Repositories[1].CheckoutBranch)
	require.Equal(t, ".claude/worktrees/feature-adopted", child.Repositories[1].WorkspaceRelativePath)
	reuseOnly, _ := child.Repositories[1].Metadata["inherited_checkout_reuse_only"].(bool)
	require.True(t, reuseOnly)

	rows, err := f.repo.ListTaskEnvironmentRepos(f.ctx, environment.ID)
	require.NoError(t, err)
	var adopted *models.TaskEnvironmentRepo
	for _, row := range rows {
		if row != nil && row.WorktreeID == "parent-adopted" {
			adopted = row
			break
		}
	}
	require.NotNil(t, adopted)
	deletedAt := time.Now()
	adopted.Status = models.TaskEnvironmentRepoStatusDetached
	adopted.DeletedAt = &deletedAt
	require.NoError(t, f.repo.UpdateTaskEnvironmentRepo(f.ctx, adopted))

	retryPayload := selfPlacementCreatePayload(f, f.child.ID)
	retryPayload["workspace_mode"] = mcpWorkspaceModeInheritParent
	retryPayload["external_id"] = "shared-checkout-retry"
	retryPayload["title"] = "Changed retry"
	retry := selfPlacementResult(t, f.create(t, f.caller(), retryPayload))
	require.Equal(t, created.ID, retry.ID)
	require.True(t, retry.Deduplicated, "a settled create retry must bypass changed workspace inventory")

	assertNoWrites := func() {
		t.Helper()
		require.Equal(t, 3, f.taskCount(t), "a rejected inherited selection must not insert a task")
		var sessions int
		require.NoError(t, f.repo.DB().QueryRowContext(f.ctx, "SELECT COUNT(*) FROM task_sessions").Scan(&sessions))
		require.Equal(t, 1, sessions, "a rejected inherited selection must not insert a session")
	}

	missingPayload := selfPlacementCreatePayload(f, f.child.ID)
	missingPayload["workspace_mode"] = mcpWorkspaceModeInheritParent
	missingPayload["external_id"] = "missing-checkout"
	missingPayload["title"] = "Missing shared checkout"
	missing := f.create(t, f.caller(), missingPayload)
	assertWSError(t, missing, "VALIDATION_ERROR")
	require.Contains(t, string(missing.Payload), repository.ID)
	require.Contains(t, string(missing.Payload), "feature/adopted")
	assertNoWrites()

	adopted.Status = models.TaskEnvironmentRepoStatusActive
	adopted.DeletedAt = nil
	require.NoError(t, f.repo.UpdateTaskEnvironmentRepo(f.ctx, adopted))
	adopted.BranchSlug = "moved-adopted-slot"
	adopted.WorktreeBranch = "feature/adopted"
	require.NoError(t, f.repo.UpdateTaskEnvironmentRepo(f.ctx, adopted))
	duplicate := *adopted
	duplicate.ID = "ambiguous-adopted-checkout"
	duplicate.BranchSlug = "other-adopted-slot"
	duplicate.WorktreeBranch = "feature/adopted"
	duplicate.WorktreeID = "parent-adopted-duplicate"
	require.NoError(t, f.repo.CreateTaskEnvironmentRepo(f.ctx, &duplicate))
	ambiguousPayload := selfPlacementCreatePayload(f, f.child.ID)
	ambiguousPayload["workspace_mode"] = mcpWorkspaceModeInheritParent
	ambiguousPayload["external_id"] = "ambiguous-checkout"
	ambiguousPayload["title"] = "Ambiguous shared checkout"
	ambiguous := f.create(t, f.caller(), ambiguousPayload)
	assertWSError(t, ambiguous, "VALIDATION_ERROR")
	require.Contains(t, string(ambiguous.Payload), repository.ID)
	require.Contains(t, string(ambiguous.Payload), "feature/adopted")
	assertNoWrites()
}
