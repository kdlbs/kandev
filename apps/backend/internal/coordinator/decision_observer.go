package coordinator

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/outcomes"
)

// DecisionEvent describes one decision taken on a proposal. It carries names
// and codes only, never a manager's text.
type DecisionEvent struct {
	ProposalID    string
	CoordinatorID string
	WorkspaceID   string
	ProposalKind  string
	Decision      string
	Automatic     bool
	// ActorUserID is the deciding user's id; empty for an internal or
	// synthetic caller (authentication disabled).
	ActorUserID  string
	EditedFields []string
	ReasonCode   string
	// At is the decision time; zero when the path has no decision time of its
	// own (an undo).
	At time.Time
}

// DecisionObserver is told about every decision after its transaction
// committed. It must not fail the decision.
type DecisionObserver interface {
	OnDecision(ctx context.Context, e DecisionEvent)
}

// SetDecisionObserver registers the observer of decisions; nil clears it.
func (s *Service) SetDecisionObserver(o DecisionObserver) {
	s.observerMu.Lock()
	s.decisionObserver = o
	s.observerMu.Unlock()
}

// notifyDecision calls the registered observer, if any. A panic in it is
// recovered and logged: observing never changes a decision.
func (s *Service) notifyDecision(ctx context.Context, e DecisionEvent) {
	s.observerMu.RLock()
	o := s.decisionObserver
	s.observerMu.RUnlock()
	if o == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("coordinator decision observer panicked", zap.String("proposal_id", e.ProposalID), zap.Any("panic", r))
		}
	}()
	o.OnDecision(context.WithoutCancel(ctx), e)
}

// notifyApproved reports an approval that took effect. The decision is
// edited when the frozen spec differs from the proposed one.
func (s *Service) notifyApproved(ctx context.Context, p *Proposal) {
	if p == nil || p.Status != ProposalStatusApproved {
		return
	}
	edited := editedFieldsOf(p)
	decision := outcomes.DecisionApproved
	if len(edited) > 0 {
		decision = outcomes.DecisionEdited
	}
	s.notifyDecision(ctx, DecisionEvent{
		ProposalID: p.ID, CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ProposalKind: proposalKindOf(p),
		Decision: decision, Automatic: p.ClaimedAutomatically, ActorUserID: decidingUserID(ctx),
		EditedFields: edited, ReasonCode: outcomes.ReasonNone, At: p.UpdatedAt,
	})
}

// notifyDecided reports a rejection, a return or an undo.
func (s *Service) notifyDecided(ctx context.Context, p *Proposal, decision, reasonCode string, at time.Time) {
	if p == nil {
		return
	}
	s.notifyDecision(ctx, DecisionEvent{
		ProposalID: p.ID, CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ProposalKind: proposalKindOf(p),
		Decision: decision, ActorUserID: decidingUserID(ctx), ReasonCode: reasonCode, At: at,
	})
}

func proposalKindOf(p *Proposal) string {
	if p.Kind == "" {
		return ProposalKindCreateTask
	}
	return p.Kind
}

// editedFieldsOf is the sorted names of the spec fields a manager changed
// before approval; the proposal's raw documents are compared, so an
// unreadable one reads as no edits.
func editedFieldsOf(p *Proposal) []string {
	if p.RawFinalSpec != "" || p.FinalSpec != nil {
		return outcomes.EditedFields(rawSpecOf(p), rawFinalSpecOf(p))
	}
	return []string{}
}

func rawSpecOf(p *Proposal) []byte {
	if p.RawSpec != "" {
		return []byte(p.RawSpec)
	}
	b, _ := json.Marshal(p.Spec)
	return b
}

func rawFinalSpecOf(p *Proposal) []byte {
	if p.RawFinalSpec != "" {
		return []byte(p.RawFinalSpec)
	}
	if p.FinalSpec == nil {
		return nil
	}
	b, _ := json.Marshal(p.FinalSpec)
	return b
}

// notifyUndone reports the undo of an approved action. The undo has no
// decision time of its own, so the event carries a zero time.
func (s *Service) notifyUndone(ctx context.Context, row *ActivityRow) {
	if row.ProposalID == nil {
		return
	}
	p, err := s.store.GetProposalAnyKind(ctx, row.WorkspaceID, row.CoordinatorID, *row.ProposalID)
	if err != nil {
		s.logger.Warn("undo decision not observed: proposal read failed", zap.String("proposal_id", *row.ProposalID), zap.Error(err))
		return
	}
	s.notifyDecided(ctx, p, outcomes.DecisionUndone, outcomes.ReasonNone, time.Time{})
}
