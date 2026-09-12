package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// AcquireTaskEnvironmentRecoveryClaim obtains the durable, environment-scoped
// authority used by host worktree recovery. The claim package owns the SQL
// transaction so every caller uses the same owner, cleanup, and consumer checks.
func (r *Repository) AcquireTaskEnvironmentRecoveryClaim(
	ctx context.Context,
	req models.TaskEnvironmentRecoveryClaimRequest,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	return recoveryclaim.Acquire(ctx, r.db, req)
}

// ReleaseTaskEnvironmentRecoveryClaim releases only the exact operation that
// acquired the environment authority.
func (r *Repository) ReleaseTaskEnvironmentRecoveryClaim(
	ctx context.Context,
	claim *models.TaskEnvironmentRecoveryClaim,
) error {
	return recoveryclaim.Release(ctx, r.db, claim)
}
