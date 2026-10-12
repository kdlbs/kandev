package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func patchString(value string) *string { return &value }

func policyPatchStoreFixture(t *testing.T, repo *Repository) *models.RepositoryBranchPolicy {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-patch", Name: "patch"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repo-patch", WorkspaceID: "ws-patch", Name: "patch"}))
	policy := &models.RepositoryBranchPolicy{ID: "policy-patch", RepositoryID: "repo-patch", Name: "Feature", Description: "original",
		BaseBranch: "develop", BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "develop"}
	require.NoError(t, repo.CreateRepositoryBranchPolicy(ctx, policy))
	stored, err := repo.GetRepositoryBranchPolicy(ctx, policy.ID)
	require.NoError(t, err)
	return stored
}

func storePatchNormalizer(reset bool) func(*models.RepositoryBranchPolicy) (*models.RepositoryBranchPolicy, error) {
	return func(policy *models.RepositoryBranchPolicy) (*models.RepositoryBranchPolicy, error) {
		policy.Name = strings.TrimSpace(policy.Name)
		policy.Description = strings.TrimSpace(policy.Description)
		policy.BaseBranch = strings.TrimSpace(policy.BaseBranch)
		policy.PullRequestTarget = strings.TrimSpace(policy.PullRequestTarget)
		if reset && policy.PullRequestTarget == "" {
			policy.PullRequestTarget = policy.BaseBranch
		}
		if policy.Name == "" {
			return nil, errors.New("invalid policy name")
		}
		return policy, nil
	}
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-001.7, AC-WORKSPACES-BRANCH-POLICIES-001.9
func TestBranchPolicyPatchSQLite(t *testing.T) {
	for _, tc := range []struct {
		name  string
		check func(*testing.T, *Repository)
	}{
		{"presence and defaults", checkBranchPolicyPatchPresence}, {"rollback", checkBranchPolicyPatchRollback},
		{"uniqueness", checkBranchPolicyPatchUniqueness}, {"scope and deletion", checkBranchPolicyPatchScope},
	} {
		t.Run(tc.name, func(t *testing.T) { tc.check(t, newRepoForSetTests(t)) })
	}
	t.Run("independent current row", checkSQLitePolicyPatchIndependent)
}

func checkBranchPolicyPatchPresence(t *testing.T, repo *Repository) {
	policy := policyPatchStoreFixture(t, repo)
	ctx := context.Background()
	updated, err := repo.PatchRepositoryBranchPolicy(ctx, policy.ID, policy.RepositoryID,
		&models.RepositoryBranchPolicyPatch{BaseBranch: patchString(" main ")}, storePatchNormalizer(false))
	require.NoError(t, err)
	require.Equal(t, "main", updated.BaseBranch)
	require.Equal(t, "develop", updated.PullRequestTarget)
	require.Equal(t, policy.Description, updated.Description)
	reset, err := repo.PatchRepositoryBranchPolicy(ctx, policy.ID, policy.RepositoryID,
		&models.RepositoryBranchPolicyPatch{PullRequestTarget: patchString(" ")}, storePatchNormalizer(true))
	require.NoError(t, err)
	require.Equal(t, "main", reset.PullRequestTarget)
	stored, err := repo.GetRepositoryBranchPolicy(ctx, policy.ID)
	require.NoError(t, err)
	require.Equal(t, *reset, *stored)
	require.Equal(t, policy.CreatedAt, reset.CreatedAt)
}

func checkBranchPolicyPatchRollback(t *testing.T, repo *Repository) {
	original := policyPatchStoreFixture(t, repo)
	_, err := repo.PatchRepositoryBranchPolicy(context.Background(), original.ID, original.RepositoryID,
		&models.RepositoryBranchPolicyPatch{Name: patchString(""), Description: patchString("changed")}, storePatchNormalizer(false))
	require.Error(t, err)
	saved, err := repo.GetRepositoryBranchPolicy(context.Background(), original.ID)
	require.NoError(t, err)
	require.Equal(t, *original, *saved)
}

func checkBranchPolicyPatchUniqueness(t *testing.T, repo *Repository) {
	original := policyPatchStoreFixture(t, repo)
	duplicate := *original
	duplicate.ID, duplicate.Name = "other-policy", "Hotfix"
	require.NoError(t, repo.CreateRepositoryBranchPolicy(context.Background(), &duplicate))
	_, err := repo.PatchRepositoryBranchPolicy(context.Background(), original.ID, original.RepositoryID,
		&models.RepositoryBranchPolicyPatch{Name: patchString(" hotfix "), BaseBranch: patchString("main")}, storePatchNormalizer(false))
	require.ErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNameConflict)
	saved, err := repo.GetRepositoryBranchPolicy(context.Background(), original.ID)
	require.NoError(t, err)
	require.Equal(t, *original, *saved)
	collision := *original
	collision.Name = "HOTFIX"
	err = repo.UpdateRepositoryBranchPolicy(context.Background(), &collision)
	require.Error(t, err)
	require.True(t, isBranchPolicyNameConflict(err), "actual database uniqueness error was not recognized")
}

func checkBranchPolicyPatchScope(t *testing.T, repo *Repository) {
	original := policyPatchStoreFixture(t, repo)
	ctx := context.Background()
	patch := &models.RepositoryBranchPolicyPatch{Description: patchString("changed")}
	_, err := repo.PatchRepositoryBranchPolicy(ctx, original.ID, "foreign", patch, storePatchNormalizer(false))
	require.ErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNotFound)
	saved, err := repo.GetRepositoryBranchPolicy(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, *original, *saved)
	require.NoError(t, repo.DeleteRepository(ctx, original.RepositoryID))
	_, err = repo.PatchRepositoryBranchPolicy(ctx, original.ID, original.RepositoryID, patch, storePatchNormalizer(false))
	require.ErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNotFound)
}

func checkSQLitePolicyPatchIndependent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "independent.db")
	writer, err := db.OpenSQLite(filename)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	database := sqlx.NewDb(writer, "sqlite3")
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	second, err := db.OpenSQLite(filename)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	peerDB := sqlx.NewDb(second, "sqlite3")
	peer := NewWithInitializedDB(peerDB, peerDB, nil)
	policy := policyPatchStoreFixture(t, repo)
	// Retain a stale full row independently of the writer connection.
	stale, err := peer.GetRepositoryBranchPolicy(context.Background(), policy.ID)
	require.NoError(t, err)
	_, err = repo.PatchRepositoryBranchPolicy(context.Background(), policy.ID, policy.RepositoryID,
		&models.RepositoryBranchPolicyPatch{BaseBranch: patchString("main"), BranchTemplate: patchString("hotfix/{title}-{suffix}")}, storePatchNormalizer(false))
	require.NoError(t, err)
	result, err := peer.PatchRepositoryBranchPolicy(context.Background(), stale.ID, stale.RepositoryID,
		&models.RepositoryBranchPolicyPatch{Description: patchString("new"), PullRequestTarget: patchString("")}, storePatchNormalizer(true))
	require.NoError(t, err)
	require.Equal(t, "main", result.BaseBranch)
	require.Equal(t, "main", result.PullRequestTarget)
	require.Equal(t, "hotfix/{title}-{suffix}", result.BranchTemplate)
}
