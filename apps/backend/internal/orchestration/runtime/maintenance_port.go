package runtime

import (
	"context"

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
}

type MaintenanceScopeReader interface {
	ValidateMaintenanceScope(context.Context, string, models.MaintenanceScope) (string, error)
}
