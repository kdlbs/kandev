package coordinator

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
)

// mergeSettle describes one settlement of a merge proposal.
type mergeSettle struct {
	to      ProposalStatus
	code    string
	outcome ActivityOutcome
	detail  string
	// dispatch, when set, is the outcome that settles the dispatch record.
	dispatch string
	commit   string
	// lower marks the dispatch for a policy lowering in the same transaction.
	lower bool
	// from are the statuses the settlement may start from; fenced limits the
	// approving one to this run's claim.
	from   []ProposalStatus
	fenced bool
}

var settleFromClaim = []ProposalStatus{ProposalStatusApproving, ProposalStatusUncertain}

func (r *mergeRun) row(outcome ActivityOutcome, code, detail string) ActivityRow {
	row := ActivityRow{
		CoordinatorID: r.p.CoordinatorID, WorkspaceID: r.p.WorkspaceID, ActionClass: ActionMerge, Outcome: outcome,
		Authorization: AuthRequiresApproval, ReasonCode: optString(code), TargetTaskID: r.p.TargetTaskID,
		ProposalID: &r.p.ID, Detail: truncateRunes(r.m.ChangeKey+" "+detail, 400),
	}
	if r.automatic {
		row.Authorization = AuthAutomatic
		row.UnattendedTurnID = automaticTurnFrom(context.Background())
	} else {
		row.ActorUserID = optString(r.claim.ApprovedBy)
	}
	return row
}

func (r *mergeRun) outcomeJSON(decision, result, commit string) string {
	b, _ := json.Marshal(map[string]string{
		"change_key": r.m.ChangeKey, "head_sha": r.m.HeadSHA, "method": r.m.Method,
		"decision": decision, "result": result, "merge_commit": commit,
	})
	return string(b)
}

// settle applies st in one coordinator-locked transaction. A settlement that
// lost to another path records a note when it carried a provider answer.
func (r *mergeRun) settle(ctx context.Context, st mergeSettle) bool {
	s := r.svc
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleBound)
	defer cancel()
	from := st.from
	if from == nil {
		from = []ProposalStatus{ProposalStatusApproving}
	}
	row := r.row(st.outcome, st.code, st.detail)
	matched, err := s.settleDecision(sctx, r.p.CoordinatorID, row, func(tx coordinatorExec) (bool, error) {
		tr := mergeTransition{ID: r.p.ID, From: from, To: st.to, Code: st.code, OutcomeJSON: r.outcomeJSON(string(st.to), st.code, st.commit)}
		if st.fenced {
			tr.Token = r.claim.Token
		}
		ok, err := s.store.transitionMergeTx(sctx, tx, tr, time.Now())
		if err != nil || !ok || st.dispatch == "" {
			return ok, err
		}
		return r.settleDispatchIn(sctx, tx, st)
	})
	if err != nil {
		s.logger.Error("merge: settlement failed", zap.String("proposal_id", r.p.ID), zap.String("to", string(st.to)), zap.Error(err))
		return false
	}
	if !matched {
		r.noteLate(sctx, st)
		return false
	}
	s.publishCoordinatorUpdated(sctx, r.p.WorkspaceID, r.p.CoordinatorID)
	if st.lower {
		s.applyPendingLowering(sctx, r.p.ID)
	}
	return true
}

func (r *mergeRun) settleDispatchIn(ctx context.Context, tx coordinatorExec, st mergeSettle) (bool, error) {
	s := r.svc
	ok, err := s.store.settleDispatchTx(ctx, tx, r.p.ID, st.dispatch, st.commit, time.Now())
	if err != nil || !ok {
		return false, err
	}
	if st.lower {
		if _, err := s.store.setLoweringTx(ctx, tx, r.p.ID, loweringNone, loweringPending); err != nil {
			return false, err
		}
	}
	return true, nil
}

// noteLate records a provider answer that arrived after another path settled
// the dispatch. It changes no status and lowers nothing.
func (r *mergeRun) noteLate(ctx context.Context, st mergeSettle) {
	if st.dispatch == "" {
		return
	}
	row := r.row(ActivityNote, "late_response", "provider answered after the dispatch was settled: "+st.code)
	err := r.svc.store.withCoordinatorLock(ctx, r.p.CoordinatorID, func(tx coordinatorExec) error { return r.svc.Record(ctx, tx, row) })
	if err != nil {
		r.svc.logger.Warn("merge: late response not recorded", zap.String("proposal_id", r.p.ID), zap.Error(err))
	}
}

// refuse settles the claimed proposal refused before any dispatch.
func (r *mergeRun) refuse(ctx context.Context, reason string) {
	r.settle(ctx, mergeSettle{to: ProposalStatusRejected, code: reason, outcome: ActivityRefused, detail: "refused: " + reason, fenced: true})
}

// settleDispatched settles a recorded dispatch refused.
func (r *mergeRun) settleDispatched(ctx context.Context, code, detail string, lower bool) {
	r.settle(ctx, mergeSettle{to: ProposalStatusRejected, code: code, outcome: ActivityRefused, detail: detail,
		dispatch: dispatchRefused, lower: lower, from: settleFromClaim, fenced: true})
}

func (r *mergeRun) settleMerged(ctx context.Context, commit string) {
	r.settle(ctx, mergeSettle{to: ProposalStatusApproved, outcome: ActivityApproved, detail: "merged " + commit,
		dispatch: dispatchMerged, commit: commit, from: settleFromClaim, fenced: true})
}

func (r *mergeRun) settleUncertain(ctx context.Context, code string) {
	r.settle(ctx, mergeSettle{to: ProposalStatusUncertain, code: code, outcome: ActivityUncertain, detail: "outcome not confirmed",
		from: []ProposalStatus{ProposalStatusApproving}, fenced: true})
}

// settleQueued records that the provider queued or deferred the merge: the
// outcome is not known.
func (r *mergeRun) settleQueued(ctx context.Context) {
	s := r.svc
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleBound)
	defer cancel()
	if err := s.store.withCoordinatorLock(sctx, r.p.CoordinatorID, func(tx coordinatorExec) error {
		return s.store.markDispatchQueued(sctx, tx, r.p.ID)
	}); err != nil {
		s.logger.Warn("merge: queued mark failed", zap.String("proposal_id", r.p.ID), zap.Error(err))
	}
	r.settleUncertain(ctx, "queued")
}

// release returns the claim to pending with no dispatch. A change of reason
// writes an activity row; force writes one regardless.
func (r *mergeRun) release(ctx context.Context, reason string, force bool) {
	s := r.svc
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleBound)
	defer cancel()
	changed := force || reason != r.m.LastReason
	err := s.store.withCoordinatorLock(sctx, r.p.CoordinatorID, func(tx coordinatorExec) error {
		ok, err := s.store.transitionMergeTx(sctx, tx, mergeTransition{ID: r.p.ID, From: []ProposalStatus{ProposalStatusApproving},
			Token: r.claim.Token, To: ProposalStatusPending}, time.Now())
		if err != nil || !ok {
			return err
		}
		if err := s.store.setMergeStateTx(sctx, tx, r.p.ID, r.m.ManagerOnly, r.m.Note, reason); err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return s.Record(sctx, tx, r.row(ActivityNote, reason, "waiting: "+reason))
	})
	if err != nil {
		s.logger.Warn("merge: claim not released", zap.String("proposal_id", r.p.ID), zap.Error(err))
	}
	s.publishCoordinatorUpdated(sctx, r.p.WorkspaceID, r.p.CoordinatorID)
}

// managerOnly returns the claim to pending and marks the proposal for a
// manager; it is never evaluated automatically again.
func (r *mergeRun) managerOnly(ctx context.Context, note, reason string) {
	s := r.svc
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleBound)
	defer cancel()
	err := s.store.withCoordinatorLock(sctx, r.p.CoordinatorID, func(tx coordinatorExec) error {
		ok, err := s.store.transitionMergeTx(sctx, tx, mergeTransition{ID: r.p.ID, From: []ProposalStatus{ProposalStatusApproving},
			Token: r.claim.Token, To: ProposalStatusPending}, time.Now())
		if err != nil || !ok {
			return err
		}
		if err := s.store.setMergeStateTx(sctx, tx, r.p.ID, true, note, reason); err != nil {
			return err
		}
		return s.Record(sctx, tx, r.row(ActivityNote, reason, note))
	})
	if err != nil {
		s.logger.Warn("merge: manager-only mark failed", zap.String("proposal_id", r.p.ID), zap.Error(err))
	}
	s.publishCoordinatorUpdated(sctx, r.p.WorkspaceID, r.p.CoordinatorID)
}

// settleStaleMerge recovers a stale approving claim as a status write: back to
// pending when no dispatch was recorded, uncertain when one was. It never
// fails the proposal and calls no provider.
func (s *Service) settleStaleMerge(ctx context.Context, current *Proposal, cutoff time.Time) (*Proposal, error) {
	matched := false
	err := s.store.withCoordinatorLock(ctx, current.CoordinatorID, func(tx coordinatorExec) error {
		d, err := s.store.getDispatch(ctx, tx, current.ID)
		if err != nil {
			return err
		}
		to, code, outcome := ProposalStatusPending, "claim_recovered", ActivityNote
		if d != nil && d.SettledAt == nil {
			to, outcome = ProposalStatusUncertain, ActivityUncertain
		}
		matched, err = s.store.transitionMergeTx(ctx, tx, mergeTransition{ID: current.ID, From: []ProposalStatus{ProposalStatusApproving},
			ClaimedBefore: &cutoff, To: to, Code: staleCode(to, code)}, time.Now())
		if err != nil || !matched {
			return err
		}
		return s.Record(ctx, tx, ActivityRow{
			CoordinatorID: current.CoordinatorID, WorkspaceID: current.WorkspaceID, ActionClass: ActionMerge, Outcome: outcome,
			Authorization: AuthRequiresApproval, ReasonCode: optString(code), TargetTaskID: current.TargetTaskID,
			ProposalID: &current.ID, Detail: "stale claim recovered as " + string(to),
		})
	})
	if err != nil {
		return nil, err
	}
	latest, err := s.store.GetProposalAnyKind(ctx, current.WorkspaceID, current.CoordinatorID, current.ID)
	if err != nil {
		return nil, err
	}
	if !matched {
		return nil, &ProposalConflictError{Proposal: latest}
	}
	s.publishCoordinatorUpdated(ctx, current.WorkspaceID, current.CoordinatorID)
	return latest, nil
}

// staleCode is the error text a recovered claim stores: none for a return to
// pending, the reason the outcome is unknown otherwise.
func staleCode(to ProposalStatus, code string) string {
	if to == ProposalStatusPending {
		return ""
	}
	return outcomeUnknownError
}
