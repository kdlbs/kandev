package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/runs/models"
)

func (r *Repository) readClaimGateSnapshot(ctx context.Context, tx *sqlx.Tx, candidate *models.Run, limits ClaimSafetyLimits, window time.Time, gate string) (bool, error) {
	switch gate {
	case gateAgentCeiling:
		cap, err := r.agentCeiling(ctx, tx, candidate.AgentProfileID, limits)
		if err != nil {
			return true, err
		}
		if cap <= 0 {
			return true, errors.New("agent claim ceiling unavailable")
		}
		count, err := r.countClaimed(ctx, tx, "agent_profile_id = ?", candidate.AgentProfileID)
		return count >= cap, err
	case gateWorkspaceCeiling:
		count, err := r.countClaimed(ctx, tx, "workspace_id = ?", candidate.WorkspaceID)
		return count >= limits.MaxConcurrentWorkspace, err
	case gateInstanceCeiling:
		count, err := r.countClaimed(ctx, tx, "1 = 1")
		return count >= limits.MaxConcurrentInstance, err
	case gateRoutineBudget:
		count, err := r.countLedger(ctx, tx, "routine_id = ? AND claimed_at > ?", candidate.RoutineID, window)
		return count >= limits.RoutineBudgetPerHour, err
	case gateWorkspaceBudget:
		count, err := r.countLedger(ctx, tx, "workspace_id = ? AND claimed_at > ?", candidate.WorkspaceID, window)
		return count >= limits.WorkspaceBudgetPerHour, err
	default:
		return true, fmt.Errorf("unknown claim gate %q", gate)
	}
}

// ListTasklessCapacityCandidates observes the existing Office queue and its
// real launch gates. It writes no claim, ledger, metrics or gate-failure state.
// The existing local priority/age order and scan bound are preserved.
func (r *Repository) ListTasklessCapacityCandidates(ctx context.Context) ([]models.Run, error) {
	return r.listCapacityCandidates(ctx, true)
}

// ListCapacityCandidates retains the original queue order and gates for both
// task-bound and taskless Office owners without claiming or rewriting a run.
func (r *Repository) ListCapacityCandidates(ctx context.Context) ([]models.Run, error) {
	return r.listCapacityCandidates(ctx, false)
}

func (r *Repository) listCapacityCandidates(ctx context.Context, tasklessOnly bool) ([]models.Run, error) {
	if r.ro == nil {
		return nil, errors.New("office queue reader unavailable")
	}
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	limits := r.effectiveClaimLimits()
	return r.readTasklessCapacityPages(ctx, tx, now, limits, tasklessOnly)
}

func (r *Repository) readTasklessCapacityPages(ctx context.Context, tx *sqlx.Tx, now time.Time, limits ClaimSafetyLimits, tasklessOnly bool) ([]models.Run, error) {
	const query = `SELECT w.* FROM runs w JOIN agent_profiles a ON a.id=w.agent_profile_id AND (w.workspace_id='' OR a.workspace_id=w.workspace_id)
	WHERE w.status='queued' AND (w.scheduled_retry_at IS NULL OR w.scheduled_retry_at <= ?)
	AND w.routing_blocked_status IS NULL AND COALESCE(a.status,'idle') NOT IN ('paused','stopped','pending_approval')
	ORDER BY CASE WHEN w.requested_at < ? AND w.priority_class > 1 THEN w.priority_class-1 ELSE w.priority_class END ASC,w.requested_at ASC,w.id ASC LIMIT ? OFFSET ?`
	eligibleRead := r.claimSnapshotEligible
	if tasklessOnly {
		eligibleRead = r.tasklessClaimSnapshotEligible
	}
	var result []models.Run
	for offset := 0; offset < claimCandidateScanCap; offset += claimCandidatePageSize {
		var page []models.Run
		if err := tx.SelectContext(ctx, &page, tx.Rebind(query), now, now.Add(-limits.PromotionAge), claimCandidatePageSize, offset); err != nil {
			return nil, err
		}
		for i := range page {
			eligible, err := eligibleRead(ctx, tx, &page[i], limits, now.Add(-BudgetWindow))
			if err != nil {
				return nil, err
			}
			if eligible {
				result = append(result, page[i])
			}
		}
		if len(page) < claimCandidatePageSize {
			return result, nil
		}
	}
	return nil, errors.New("office priority snapshot reached existing claim scan bound")
}
