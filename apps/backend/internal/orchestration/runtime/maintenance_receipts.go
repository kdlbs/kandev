package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/orchestration/maintenance"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (s *Service) checkMaintenance(ctx context.Context, b *models.AssistantBinding, g models.MaintenanceGrant, guard maintenance.Guard) (any, error) {
	v, err := s.Maintenance.Check(ctx, g, guard)
	if err != nil || len(v.Checks) != 2 {
		v.Passed = false
	}
	receipt, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if len(v.Checks) > 0 {
		if saveErr := s.Repo.SaveMaintenanceValidation(receipt, b.ID, g.CandidateID, v); saveErr != nil {
			return v, saveErr
		}
	}
	return v, err
}

func (s *Service) commitMaintenance(ctx context.Context, b *models.AssistantBinding, g models.MaintenanceGrant, guard maintenance.Guard) (any, error) {
	v, err := s.Repo.MaintenanceValidation(ctx, b.ID, g.CandidateID)
	if err != nil || !v.Passed || len(v.Checks) != 2 || v.GrantRevision != g.Revision {
		return nil, rejectOperation(422, "maintenance_checks_required")
	}
	artifact, err := s.Maintenance.Commit(ctx, g, guard)
	if errors.Is(err, maintenance.ErrBoundary) {
		return nil, rejectOperation(422, "maintenance_checks_required_for_current_tree")
	}
	if err != nil {
		return artifact, err
	}
	receipt, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if artifact.TreeOID != v.TreeOID || artifact.CommitOID == "" {
		return nil, models.ErrConflict
	}
	return artifact, s.Repo.RecordMaintenancePrepared(receipt, b, g.CandidateID, artifact)
}
