package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/runs/models"
)

type capacityClaimContextKey struct{}

// SetClaimCapacityCandidates installs the existing common reservation owner
// before schedulers start. No callback runs inside the writer transaction.
func (r *Repository) SetClaimCapacityCandidates(read func(context.Context) ([]string, error)) {
	r.capacityClaimCandidates = read
}

func (r *Repository) prepareCapacityClaim(ctx context.Context) (context.Context, error) {
	if r.capacityClaimCandidates == nil {
		return ctx, nil
	}
	ids, err := r.capacityClaimCandidates(ctx)
	if err != nil || ids == nil {
		return ctx, err
	}
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	return context.WithValue(ctx, capacityClaimContextKey{}, allowed), nil
}

func capacityClaimAllows(ctx context.Context, run *models.Run) bool {
	// Task-bound Office runs keep their existing task-start owner. This bridge
	// covers taskless run sessions and cannot grant a synthetic task identity.
	if models.PayloadTaskID(run.Payload) != "" {
		return true
	}
	allowed, enabled := ctx.Value(capacityClaimContextKey{}).(map[string]struct{})
	if !enabled {
		return true
	}
	_, ok := allowed[run.ID]
	return ok
}

func claimCapacityGates(candidate *models.Run) []string {
	gates := []string{gateAgentCeiling, gateWorkspaceCeiling, gateInstanceCeiling}
	if candidate.HumanRooted {
		return gates
	}
	if candidate.RoutineID != "" {
		gates = append(gates, gateRoutineBudget)
	}
	return append(gates, gateWorkspaceBudget)
}

func (r *Repository) tasklessClaimSnapshotEligible(ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits, window time.Time) (bool, error) {
	if models.PayloadTaskID(candidate.Payload) != "" {
		return false, nil
	}
	for _, gate := range claimCapacityGates(candidate) {
		blocked, err := r.readClaimGateSnapshot(ctx, tx, candidate, limits, window, gate)
		if err != nil {
			return false, err
		}
		if blocked {
			return false, nil
		}
	}
	return true, nil
}
