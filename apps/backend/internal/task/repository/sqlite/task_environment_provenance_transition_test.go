package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func exerciseTaskEnvironmentTransitionRetainsProvenance(t *testing.T, r *Repository) {
	p := seedWorkspaceAdoptionDB(t, r)
	ctx := context.Background()
	compacted := time.Date(2026, time.October, 14, 1, 2, 3, 0, time.UTC)
	_, err := r.db.Exec(r.db.Rebind(`UPDATE task_environment_repos SET worktree_source_clone_path=?,worktree_source_common_dir=?,worktree_branch_owner='task',worktree_integration_ref='refs/heads/main',worktree_recovery_head_sha='0123456789012345678901234567890123456789',worktree_branch_compacted_at=? WHERE id='adopt-slot'`), p.Proof.CandidatePath, p.Proof.CandidatePath+"/.git", compacted)
	require.NoError(t, err)
	env, err := r.GetTaskEnvironment(ctx, "adopt-env")
	require.NoError(t, err)
	before := *env.Repos[0]
	require.NoError(t, r.PersistTaskEnvironmentTransition(ctx, env, []*models.TaskEnvironmentRepo{{
		RepositoryID: before.RepositoryID, BranchSlug: before.BranchSlug,
		WorktreeID: before.WorktreeID, WorktreePath: before.WorktreePath, WorktreeBranch: before.WorktreeBranch,
	}}, false))
	after, err := r.ListTaskEnvironmentRepos(ctx, env.ID)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assertEnvironmentProvenance(t, &before, after[0])
	sessionRows, err := r.ListTaskSessionWorktrees(ctx, "adopt-session")
	require.NoError(t, err)
	require.Len(t, sessionRows, 1)
	assertEnvironmentProvenance(t, &before, sessionRows[0])
	bulk, err := r.ListWorktreesBySessionIDs(ctx, []string{"adopt-session"})
	require.NoError(t, err)
	require.Len(t, bulk["adopt-session"], 1)
	assertEnvironmentProvenance(t, &before, bulk["adopt-session"][0])
}

func assertEnvironmentProvenance(t *testing.T, expected, actual *models.TaskEnvironmentRepo) {
	t.Helper()
	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.WorktreeID, actual.WorktreeID)
	require.Equal(t, expected.WorktreeBranchOwner, actual.WorktreeBranchOwner)
	require.Equal(t, expected.WorktreeIntegrationRef, actual.WorktreeIntegrationRef)
	require.Equal(t, expected.WorktreeRecoveryHeadSHA, actual.WorktreeRecoveryHeadSHA)
	require.Equal(t, expected.WorktreeSourceClonePath, actual.WorktreeSourceClonePath)
	require.Equal(t, expected.WorktreeSourceCommonDir, actual.WorktreeSourceCommonDir)
	require.Equal(t, expected.WorktreeBranchCompactedAt, actual.WorktreeBranchCompactedAt)
}

func TestTaskEnvironmentTransitionRetainsProvenanceSQLite(t *testing.T) {
	exerciseTaskEnvironmentTransitionRetainsProvenance(t, newRepoForEntityTests(t))
}

func TestTaskEnvironmentTransitionRetainsProvenancePostgres(t *testing.T) {
	r, _, _ := newTaskPostgresRepoPair(t)
	exerciseTaskEnvironmentTransitionRetainsProvenance(t, r)
}
