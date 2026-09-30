package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestStartUnattendedTurnPostgres_ConcurrentStartsYieldOneOpenTurn(t *testing.T) {
	s := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, s, "ws-1")
	mustExec(t, s, `UPDATE coordinators SET autonomy_enabled = ?, cost_ceiling_subcents = 1000, conversation_task_id = ? WHERE id = ?`, true, "conv", c.ID)
	now := time.Now().UTC()
	insertWakeRow(t, s, c, "w1", "task-1", string(WakeKindQuestion), "q1", "pending", now)
	insertWakeRow(t, s, c, "w2", "task-2", string(WakeKindQuestion), "q2", "pending", now)

	var mu sync.Mutex
	started := 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := s.startUnattendedTurn(context.Background(), turnStart{
				CoordinatorID: c.ID, ConvTaskID: "conv", SessionID: "sess", WakeIDs: []string{"w1", "w2"},
			})
			if err != nil {
				t.Errorf("startUnattendedTurn: %v", err)
				return
			}
			if got != nil {
				mu.Lock()
				started++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if started != 1 {
		t.Fatalf("started = %d, want exactly 1", started)
	}
	var open int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM coordinator_unattended_turns WHERE outcome IS NULL`).Scan(&open); err != nil || open != 1 {
		t.Fatalf("open turns = %d err=%v, want 1", open, err)
	}
}

func TestSettleOpenTurnPostgres_ChangesARowOnce(t *testing.T) {
	s := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)
	ctx := context.Background()
	first, err := s.settleOpenTurn(ctx, "turn-1", outcomeCompleted, time.Now())
	if err != nil || !first {
		t.Fatalf("first settle = %v %v, want changed", first, err)
	}
	if second, err := s.settleOpenTurn(ctx, "turn-1", outcomeFailed, time.Now()); err != nil || second {
		t.Fatalf("second settle = %v %v, want unchanged", second, err)
	}
	if got, err := s.openBoundTurn(ctx, c.ID); err != nil || got != nil {
		t.Fatalf("openBoundTurn = %+v %v, want none after settle", got, err)
	}
	recent, err := s.recentSettledTurns(ctx, c.ID, time.Now().Add(-time.Minute))
	if err != nil || len(recent) != 1 || recent[0].Outcome != outcomeCompleted {
		t.Fatalf("recentSettledTurns = %+v %v", recent, err)
	}
}

func TestSettleUnsentTurnPostgres_LosesToALateBinding(t *testing.T) {
	s := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", nil, nil)
	ctx := context.Background()
	if bound, err := s.bindAcceptedTurn(ctx, "turn-1", "st-1"); err != nil || !bound {
		t.Fatalf("bind = %v %v", bound, err)
	}
	if changed, err := s.settleUnsentTurn(ctx, "turn-1", outcomeSendFailed); err != nil || changed {
		t.Fatalf("settle after binding = %v %v, want unchanged", changed, err)
	}
}

func TestRecordUnattendedDenialPostgres_UnboundRowWithReservedIDDeniesOnlyThatTurn(t *testing.T) {
	s := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", nil, nil)
	mustExec(t, s, `UPDATE coordinator_unattended_turns SET reserved_turn_id = ? WHERE id = ?`, "rt", "turn-1")
	ctx := context.Background()
	if _, matched, err := s.RecordUnattendedDenial(ctx, "conv", "sess", "p1", "manager-turn"); err != nil || matched {
		t.Fatalf("another turn: matched=%v err=%v, want no match", matched, err)
	}
	if _, matched, err := s.RecordUnattendedDenial(ctx, "conv", "sess", "p1", "rt"); err != nil || !matched {
		t.Fatalf("reserved turn: matched=%v err=%v, want a match", matched, err)
	}
}
