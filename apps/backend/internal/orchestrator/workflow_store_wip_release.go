package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

// sessionlessWIPReleaseStepLister finds the WIP-limited steps that hold an
// admitted IN_PROGRESS or FAILED task with no live session in a workspace
// that turned on sessionless WIP release.
type sessionlessWIPReleaseStepLister interface {
	ListSessionlessWIPReleaseSteps(ctx context.Context) ([]string, error)
}

// ReleaseSessionlessWIPSlots runs the normal WIP pull for every step whose
// sessionless occupant may have passed its grace. A step with the policy off
// is never listed, and the pull's own policy-aware count decides whether a
// slot is free, so a step still inside its grace is a no-op. The released
// task is neither moved nor re-stated.
func (s *workflowStore) ReleaseSessionlessWIPSlots(ctx context.Context) {
	lister, ok := s.repo.(sessionlessWIPReleaseStepLister)
	if !ok {
		return
	}
	stepIDs, err := lister.ListSessionlessWIPReleaseSteps(ctx)
	if err != nil {
		s.logger.Warn("failed to list steps with sessionless WIP occupants", zap.Error(err))
		return
	}
	for _, stepID := range stepIDs {
		if ctx.Err() != nil {
			return
		}
		s.pullNextTaskOnVacate(ctx, stepID, "")
	}
}

// releaseSessionlessWIPSlotsOnce is the reaper-tick scan for sessionless WIP
// release.
func (s *Service) releaseSessionlessWIPSlotsOnce(ctx context.Context) {
	if s.workflowStore == nil {
		return
	}
	s.workflowStore.ReleaseSessionlessWIPSlots(ctx)
}
