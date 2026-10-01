package dream

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
)

type plan struct {
	dream    coordinator.Dream
	evidence Evidence
	turns    map[string]coordinator.DreamTurn
}

// admit evaluates the admission conditions in order and, when all hold,
// returns the plan with the running lease row stored. A condition that fails
// stores nothing, except an unchanged input, which stores one skipped row.
func (s *Scheduler) admit(ctx context.Context, c *coordinator.Coordinator, now time.Time) (plan, bool) {
	st := s.d.Store
	switch {
	case c.PausedAt != nil, !c.AutonomyEnabled:
		return plan{}, false
	}
	if on, err := st.ShadowDreamEnabled(ctx, c.ID); err != nil || !on {
		return plan{}, false
	}
	if running, err := st.RunningDream(ctx, c.ID); err != nil || running != nil {
		return plan{}, false
	}
	if !s.d.Conditions.ContainmentOK(ctx, c) {
		return plan{}, false
	}
	if measurable, atCeiling := s.d.Conditions.Spend(ctx, c, now); !measurable || atCeiling {
		return plan{}, false
	}
	if prev, err := st.LastDream(ctx, c.ID); err != nil || (prev != nil && now.Sub(prev.StartedAt) < Spacing) {
		return plan{}, false
	}
	last, err := st.LastAcceptedDream(ctx, c.ID)
	if err != nil {
		return plan{}, false
	}
	start, ok := s.windowStart(ctx, c.ID, last, now)
	if !ok {
		return plan{}, false
	}
	return s.gather(ctx, c, start, now)
}

func (s *Scheduler) windowStart(ctx context.Context, coordinatorID string, last *coordinator.Dream, now time.Time) (time.Time, bool) {
	var start time.Time
	if last != nil {
		start = last.WindowEnd
	} else if first, err := s.d.Store.FirstLedgerTurnAt(ctx, coordinatorID); err != nil || first == nil {
		return time.Time{}, false
	} else {
		start = *first
	}
	if floor := now.Add(-maxWindow); start.Before(floor) {
		start = floor
	}
	return start, start.Before(now)
}

func (s *Scheduler) gather(ctx context.Context, c *coordinator.Coordinator, start, end time.Time) (plan, bool) {
	st := s.d.Store
	turns, err := st.CountDreamTurns(ctx, c.ID, start, end)
	if err != nil || turns < MinTurns {
		return plan{}, false
	}
	if n, err := st.CountDreamDecisions(ctx, c.ID, start, end); err != nil || n < MinDecisions {
		return plan{}, false
	}
	rows, err := st.DreamWindowTurns(ctx, c.ID, start, end, defaultBatchLimit)
	if err != nil {
		return plan{}, false
	}
	decisions, err := st.DreamWindowDecisions(ctx, c.ID, start, end, defaultBatchLimit)
	if err != nil {
		return plan{}, false
	}
	ev := BuildEvidence(projectTurns(rows), projectDecisions(decisions))
	model := s.d.Conditions.Model(ctx, c)
	d := coordinator.Dream{
		ID: newID(), CoordinatorID: c.ID, WindowStart: start, WindowEnd: end,
		InputHash: InputHash(ev, model), TurnIDs: ev.TurnIDs, Model: model, StartedAt: end,
	}
	if same, err := st.AcceptedDreamWithHash(ctx, c.ID, d.InputHash); err != nil {
		return plan{}, false
	} else if same {
		d.Reason = ReasonUnchanged
		if _, err := st.InsertSkippedDream(ctx, d); err != nil {
			s.d.Log.Warn("dream tick: skipped row", zap.Error(err))
		}
		return plan{}, false
	}
	if held, err := st.InsertRunningDream(ctx, d); err != nil || !held {
		return plan{}, false
	}
	byID := make(map[string]coordinator.DreamTurn, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	return plan{dream: d, evidence: ev, turns: byID}, true
}

func projectTurns(rows []coordinator.DreamTurn) []Turn {
	out := make([]Turn, 0, len(rows))
	for _, r := range rows {
		out = append(out, Turn{ID: r.ID, Trigger: r.Trigger, Verdict: r.Verdict, Calls: r.Calls})
	}
	return out
}

func projectDecisions(rows []coordinator.DreamDecision) []Decision {
	out := make([]Decision, 0, len(rows))
	for _, r := range rows {
		out = append(out, Decision{
			ProposalID: r.ProposalID, Kind: r.Kind, Decision: r.Decision, EditedFields: r.EditedFields,
			ReasonCode: r.ReasonCode, TaskResult: r.TaskResult, CostSubcents: r.CostSubcents, Title: r.Title,
		})
	}
	return out
}
