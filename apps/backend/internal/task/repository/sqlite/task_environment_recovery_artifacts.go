package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/recoveryartifact"
)

func (r *Repository) RegisterTaskEnvironmentRecoveryArtifacts(
	ctx context.Context,
	registration recoveryartifact.Registration,
) error {
	return recoveryartifact.Register(ctx, r.db, registration)
}

func (r *Repository) ListTaskEnvironmentRecoveryArtifacts(
	ctx context.Context,
	environmentID string,
) ([]recoveryartifact.Registered, error) {
	return recoveryartifact.ListForEnvironment(ctx, r.db, environmentID)
}
