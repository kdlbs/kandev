package coordinator

import (
	"context"

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
	classes, ok := neededClasses(p)
	if !ok {
		return ""
	}
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("override lowering panicked", zap.String("coordinator_id", p.CoordinatorID), zap.Any("panic", r))
		}
	}()
	lctx := context.WithoutCancel(ctx)
	decidedAt := s.store.now().UTC()
	lowered := ""
	for _, class := range classes {
		if class != ActionResume && class != ActionStartAgent {
			continue
		}
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
