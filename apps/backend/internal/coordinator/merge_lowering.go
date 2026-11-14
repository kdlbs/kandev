package coordinator

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

const (
	auditPolicyLowered = "policy_lowered"
	loweringAttempts   = 2
)

// pendingLoweringTx reports whether a dispatch of the coordinator's repository
// still has a lowering to apply.
func (s *Store) pendingLoweringTx(ctx context.Context, q coordinatorExec, coordinatorID, repositoryID string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, s.db.Rebind(`SELECT COUNT(*) FROM coordinator_merge_dispatches d
		JOIN coordinator_merge_proposals m ON m.proposal_id = d.proposal_id
		WHERE m.coordinator_id = ? AND m.repository_id = ? AND d.lowering = ?`), coordinatorID, repositoryID, loweringPending).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("read pending lowering: %w", err)
	}
	return n > 0, nil
}

// pendingLowerings lists the proposals whose dispatch has a lowering to apply.
func (s *Store) pendingLowerings(ctx context.Context, q coordinatorExec) ([]string, error) {
	rows, err := q.QueryContext(ctx, s.db.Rebind(`SELECT proposal_id FROM coordinator_merge_dispatches WHERE lowering = ? ORDER BY dispatched_at, proposal_id`), loweringPending)
	if err != nil {
		return nil, fmt.Errorf("list pending lowerings: %w", err)
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

// dailyMergeCountTx counts the automatic claims of the last 24 hours: the
// other classes' decided proposals plus the merge dispatches, which count at
// the dispatch record. Every automatic class shares this one budget.
func (s *Service) dailyMergeCountTx(ctx context.Context, tx coordinatorExec, coordinatorID string) (int, error) {
	return s.store.CountAutomaticDecidedTx(ctx, tx, coordinatorID, s.store.now().UTC().Add(-automaticLimitWindow))
}

// applyPendingLowering lowers the repository of a provider-rejected automatic
// dispatch, retrying once on contention. A failure leaves the lowering pending
// on the dispatch for the backstop.
func (s *Service) applyPendingLowering(ctx context.Context, proposalID string) {
	var err error
	for i := 0; i < loweringAttempts; i++ {
		if err = s.lowerForDispatch(ctx, proposalID); err == nil {
			return
		}
	}
	s.logger.Warn("merge: lowering stays pending", zap.String("proposal_id", proposalID), zap.Error(err))
}

// lowerForDispatch is a value compare under the coordinator lock: it lowers
// only a repository still automatic at the revision the dispatch recorded, and
// settles the pending marker either way so it applies at most once.
func (s *Service) lowerForDispatch(ctx context.Context, proposalID string) error {
	m, err := s.store.getMergeProposal(ctx, s.store.ro, proposalID)
	if err != nil {
		return err
	}
	return s.store.withCoordinatorLock(ctx, m.CoordinatorID, func(tx coordinatorExec) error {
		d, err := s.store.getDispatch(ctx, tx, proposalID)
		if err != nil || d == nil || d.Lowering != loweringPending {
			return err
		}
		policies, err := s.store.loadRepoPolicies(ctx, tx, m.CoordinatorID)
		if err != nil {
			return err
		}
		row := policies[m.RepositoryID]
		if row.Policy != RepoAutomatic || row.AutomaticSinceRevision != d.AutomaticSinceRevision {
			_, err := s.store.setLoweringTx(ctx, tx, proposalID, loweringPending, loweringSkipped)
			return err
		}
		var revision int
		if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`UPDATE coordinators SET policy_revision = policy_revision + 1, updated_at = ?
			WHERE id = ? RETURNING policy_revision`), s.store.now().UTC(), m.CoordinatorID).Scan(&revision); err != nil {
			return fmt.Errorf("bump policy revision: %w", err)
		}
		plan := policyPlan{
			Rows:   []RepoPolicyRow{{RepositoryID: m.RepositoryID, Policy: RepoRequiresApproval, SelectedBy: row.SelectedBy}},
			Audits: []PolicyAudit{{Kind: auditPolicyLowered, Subject: m.RepositoryID, Before: string(RepoAutomatic), After: string(RepoRequiresApproval), Revision: revision}},
		}
		if err := s.store.writeRepoPolicyPlan(ctx, tx, m.CoordinatorID, "", plan); err != nil {
			return err
		}
		_, err = s.store.setLoweringTx(ctx, tx, proposalID, loweringPending, loweringDone)
		return err
	})
}

// retryPendingLowerings applies every pending lowering; the backstop calls it
// on each pass.
func (s *Service) retryPendingLowerings(ctx context.Context) {
	ids, err := s.store.pendingLowerings(ctx, s.store.ro)
	if err != nil {
		s.logger.Warn("merge: pending lowerings unreadable", zap.Error(err))
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		s.applyPendingLowering(ctx, id)
	}
}
