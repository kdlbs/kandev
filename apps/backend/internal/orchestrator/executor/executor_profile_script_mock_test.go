package executor

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func (*mockRepository) UpdateExecutorProfileWithScriptIntent(context.Context, *models.ExecutorProfile, models.ExecutorProfileScriptIntent) error {
	return fmt.Errorf("executor profile script updates are unsupported by the executor fixture")
}
