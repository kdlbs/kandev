package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func (*mockRepository) UpdateExecutorProfileWithScriptIntent(context.Context, *models.ExecutorProfile, models.ExecutorProfileScriptIntent) error {
	return fmt.Errorf("executor profile script updates are unsupported by the process fixture")
}

func (r *executorRepo) UpdateExecutorProfileWithScriptIntent(ctx context.Context, profile *models.ExecutorProfile, intent models.ExecutorProfileScriptIntent) error {
	current, err := r.GetExecutorProfile(ctx, profile.ID)
	if err != nil {
		return err
	}
	if intent.PrepareScript == nil {
		profile.PrepareScript = current.PrepareScript
	} else {
		profile.PrepareScript = *intent.PrepareScript
	}
	if intent.CleanupScript == nil {
		profile.CleanupScript = current.CleanupScript
	} else {
		profile.CleanupScript = *intent.CleanupScript
	}
	return r.UpdateExecutorProfile(ctx, profile)
}
