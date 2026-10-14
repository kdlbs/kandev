package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func exerciseWorkspaceAdoptionNativeWorkingDirectory(t *testing.T, r *Repository) {
	p := seedWorkspaceAdoptionDB(t, r)
	ctx := context.Background()
	for _, table := range []string{"task_environments", "task_sessions"} {
		_, err := r.db.Exec(r.db.Rebind("UPDATE "+table+" SET workspace_path=?"), p.Proof.OriginalPath)
		require.NoError(t, err)
	}
	p.Selection.Snapshot.WorkspacePath = p.Proof.OriginalPath
	p.Selection.SessionWorkspacePath = p.Proof.OriginalPath
	p.Token = p.Digest()
	claim := acquireWorkspaceAdoptionClaim(t, r, p)
	injected := errors.New("candidate replaced before commit")
	_, err := r.CommitWorkspaceAdoption(ctx, p, claim.OperationID, claim, func() error { return injected })
	require.ErrorIs(t, err, injected)
	assertAdoptionDBUnpublished(t, r, p)
	for _, table := range []string{"task_environments", "task_sessions"} {
		var cwd string
		require.NoError(t, r.db.Get(&cwd, "SELECT workspace_path FROM "+table))
		require.Equal(t, p.Proof.OriginalPath, cwd)
	}
	receipt, err := r.CommitWorkspaceAdoption(ctx, p, claim.OperationID, claim, func() error { return nil })
	require.NoError(t, err)
	for _, table := range []string{"task_environments", "task_sessions"} {
		var cwd string
		require.NoError(t, r.db.Get(&cwd, "SELECT workspace_path FROM "+table))
		require.Equal(t, p.Proof.CandidatePath, cwd)
	}
	require.NoError(t, r.ValidateWorkspaceAdoption(ctx, receipt))
	require.NoError(t, r.FinalizeWorkspaceAdoption(ctx, receipt))
	stored, err := NewWithInitializedDB(r.db, r.ro, nil).GetWorkspaceAdoption(ctx, claim.OperationID)
	require.NoError(t, err)
	require.NoError(t, r.ValidateWorkspaceAdoption(ctx, stored))
}

func TestWorkspaceAdoptionNativeWorkingDirectorySQLite(t *testing.T) {
	exerciseWorkspaceAdoptionNativeWorkingDirectory(t, newRepoForEntityTests(t))
}

func TestWorkspaceAdoptionNativeWorkingDirectoryPostgres(t *testing.T) {
	r, _, _ := newTaskPostgresRepoPair(t)
	exerciseWorkspaceAdoptionNativeWorkingDirectory(t, r)
}
