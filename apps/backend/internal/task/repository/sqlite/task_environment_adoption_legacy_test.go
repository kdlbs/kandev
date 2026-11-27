package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func exerciseWorkspaceAdoptionLegacyBinding(t *testing.T, r *Repository, competing bool) {
	p := seedWorkspaceAdoptionDB(t, r)
	ctx := context.Background()
	if competing {
		_, err := r.db.Exec(r.db.Rebind(`INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,branch_slug,position,status,created_at,updated_at) VALUES('legacy-slot','adopt-env','adopt-repo','legacy-wt',?,'feature/adopt','legacy',0,'',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), p.Proof.CandidatePath)
		require.NoError(t, err)
		session, err := r.GetTaskSession(ctx, "adopt-session")
		require.NoError(t, err)
		environment, err := r.GetTaskEnvironment(ctx, "adopt-env")
		require.NoError(t, err)
		entity, err := r.GetRepository(ctx, "adopt-repo")
		require.NoError(t, err)
		p.Selection.Snapshot = models.NewWorkspaceRecoverySelectionSnapshot(session, environment, map[string]*models.Repository{entity.ID: entity})
		p.Token = p.Digest()
	} else {
		_, err := r.db.Exec(`UPDATE task_environment_repos SET status='' WHERE id='adopt-slot'`)
		require.NoError(t, err)
	}
	claim := acquireWorkspaceAdoptionClaim(t, r, p)
	receipt, err := r.CommitWorkspaceAdoption(ctx, p, claim.OperationID, claim, func() error { return nil })
	if competing {
		require.ErrorIs(t, err, models.ErrWorkspaceAdoptionStale)
		assertAdoptionDBUnpublished(t, r, p)
		return
	}
	require.NoError(t, err)
	require.NoError(t, r.ValidateWorkspaceAdoption(ctx, receipt))
	var path, status string
	require.NoError(t, r.db.QueryRow(`SELECT worktree_path,status FROM task_environment_repos WHERE id='adopt-slot'`).Scan(&path, &status))
	require.Equal(t, p.Proof.CandidatePath, path)
	require.Empty(t, status)
}

func TestWorkspaceAdoptionLegacyBindingSQLite(t *testing.T) {
	for _, competing := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected", true: "competing"}[competing], func(t *testing.T) {
			exerciseWorkspaceAdoptionLegacyBinding(t, newRepoForEntityTests(t), competing)
		})
	}
}

func TestWorkspaceAdoptionLegacyBindingPostgres(t *testing.T) {
	for _, competing := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected", true: "competing"}[competing], func(t *testing.T) {
			r, _, _ := newTaskPostgresRepoPair(t)
			exerciseWorkspaceAdoptionLegacyBinding(t, r, competing)
		})
	}
}

func exerciseWorkspaceAdoptionEmptyMetadata(t *testing.T, r *Repository) {
	p := seedWorkspaceAdoptionDB(t, r)
	_, err := r.db.Exec(`UPDATE task_repositories SET metadata='null' WHERE id='adopt-attachment'`)
	require.NoError(t, err)
	claim := acquireWorkspaceAdoptionClaim(t, r, p)
	receipt, err := r.CommitWorkspaceAdoption(context.Background(), p, claim.OperationID, claim, func() error { return nil })
	require.NoError(t, err)
	require.NoError(t, r.ValidateWorkspaceAdoption(context.Background(), receipt))
}

func TestWorkspaceAdoptionEmptyMetadataSQLite(t *testing.T) {
	exerciseWorkspaceAdoptionEmptyMetadata(t, newRepoForEntityTests(t))
}

func TestWorkspaceAdoptionEmptyMetadataPostgres(t *testing.T) {
	r, _, _ := newTaskPostgresRepoPair(t)
	exerciseWorkspaceAdoptionEmptyMetadata(t, r)
}
