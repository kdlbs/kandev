package runtime

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/orchestration/maintenance"
	"github.com/kandev/kandev/internal/orchestration/models"
)

type MaintenanceSandbox interface {
	Inspect(context.Context, string, models.MaintenanceScope) (models.MaintenanceScope, string, error)
	Prepare(context.Context, string, models.MaintenanceGrant, maintenance.Guard) error
	Read(context.Context, models.MaintenanceGrant, string) (models.MaintenanceFile, error)
	Patch(context.Context, models.MaintenanceGrant, models.MaintenanceFile, maintenance.Guard) (models.MaintenanceArtifact, error)
	Check(context.Context, models.MaintenanceGrant, maintenance.Guard) (models.MaintenanceValidation, error)
	Commit(context.Context, models.MaintenanceGrant, maintenance.Guard) (models.MaintenanceArtifact, error)
	Review(context.Context, models.MaintenanceGrant) (models.MaintenanceReviewArtifact, error)
}

type MaintenanceReviewReader interface {
	ValidateMaintenanceReview(context.Context, string, string) error
	ValidateMaintenanceSuccess(context.Context, string, models.Evidence, time.Time) error
}

type MaintenanceScopeReader interface {
	ValidateMaintenanceScope(context.Context, string, models.MaintenanceScope) (string, error)
}

type MaintenanceOptionsReader interface {
	MaintenanceOptions(context.Context, string) ([]models.MaintenanceOption, error)
}

type MaintenanceSuccessFinder interface {
	FindMaintenanceSuccess(context.Context, string, string, string, time.Time) (*models.MaintenanceSuccess, error)
}
