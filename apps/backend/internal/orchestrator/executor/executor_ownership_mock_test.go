package executor

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *mockRepository) DeleteExecutorIfOwnershipUnmodified(ctx context.Context, id, owner string) error {
	return m.DeleteExecutor(ctx, id)
}
func (m *mockRepository) DeleteExecutorProfileIfOwnershipUnmodified(ctx context.Context, profile *models.ExecutorProfile, parentOwner *string) error {
	return m.DeleteExecutorProfile(ctx, profile.ID)
}
