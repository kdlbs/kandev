package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/checkoutadoption"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func exerciseWorkspaceAdoptionReceiptAuthority(t *testing.T, newRepository func(*testing.T) *Repository) {
	for _, mutation := range []string{
		`UPDATE task_repositories SET metadata='{"pr_number":17}' WHERE id='adopt-attachment'`,
		`UPDATE task_repositories SET position=7 WHERE id='adopt-attachment'`,
		`UPDATE task_environment_repos SET branch_slug='foreign' WHERE id='adopt-slot'`,
		`UPDATE task_environment_repos SET position=7 WHERE id='adopt-slot'`,
		`INSERT INTO task_repositories(id,task_id,repository_id,checkout_branch,position,created_at,updated_at) VALUES('new-attachment','adopt-task','adopt-repo','feature/new',3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
	} {
		t.Run(mutation, func(t *testing.T) {
			r := newRepository(t)
			p := seedWorkspaceAdoptionDB(t, r)
			claim := acquireWorkspaceAdoptionClaim(t, r, p)
			ctx := context.Background()
			receipt, err := r.CommitWorkspaceAdoption(ctx, p, claim.OperationID, claim, func() error { return nil })
			require.NoError(t, err)
			require.NoError(t, r.FinalizeWorkspaceAdoption(ctx, receipt))
			require.NoError(t, r.ValidateWorkspaceAdoption(ctx, receipt))
			_, err = r.db.Exec(mutation)
			require.NoError(t, err)
			require.ErrorIs(t, r.ValidateWorkspaceAdoption(ctx, receipt), models.ErrWorkspaceAdoptionStale)
			_, err = checkoutadoption.ForWorktree(ctx, r.ro, "adopt-wt")
			require.ErrorIs(t, err, models.ErrWorkspaceAdoptionStale)
		})
	}
}

func TestWorkspaceAdoptionReceiptAuthoritySQLite(t *testing.T) {
	exerciseWorkspaceAdoptionReceiptAuthority(t, newRepoForEntityTests)
}

func TestWorkspaceAdoptionReceiptAuthorityPostgres(t *testing.T) {
	exerciseWorkspaceAdoptionReceiptAuthority(t, func(t *testing.T) *Repository {
		r, _, _ := newTaskPostgresRepoPair(t)
		return r
	})
}
