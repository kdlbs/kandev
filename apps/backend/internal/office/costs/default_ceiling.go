package costs

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// DefaultCeilingSubcents is the built-in default ceiling's shipped value:
// 500,000 subcents (50.00 USD) of priced spend per workspace per UTC day
// (AC-OFFICE-BUDGET-003.2), in force for any workspace with no written
// override (AC-OFFICE-BUDGET-003.9).
const DefaultCeilingSubcents int64 = 500_000

// GetWorkspaceBudgetDefault returns workspaceID's effective built-in default
// ceiling: the written override if one exists, otherwise the shipped
// constant. Reading the effective value never reports the ceiling as absent
// (AC-OFFICE-BUDGET-003.9).
func (s *CostService) GetWorkspaceBudgetDefault(ctx context.Context, workspaceID string) (int64, error) {
	limitSubcents, found, err := s.repo.GetWorkspaceBudgetDefault(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	if !found {
		return DefaultCeilingSubcents, nil
	}
	return limitSubcents, nil
}

// SetWorkspaceBudgetDefault writes workspaceID's built-in default ceiling.
// A non-positive limit is rejected and the previous effective value stays
// in force (AC-OFFICE-BUDGET-003.8).
func (s *CostService) SetWorkspaceBudgetDefault(ctx context.Context, workspaceID string, limitSubcents int64) error {
	if limitSubcents <= 0 {
		return fmt.Errorf("%w: limit_subcents must be positive, got %d", ErrInvalidBudgetPolicy, limitSubcents)
	}
	return s.repo.SetWorkspaceBudgetDefault(ctx, workspaceID, limitSubcents)
}

// EvaluateDefaultCeiling evaluates workspaceID's built-in default ceiling as
// though it were a workspace-scoped daily block_new_tasks policy
// (AC-OFFICE-BUDGET-003.11), using the same SpendWindowForWorkspace query
// gate 4's workspace-scoped policies use so a policy and the default can
// never disagree about which events count in the same window. Callers only
// invoke this for an unattended run (REQ-OFFICE-BUDGET-003's gate 5 is only
// reached after AC-OFFICE-BUDGET-001.6 has already let an attended run
// through), so the degradation test is evaluated as unattended here rather
// than taking a provenance parameter that every caller would pass the same
// value for.
func (s *CostService) EvaluateDefaultCeiling(ctx context.Context, workspaceID string, at time.Time) (PreLaunchPolicyResult, error) {
	limitSubcents, err := s.GetWorkspaceBudgetDefault(ctx, workspaceID)
	if err != nil {
		return PreLaunchPolicyResult{}, err
	}

	start, _ := windowStart(models.BudgetPeriodDaily, at)
	window, err := s.repo.SpendWindowForWorkspace(ctx, workspaceID, start, true, at)
	if err != nil {
		return PreLaunchPolicyResult{}, fmt.Errorf("spend window for default ceiling: %w", err)
	}

	return PreLaunchPolicyResult{
		IsDefault:      true,
		ScopeType:      models.BudgetScopeWorkspace,
		Period:         models.BudgetPeriodDaily,
		ActionOnExceed: models.BudgetActionBlockNewTasks,
		LimitSubcents:  limitSubcents,
		PricedSubcents: window.PricedSubcents,
		Degraded:       window.Degraded,
		LimitExceeded:  window.PricedSubcents >= limitSubcents,
		DegradationBlocked: degradationBlocks(
			window.Degraded, window.PricedSubcents, limitSubcents, shared.RunProvenanceUnattended,
		),
	}, nil
}
