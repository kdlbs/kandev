package dream

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/coordinator"
)

// Health computes the health line of one coordinator. An evidence read that
// fails is an error, never a quiet "no evidence".
func (s *Scheduler) Health(ctx context.Context, c *coordinator.Coordinator) (coordinator.LearningHealth, error) {
	in, err := s.healthInput(ctx, c)
	if err != nil {
		return coordinator.LearningHealth{}, err
	}
	h := Health(in)
	return coordinator.LearningHealth{State: h.State, Condition: h.Condition, Detail: h.Detail}, nil
}

func (s *Scheduler) healthInput(ctx context.Context, c *coordinator.Coordinator) (HealthInput, error) {
	st := s.d.Store
	now := s.d.Clock()
	measurable, atCeiling := s.d.Conditions.Spend(ctx, c, now)
	in := HealthInput{
		Now: now, Autonomy: c.AutonomyEnabled, Paused: c.PausedAt != nil,
		ContainmentOK: s.d.Conditions.ContainmentOK(ctx, c), SpendMeasurable: measurable, AtCeiling: atCeiling,
	}
	var err error
	if in.Enabled, err = st.ShadowDreamEnabled(ctx, c.ID); err != nil {
		return in, fmt.Errorf("health: shadow switch: %w", err)
	}
	running, err := st.RunningDream(ctx, c.ID)
	if err != nil {
		return in, fmt.Errorf("health: running dream: %w", err)
	}
	in.Running = running != nil
	last, err := st.LastDream(ctx, c.ID)
	if err != nil {
		return in, fmt.Errorf("health: last dream: %w", err)
	}
	if last != nil && last.Status == coordinator.DreamFailed {
		in.LastFailed, in.LastFailedReason = true, last.Reason
	}
	if last != nil {
		next := last.StartedAt.Add(Spacing)
		in.NextAfter = &next
	}
	accepted, err := st.LastAcceptedDream(ctx, c.ID)
	if err != nil {
		return in, fmt.Errorf("health: last accepted dream: %w", err)
	}
	if accepted != nil && accepted.FinishedAt != nil {
		in.LastAcceptedAt = accepted.FinishedAt
	}
	if in.DebtSince, err = s.debtSince(ctx, c.ID, accepted, now); err != nil {
		return in, fmt.Errorf("health: evidence: %w", err)
	}
	return in, nil
}

// debtSince is when the evidence for a dream became sufficient and has not
// been dreamed on since; nil when it is not sufficient.
func (s *Scheduler) debtSince(ctx context.Context, coordinatorID string, accepted *coordinator.Dream, now time.Time) (*time.Time, error) {
	start, ok := s.windowStart(ctx, coordinatorID, accepted, now)
	if !ok {
		return nil, nil
	}
	turns, err := s.d.Store.CountDreamTurns(ctx, coordinatorID, start, now)
	if err != nil || turns < MinTurns {
		return nil, err
	}
	decisions, err := s.d.Store.CountDreamDecisions(ctx, coordinatorID, start, now)
	if err != nil || decisions < MinDecisions {
		return nil, err
	}
	return s.d.Store.NthCompletedTurnFinish(ctx, coordinatorID, start, MinTurns)
}
