package coordinator

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// CoordinatorIDsWithMergeWork lists the coordinators holding an open merge
// proposal or a dispatch whose lowering is still pending, ordered by id.
func (s *Store) CoordinatorIDsWithMergeWork(ctx context.Context) ([]string, error) {
	rows, err := s.ro.QueryContext(ctx, s.db.Rebind(`SELECT DISTINCT m.coordinator_id FROM coordinator_merge_proposals m
		JOIN coordinator_proposals p ON p.id = m.proposal_id
		LEFT JOIN coordinator_merge_dispatches d ON d.proposal_id = m.proposal_id AND d.lowering = ?
		WHERE p.status IN (`+mergeOpenStatuses+`) OR d.proposal_id IS NOT NULL
		ORDER BY m.coordinator_id`), loweringPending)
	if err != nil {
		return nil, fmt.Errorf("list coordinators with merge work: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RunMergeDuties is the backstop's merge duty for one coordinator. It runs
// before the autonomy gate: pending lowerings are applied, every pending
// proposal that can no longer be merged is settled, and the automatic ones are
// rechecked and dispatched. Nothing runs while the merge flag is off.
func (s *Service) RunMergeDuties(ctx context.Context, coordinatorID string) error {
	if !s.merge {
		return nil
	}
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return err
	}
	s.retryPendingLowerings(ctx)
	open, err := s.store.listOpenMerges(ctx, s.store.ro, coordinatorID)
	if err != nil {
		return err
	}
	for _, m := range open {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.dutyOne(ctx, c, m)
	}
	return nil
}

func (s *Service) dutyOne(ctx context.Context, c *Coordinator, m *MergeProposal) {
	p, err := s.store.GetProposalAnyKind(ctx, c.WorkspaceID, c.ID, m.ProposalID)
	if err != nil {
		s.logger.Warn("merge: proposal unreadable", zap.String("proposal_id", m.ProposalID), zap.Error(err))
		return
	}
	if p.Status != ProposalStatusPending {
		return
	}
	if s.sweepPendingMerge(ctx, c, p, m) {
		return
	}
	if c.AutonomyEnabled && m.AutomaticRevision != nil {
		s.evaluateAutomaticMerge(ctx, c, p, m, true)
	}
}
