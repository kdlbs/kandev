package coordinator

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// mergeExpiry is how long a merge proposal may stay pending.
const mergeExpiry = 7 * 24 * time.Hour

const (
	statusPendingMerge = "pending"
	reasonManagerOnly  = "manager_only"
)

func (s *Service) recordMergeProposed(ctx context.Context, tx coordinatorExec, p *Proposal, args mergeArgs, t *mergeTarget, turnID *string) error {
	id := p.ID
	return s.Record(ctx, tx, ActivityRow{
		CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ActionClass: ActionMerge, Outcome: ActivityProposed,
		Authorization: AuthRequiresApproval, TargetTaskID: &args.TaskID, ProposalID: &id,
		Detail: truncateRunes(t.key.String()+" head "+t.head+" via "+t.method, 200), UnattendedTurnID: turnID,
	})
}

// evaluateCreated evaluates a just-created proposal for the tool answer. It
// never dispatches.
func (s *Service) evaluateCreated(ctx context.Context, c *Coordinator, p *Proposal, m *MergeProposal) (*MergeProposeResult, error) {
	res := &MergeProposeResult{ProposalID: &p.ID, Status: statusPendingMerge}
	if m.AutomaticRevision == nil {
		return res, nil
	}
	status, reason := s.evaluateAutomaticMerge(ctx, c, p, m, false)
	res.Status, res.Reason = status, reason
	return res, nil
}

// evaluateAutomaticMerge applies the automatic recheck to a pending proposal:
// the stored policy, Pause and the change conditions. With dispatch it claims
// and runs a proposal whose conditions are all met. It returns the proposal's
// resulting status and reason.
func (s *Service) evaluateAutomaticMerge(ctx context.Context, c *Coordinator, p *Proposal, m *MergeProposal, dispatch bool) (string, string) {
	if m.ManagerOnly {
		return statusPendingMerge, reasonManagerOnly
	}
	policies, err := s.store.loadRepoPolicies(ctx, s.store.ro, c.ID)
	if err != nil {
		s.logger.Warn("merge: policy unreadable", zap.String("proposal_id", p.ID), zap.Error(err))
		return statusPendingMerge, ""
	}
	row := policies[m.RepositoryID]
	switch {
	case row.Policy == "" || row.Policy == RepoDenied:
		return s.refuseUnclaimed(ctx, p, m, refuseAuthority)
	case row.Policy != RepoAutomatic || m.AutomaticRevision == nil || row.AutomaticSinceRevision != *m.AutomaticRevision:
		s.markManagerOnly(ctx, p, m, notePolicyChange, reasonPolicyChanged)
		return statusPendingMerge, reasonManagerOnly
	}
	reading, err := s.readMerge(ctx, c, m, targetOf(p), true)
	if err != nil {
		s.logger.Warn("merge: recheck unreadable", zap.String("proposal_id", p.ID), zap.Error(err))
		return statusPendingMerge, ""
	}
	d := reading.Decision
	switch d.Outcome {
	case mergestate.Wait:
		s.noteWaiting(ctx, p, m, d.Reason)
		return statusPendingMerge, d.Reason
	case mergestate.Refuse:
		return s.refuseUnclaimed(ctx, p, m, d.Reason)
	case mergestate.ManagerOnly:
		s.markManagerOnly(ctx, p, m, pausedNote, d.Reason)
		return statusPendingMerge, reasonManagerOnly
	}
	if dispatch {
		return s.claimAndRunAutomatic(ctx, c, p, row)
	}
	return statusPendingMerge, ""
}

func targetOf(p *Proposal) string {
	if p.TargetTaskID == nil {
		return ""
	}
	return *p.TargetTaskID
}

// claimAndRunAutomatic claims the pending proposal with the automatic
// principal and runs the dispatch.
func (s *Service) claimAndRunAutomatic(ctx context.Context, c *Coordinator, p *Proposal, row RepoPolicyRow) (string, string) {
	exec := s.kindExecutor(ProposalKindMerge)
	token := uuid.NewString()
	matched, err := s.claimRow(ctx, p, token, p.RawSpec, row.SelectedBy)
	if err != nil || !matched {
		if err != nil {
			s.logger.Warn("merge: automatic claim failed", zap.String("proposal_id", p.ID), zap.Error(err))
		}
		return statusPendingMerge, ""
	}
	turnCtx := withMergeAutomatic(withAutomaticTurn(ctx, s.currentUnattendedTurn(ctx, c.ID)))
	done, err := s.runClaimedKind(turnCtx, exec, p, token, []byte(p.RawSpec), row.SelectedBy)
	if err != nil || done == nil {
		return statusPendingMerge, ""
	}
	return string(done.Status), reasonOf(done)
}

func reasonOf(p *Proposal) string {
	if p.Error == nil {
		return ""
	}
	return *p.Error
}

// refuseUnclaimed settles a pending, unclaimed proposal refused.
func (s *Service) refuseUnclaimed(ctx context.Context, p *Proposal, m *MergeProposal, code string) (string, string) {
	settled, err := s.settlePendingMerge(ctx, p, m, mergeTransition{To: ProposalStatusRejected, Code: code}, ActivityRefused, "refused: "+code)
	if err != nil || !settled {
		return statusPendingMerge, ""
	}
	return statusRefusedCreation, code
}

// settlePendingMerge applies a transition out of pending and records its
// activity in one transaction.
func (s *Service) settlePendingMerge(ctx context.Context, p *Proposal, m *MergeProposal, tr mergeTransition, outcome ActivityOutcome, detail string) (bool, error) {
	tr.ID, tr.From = p.ID, []ProposalStatus{ProposalStatusPending}
	row := ActivityRow{
		CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ActionClass: ActionMerge, Outcome: outcome,
		Authorization: AuthRequiresApproval, ReasonCode: optString(tr.Code), TargetTaskID: p.TargetTaskID, ProposalID: &p.ID,
		Detail: truncateRunes(m.ChangeKey+" "+detail, 400),
	}
	matched, err := s.settleDecision(ctx, p.CoordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.transitionMergeTx(ctx, tx, tr, time.Now())
	})
	if err != nil {
		s.logger.Warn("merge: pending settlement failed", zap.String("proposal_id", p.ID), zap.Error(err))
		return false, err
	}
	if matched {
		s.publishCoordinatorUpdated(ctx, p.WorkspaceID, p.CoordinatorID)
	}
	return matched, nil
}

// markManagerOnly marks the proposal for a manager and records the note once.
func (s *Service) markManagerOnly(ctx context.Context, p *Proposal, m *MergeProposal, note, reason string) {
	s.writeMergeState(ctx, p, m, true, note, reason, note)
}

// noteWaiting records a waiting reason, an activity row only on a change.
func (s *Service) noteWaiting(ctx context.Context, p *Proposal, m *MergeProposal, reason string) {
	detail := ""
	if reason != m.LastReason {
		detail = "waiting: " + reason
	}
	s.writeMergeState(ctx, p, m, m.ManagerOnly, m.Note, reason, detail)
}

func (s *Service) writeMergeState(ctx context.Context, p *Proposal, m *MergeProposal, managerOnly bool, note, reason, detail string) {
	err := s.store.withCoordinatorLock(ctx, p.CoordinatorID, func(tx coordinatorExec) error {
		if err := s.store.setMergeStateTx(ctx, tx, p.ID, managerOnly, note, reason); err != nil {
			return err
		}
		if detail == "" {
			return nil
		}
		return s.Record(ctx, tx, ActivityRow{
			CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ActionClass: ActionMerge, Outcome: ActivityNote,
			Authorization: AuthAutomatic, ReasonCode: optString(reason), TargetTaskID: p.TargetTaskID, ProposalID: &p.ID,
			Detail: truncateRunes(m.ChangeKey+" "+detail, 400),
		})
	})
	if err != nil {
		s.logger.Warn("merge: state not recorded", zap.String("proposal_id", p.ID), zap.Error(err))
		return
	}
	m.ManagerOnly, m.Note, m.LastReason = managerOnly, note, reason
}

// sweepPendingMerge settles a pending merge proposal that can no longer be
// merged, whatever its policy: expired, a denied repository, a closed or
// merged change, or a moved head. It reports whether it settled one.
func (s *Service) sweepPendingMerge(ctx context.Context, c *Coordinator, p *Proposal, m *MergeProposal) bool {
	if !s.merge {
		return false
	}
	code := s.pendingMergeSettlement(ctx, c, p, m)
	if code == "" {
		return false
	}
	status, _ := s.refuseUnclaimed(ctx, p, m, code)
	return status == statusRefusedCreation
}

func (s *Service) pendingMergeSettlement(ctx context.Context, c *Coordinator, p *Proposal, m *MergeProposal) string {
	if !p.CreatedAt.Add(mergeExpiry).After(s.store.now()) {
		return refuseExpired
	}
	if ok, err := s.mergeAuthority(ctx, c, m, false); err == nil && !ok {
		return refuseAuthority
	}
	merger := s.changeMerger()
	key, parsed := mergestate.ParseChangeKey(m.ChangeKey)
	if merger == nil || !parsed {
		return ""
	}
	snap, err := merger.ReadLive(ctx, c.WorkspaceID, key)
	switch {
	case err != nil:
		return ""
	case snap.State == mergestate.StateClosed:
		return refuseClosed
	case snap.State == mergestate.StateMerged:
		return refuseMergedElsewhere
	case snap.HeadSHA != "" && snap.HeadSHA != m.HeadSHA:
		return refuseStaleHead
	}
	return ""
}
