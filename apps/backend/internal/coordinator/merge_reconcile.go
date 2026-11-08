package coordinator

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// Reconciliation and revert-watch bounds.
const (
	reconcileMinAge  = 10 * time.Minute
	unconfirmedAfter = 24 * time.Hour
	revertWindow     = 7 * 24 * time.Hour
	revertInterval   = 15 * time.Minute
	revertPages      = 3
	revertPageSize   = 100
)

const reasonUnconfirmed = "unconfirmed"

// reconcileRun is the settlement context of a proposal outside a claim.
func (s *Service) reconcileRun(ctx context.Context, c *Coordinator, p *Proposal, m *MergeProposal, d *MergeDispatch) *mergeRun {
	return &mergeRun{svc: s, c: c, p: p, m: m, automatic: d.Automatic, claim: Claim{ApprovedBy: decidingUserID(ctx)}}
}

// reconcileMerge reads an uncertain merge no sooner than reconcileMinAge after
// its dispatch record and settles it from what the provider reports. A read
// that fails, or an answer that does not settle it, leaves it uncertain. It
// reports whether it settled the proposal.
func (s *Service) reconcileMerge(ctx context.Context, c *Coordinator, p *Proposal, m *MergeProposal, _ bool) bool {
	if !s.merge {
		return false
	}
	d, err := s.store.getDispatch(ctx, s.store.ro, p.ID)
	if err != nil || d == nil || d.SettledAt != nil || s.store.now().Sub(d.DispatchedAt) < reconcileMinAge {
		return false
	}
	merger := s.changeMerger()
	key, ok := mergestate.ParseChangeKey(m.ChangeKey)
	if merger == nil || !ok {
		return false
	}
	snap, err := merger.ReadLive(ctx, c.WorkspaceID, key)
	if err != nil {
		return false
	}
	run := s.reconcileRun(ctx, c, p, m, d)
	st, settles := reconciledSettlement(snap, m, d)
	if !settles {
		return false
	}
	st.from = []ProposalStatus{ProposalStatusUncertain}
	return run.settle(ctx, st)
}

// reconciledSettlement maps a provider reading of the change to the
// settlement of its uncertain dispatch.
func reconciledSettlement(snap mergestate.Snapshot, m *MergeProposal, d *MergeDispatch) (mergeSettle, bool) {
	switch snap.State {
	case mergestate.StateMerged:
		if snap.MergedHeadSHA == m.HeadSHA {
			return mergeSettle{to: ProposalStatusApproved, outcome: ActivityApproved, detail: "merged " + snap.MergeCommit,
				dispatch: dispatchMerged, commit: snap.MergeCommit}, true
		}
		return mergeSettle{to: ProposalStatusRejected, code: refuseMergedElsewhere, outcome: ActivityRefused,
			detail: "merged from another head", dispatch: dispatchRefused}, true
	case mergestate.StateClosed:
		return mergeSettle{to: ProposalStatusRejected, code: refuseClosed, outcome: ActivityRefused,
			detail: "closed without merging", dispatch: dispatchRefused}, true
	case mergestate.StateOpen:
		if d.Queued || snap.InProgress {
			return mergeSettle{}, false
		}
		return mergeSettle{to: ProposalStatusRejected, code: refuseNotMerged, outcome: ActivityRefused,
			detail: "the request did not merge the change", dispatch: dispatchRefused, lower: d.Automatic}, true
	}
	return mergeSettle{}, false
}

func (s *Service) loadUncertainMerge(ctx context.Context, workspaceID, coordinatorID, proposalID string) (*Coordinator, *Proposal, *MergeProposal, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, nil, nil, err
	}
	if !s.merge {
		return nil, nil, nil, errMergeUnavailable()
	}
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return nil, nil, nil, err
	}
	p, err := s.store.GetProposalAnyKind(ctx, workspaceID, coordinatorID, proposalID)
	if err != nil {
		return nil, nil, nil, err
	}
	if p.Kind != ProposalKindMerge || p.Status != ProposalStatusUncertain {
		return nil, nil, nil, &ProposalConflictError{Proposal: p}
	}
	m, err := s.store.getMergeProposal(ctx, s.store.ro, proposalID)
	if err != nil {
		return nil, nil, nil, err
	}
	return c, p, m, nil
}

// CheckMergeNow reconciles one uncertain merge proposal on a manager's
// request, under the same minimum age as the backstop.
func (s *Service) CheckMergeNow(ctx context.Context, workspaceID, coordinatorID, proposalID string) (*Proposal, error) {
	c, p, m, err := s.loadUncertainMerge(ctx, workspaceID, coordinatorID, proposalID)
	if err != nil {
		return nil, err
	}
	s.reconcileMerge(ctx, c, p, m, true)
	return s.store.GetProposalAnyKind(ctx, workspaceID, coordinatorID, proposalID)
}

// ReleaseMerge lets a manager give up on an uncertain merge that stayed
// unconfirmed for more than 24 hours: it settles refused (unconfirmed) with no
// provider request and no lowering.
func (s *Service) ReleaseMerge(ctx context.Context, workspaceID, coordinatorID, proposalID string) (*Proposal, error) {
	c, p, m, err := s.loadUncertainMerge(ctx, workspaceID, coordinatorID, proposalID)
	if err != nil {
		return nil, err
	}
	d, err := s.store.getDispatch(ctx, s.store.ro, p.ID)
	if err != nil {
		return nil, err
	}
	if d == nil || d.SettledAt != nil || s.store.now().Sub(d.DispatchedAt) <= unconfirmedAfter {
		return nil, &ProposalConflictError{Proposal: p}
	}
	run := s.reconcileRun(ctx, c, p, m, d)
	run.settle(ctx, mergeSettle{to: ProposalStatusRejected, code: reasonUnconfirmed, outcome: ActivityRefused,
		detail: "released by a manager without confirmation", dispatch: dispatchReleased, from: []ProposalStatus{ProposalStatusUncertain}})
	return s.store.GetProposalAnyKind(ctx, workspaceID, coordinatorID, proposalID)
}

// scanReverts looks for a revert of each automatic merge still inside the
// window: up to revertPages pages of revertPageSize commit messages after the
// merge commit. A revert lowers the repository's policy; a read that fails
// lowers nothing and is retried at the next interval.
func (s *Service) scanReverts(ctx context.Context, c *Coordinator) {
	merger := s.changeMerger()
	if merger == nil {
		return
	}
	now := s.store.now()
	watches, err := s.store.revertWatches(ctx, s.store.ro, c.ID, now.Add(-revertWindow), now.Add(-revertInterval))
	if err != nil {
		s.logger.Warn("merge: revert watches unreadable", zap.Error(err))
		return
	}
	for _, d := range watches {
		if ctx.Err() != nil {
			return
		}
		s.scanRevert(ctx, merger, c, d, now)
	}
}

func (s *Service) scanRevert(ctx context.Context, merger ChangeMerger, c *Coordinator, d *MergeDispatch, now time.Time) {
	key, ok := mergestate.ParseChangeKey(d.ChangeKey)
	if !ok || d.MergeCommit == "" {
		_ = s.store.touchRevertWatch(ctx, s.store.db, d.ProposalID, now, true)
		return
	}
	reverted, err := revertedWithin(ctx, merger, c.WorkspaceID, key, d.MergeCommit)
	if err != nil {
		s.logger.Warn("merge: revert scan unavailable", zap.String("proposal_id", d.ProposalID), zap.Error(err))
		_ = s.store.touchRevertWatch(ctx, s.store.db, d.ProposalID, now, false)
		return
	}
	if !reverted {
		_ = s.store.touchRevertWatch(ctx, s.store.db, d.ProposalID, now, false)
		return
	}
	err = s.store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
		if _, err := s.store.setLoweringTx(ctx, tx, d.ProposalID, loweringNone, loweringPending); err != nil {
			return err
		}
		return s.store.touchRevertWatch(ctx, tx, d.ProposalID, now, true)
	})
	if err != nil {
		s.logger.Warn("merge: revert not recorded", zap.String("proposal_id", d.ProposalID), zap.Error(err))
		return
	}
	s.applyPendingLowering(ctx, d.ProposalID)
}

// revertedWithin reports whether a commit after the merge commit reverts it.
func revertedWithin(ctx context.Context, merger ChangeMerger, workspaceID string, key mergestate.ChangeKey, mergeCommit string) (bool, error) {
	needle := "This reverts commit " + mergeCommit
	for page := 1; page <= revertPages; page++ {
		msgs, more, err := merger.CommitMessages(ctx, workspaceID, key, mergeCommit, page)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if strings.Contains(m, needle) {
				return true, nil
			}
		}
		if !more {
			break
		}
	}
	return false, nil
}
