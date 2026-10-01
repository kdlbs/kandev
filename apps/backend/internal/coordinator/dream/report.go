package dream

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/replay"
)

// report gates every item and replays up to MaxReplays passing items in report
// order, one at a time. A non-empty reason means the dream must fail.
func (s *Scheduler) report(ctx context.Context, c *coordinator.Coordinator, p plan, ans Answer) ([]coordinator.DreamItem, string) {
	orders, err := s.d.Store.ActiveStandingOrders(ctx, c.ID)
	if err != nil {
		return nil, ReasonStoreError
	}
	active := make(map[string]struct{}, len(orders))
	for _, o := range orders {
		active[o.ID] = struct{}{}
	}
	in := GateInput{
		CoordinatorID: c.ID, WindowStart: p.dream.WindowStart, WindowEnd: p.dream.WindowEnd,
		ContextLimit: contextLimit(c),
		Turn: func(id string) (CitedTurn, bool) {
			t, ok := p.turns[id]
			return CitedTurn{CoordinatorID: c.ID, Trigger: t.Trigger, StartedAt: t.StartedAt}, ok
		},
		Order: func(id string) (StandingOrderRef, bool) {
			_, ok := active[id]
			return StandingOrderRef{CoordinatorID: c.ID}, ok
		},
		HasCredential: coordinator.ContainsCredential,
	}
	items := make([]coordinator.DreamItem, 0, len(ans.Items))
	replays := 0
	for i, it := range ans.Items {
		row := coordinator.DreamItem{
			ID: newID(), DreamID: p.dream.ID, Position: i, Kind: it.Kind, Text: it.Text,
			TargetID: it.TargetID, CitedTurnIDs: it.CitedTurnIDs, Gate: Check(it, in),
		}
		if row.Gate == GatePass && replays < MaxReplays {
			replays++
			if reason := s.replayItem(ctx, c.ID, p.dream.ID, &row, it); reason != "" {
				return nil, reason
			}
		}
		items = append(items, row)
	}
	return items, ""
}

// replayItem runs one replay. A cancelled or expired context fails the dream;
// any other error leaves the item unmeasured.
func (s *Scheduler) replayItem(ctx context.Context, coordinatorID, dreamID string, row *coordinator.DreamItem, it Item) string {
	res, err := s.d.Replay.Run(ctx, replay.Request{
		CoordinatorID: coordinatorID, DreamID: dreamID, ItemID: row.ID,
		Candidate: replay.Candidate{Kind: it.Kind, Text: it.Text, TargetID: it.TargetID, CitedTurnIDs: it.CitedTurnIDs},
	})
	if ctx.Err() != nil {
		return ReasonRunError
	}
	if err != nil {
		s.d.Log.Warn("dream replay", zap.String("dream_id", dreamID), zap.Error(err))
		row.Verdict, row.ReplayReason = replay.VerdictUnmeasured, "error"
		return ""
	}
	row.ReplayID, row.Verdict, row.ReplayReason = res.RowID, res.Verdict, res.Reason
	if res.Guard == replay.GuardBlocked {
		row.Verdict = res.Guard
	}
	return ""
}

func contextLimit(_ *coordinator.Coordinator) int {
	return coordinator.ContextMaxRunes
}
