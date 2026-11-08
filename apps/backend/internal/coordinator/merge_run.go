package coordinator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// Notes a manager-only merge proposal shows.
const (
	noteDailyLimit   = "Daily limit reached"
	noteNotContained = "Not contained"
	noteNotAdmitted  = "Not admitted"
	notePolicyChange = "Policy changed; a manager will decide"
	noteLowering     = "Lowering automatic merge policy is pending"
)

// mergeRun is one claimed merge attempt.
type mergeRun struct {
	svc       *Service
	claim     Claim
	c         *Coordinator
	p         *Proposal
	m         *MergeProposal
	automatic bool
	taskID    string
}

type dispatchVerdict int

const (
	dispatchedNow dispatchVerdict = iota
	dispatchDepsChanged
	dispatchStopped
)

func (s *Service) newMergeRun(ctx context.Context, claim Claim) (*mergeRun, error) {
	p, err := s.store.GetProposalAnyKind(ctx, claim.WorkspaceID, claim.Coordinator.ID, claim.ProposalID)
	if err != nil {
		return nil, err
	}
	m, err := s.store.getMergeProposal(ctx, s.store.ro, claim.ProposalID)
	if err != nil {
		return nil, err
	}
	return &mergeRun{svc: s, claim: claim, c: claim.Coordinator, p: p, m: m,
		automatic: mergeAutomaticFrom(ctx), taskID: claim.TargetTaskID}, nil
}

// runMerge performs a claimed merge: the ordered recheck, the dispatch record
// and the one provider request, settling the proposal at every exit. Settling
// uses a context that outlives the execute deadline.
func (s *Service) runMerge(ctx context.Context, claim Claim) {
	r, err := s.newMergeRun(ctx, claim)
	if err != nil {
		s.logger.Error("merge: claimed proposal unreadable", zap.String("proposal_id", claim.ProposalID), zap.Error(err))
		return
	}
	for attempt := 0; attempt < 2; attempt++ {
		reading, err := s.readMerge(ctx, r.c, r.m, r.taskID, r.automatic)
		if err != nil {
			s.logger.Warn("merge: recheck unreadable", zap.String("proposal_id", r.p.ID), zap.Error(err))
			r.release(ctx, "unavailable:read", false)
			return
		}
		if r.stopOn(ctx, reading.Decision) {
			return
		}
		switch r.dispatch(ctx, reading.DepRevision) {
		case dispatchedNow:
			r.send(ctx)
			return
		case dispatchStopped:
			return
		}
	}
	r.release(ctx, string(mergestate.CondPredecessor), true)
}

// stopOn settles a recheck that did not clear the merge and reports whether it
// did. A waiting reason refuses a manager's approval.
func (r *mergeRun) stopOn(ctx context.Context, d mergestate.Decision) bool {
	switch d.ForAttempt(r.automatic) {
	case mergestate.Proceed:
		return false
	case mergestate.Wait:
		r.release(ctx, d.Reason, false)
	case mergestate.ManagerOnly:
		r.managerOnly(ctx, notePausedMerge, d.Reason)
	default:
		r.refuse(ctx, d.Reason)
	}
	return true
}

const notePausedMerge = pausedNote

// dispatch inserts the dispatch record under the coordinator lock after the
// last checks that can stop it: the claim generation, the flags and policy
// read again, the dependency revision, a pending lowering and the daily limit.
func (r *mergeRun) dispatch(ctx context.Context, depRevision int) dispatchVerdict {
	s := r.svc
	if r.automatic {
		if note := s.mergeAdmission(ctx, r.c.ID); note != "" {
			r.managerOnly(ctx, note, "admission")
			return dispatchStopped
		}
	}
	verdict, note, reason := dispatchStopped, "", ""
	err := s.store.withCoordinatorLock(ctx, r.c.ID, func(tx coordinatorExec) error {
		var err error
		verdict, note, reason, err = r.dispatchLocked(ctx, tx, depRevision)
		return err
	})
	switch {
	case err != nil:
		s.logger.Warn("merge: dispatch not recorded", zap.String("proposal_id", r.p.ID), zap.Error(err))
		r.release(ctx, "unavailable:dispatch", false)
		return dispatchStopped
	case verdict == dispatchStopped && note != "":
		r.managerOnly(ctx, note, reason)
	case verdict == dispatchStopped && reason != "":
		r.settleStopped(ctx, reason)
	}
	return verdict
}

// settleStopped settles a dispatch that the locked section stopped: an
// authority loss refuses, anything else waits.
func (r *mergeRun) settleStopped(ctx context.Context, reason string) {
	if reason == refuseAuthority {
		r.refuse(ctx, reason)
		return
	}
	r.release(ctx, reason, false)
}

func (r *mergeRun) dispatchLocked(ctx context.Context, tx coordinatorExec, depRevision int) (dispatchVerdict, string, string, error) {
	s := r.svc
	var status, token string
	if err := tx.QueryRowContext(ctx, s.store.db.Rebind(`SELECT status, COALESCE(claim_token, '') FROM coordinator_proposals WHERE id = ?`), r.p.ID).Scan(&status, &token); err != nil {
		return dispatchStopped, "", "", err
	}
	if status != string(ProposalStatusApproving) || token != r.claim.Token {
		return dispatchStopped, "", "", nil
	}
	if !s.merge {
		return dispatchStopped, "", "flag_off", nil
	}
	policies, err := s.store.loadRepoPolicies(ctx, tx, r.c.ID)
	if err != nil {
		return dispatchStopped, "", "", err
	}
	row := policies[r.m.RepositoryID]
	if row.Policy == "" || row.Policy == RepoDenied || (r.automatic && row.Policy != RepoAutomatic) {
		return dispatchStopped, "", refuseAuthority, nil
	}
	sets, err := s.store.loadDepSets(ctx, tx, r.c.ID)
	if err != nil {
		return dispatchStopped, "", "", err
	}
	if sets[r.taskID].Revision != depRevision {
		return dispatchDepsChanged, "", "", nil
	}
	if r.automatic {
		if note, err := r.automaticGate(ctx, tx); err != nil || note != "" {
			return dispatchStopped, note, "gate", err
		}
	}
	err = s.store.insertDispatchTx(ctx, tx, MergeDispatch{
		ProposalID: r.p.ID, WorkspaceID: r.p.WorkspaceID, ChangeKey: r.m.ChangeKey, HeadSHA: r.m.HeadSHA,
		ClaimToken: r.claim.Token, Automatic: r.automatic, AutomaticSinceRevision: row.AutomaticSinceRevision,
	})
	return dispatchedNow, "", "", err
}

// automaticGate returns the note of the first thing that stops an automatic
// dispatch under the lock: a pending lowering of the repository or the daily
// limit counted at dispatch.
func (r *mergeRun) automaticGate(ctx context.Context, tx coordinatorExec) (string, error) {
	s := r.svc
	pending, err := s.store.pendingLoweringTx(ctx, tx, r.c.ID, r.m.RepositoryID)
	if err != nil || pending {
		return noteLowering, err
	}
	used, err := s.dailyMergeCountTx(ctx, tx, r.c.ID)
	if err != nil {
		return "", err
	}
	if used >= s.policyFor(r.c).EffectiveEligibility().DailyLimit {
		return noteDailyLimit, nil
	}
	return "", nil
}

// mergeAdmission returns the note of the phase 3 containment or admission
// check that holds an automatic merge, or "".
func (s *Service) mergeAdmission(ctx context.Context, coordinatorID string) string {
	adm := s.AdmitAction(ctx, coordinatorID)
	switch {
	case adm.OK:
		return ""
	case adm.Reason == admitContainment:
		return noteNotContained
	}
	return noteNotAdmitted
}

// send makes the one provider request and settles on its answer. A request
// that gave no answer is uncertain: it may have been accepted.
func (r *mergeRun) send(ctx context.Context) {
	key, _ := mergestate.ParseChangeKey(r.m.ChangeKey)
	merger := r.svc.changeMerger()
	if merger == nil {
		r.settleUncertain(ctx, "no_provider")
		return
	}
	res, err := merger.Merge(ctx, r.p.WorkspaceID, key, r.m.HeadSHA, r.m.Method)
	switch {
	case err != nil:
		r.svc.logger.Warn("merge: no provider answer", zap.String("proposal_id", r.p.ID), zap.Error(err))
		r.settleUncertain(ctx, "no_answer")
	case res.Outcome == MergeAccepted:
		r.settleMerged(ctx, res.MergeCommit)
	case res.Outcome == MergeStaleHead:
		r.settleDispatched(ctx, refuseStaleHead, "provider reports the head changed", false)
	case res.Outcome == MergeQueued:
		r.settleQueued(ctx)
	default:
		r.settleDispatched(ctx, refuseProviderReject, truncateRunes(res.Reason, 200), r.automatic)
	}
}
