package coordinator

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// lowerOnOverride lowers the agent-starting classes a manager's decision on p
// overrode: a reject, a return, or an approval whose spec was edited. It runs
// after the decision settled, never fails it, and returns the class it lowered
// ("" when nothing was lowered). A class that does not read automatic, or that
// was raised after the decision, is left alone.
func (s *Service) lowerOnOverride(ctx context.Context, p *Proposal) string {
	if !s.phase3 || p == nil {
		return ""
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("override lowering panicked", zap.String("coordinator_id", p.CoordinatorID), zap.Any("panic", r))
		}
	}()
	lctx := context.WithoutCancel(ctx)
	decidedAt := s.decisionTime(lctx, p)
	lowered := ""
	for _, class := range overrideClasses(p) {
		done, err := s.LowerClass(lctx, p.CoordinatorID, class, overrideLoweredReason, LowerGuard{After: &decidedAt})
		if err != nil {
			automaticLowerFailedTotal.Add(lowerTriggerOverride, 1)
			s.logger.Error("override lowering failed", zap.String("coordinator_id", p.CoordinatorID), zap.String("class", string(class)), zap.Error(err))
			continue
		}
		if done {
			lowered = string(class)
		}
	}
	return lowered
}

// decisionTime is the created_at of the newest activity row of p's decision.
// When it cannot be read the current time stands in, which only widens what is
// lowered.
func (s *Service) decisionTime(ctx context.Context, p *Proposal) time.Time {
	at, err := s.store.ProposalDecidedAt(ctx, p.ID)
	if err != nil {
		s.logger.Warn("read decision time failed", zap.String("proposal_id", p.ID), zap.Error(err))
		return s.store.now().UTC()
	}
	return at
}

// overrideClasses lists the agent-starting classes p needs: resume for a
// resume proposal, start_agent for any proposal that starts an agent,
// whether or not its kind is raisable.
func overrideClasses(p *Proposal) []Action {
	var out []Action
	if p.Kind == ProposalKindResume {
		out = append(out, ActionResume)
	}
	if p.StartsAgent {
		out = append(out, ActionStartAgent)
	}
	return out
}

// approvalOverrode reports whether a manager's approval settled p with an
// edited spec, whatever the outcome.
func approvalOverrode(p *Proposal) bool {
	if p.Status != ProposalStatusApproved && p.Status != ProposalStatusFailed {
		return false
	}
	if p.Kind == ProposalKindCreateTask || p.Kind == "" {
		return specEdited(p)
	}
	return kindSpecEdited(p)
}
