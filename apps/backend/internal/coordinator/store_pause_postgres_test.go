package coordinator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPause_Postgres_ColumnsPauseResumeAndConcurrentCommitOrder(t *testing.T) {
	s := newTestStorePostgres(t)
	ctx := context.Background()
	c := newPausableCoordinator(t, s)
	for table, want := range map[string][]string{
		"coordinators":                 {"paused_at ", "paused_by "},
		"coordinator_unattended_turns": {"pause_requested_at ", "pause_cancel_at "},
	} {
		cols := strings.Join(tableColumns(t, s.db, table), "|")
		for _, w := range want {
			if !strings.Contains(cols, w) {
				t.Fatalf("%s is missing column %q in %s", table, w, cols)
			}
		}
	}
	if changed, err := s.SetPaused(ctx, c.ID, true, "u-1"); err != nil || !changed {
		t.Fatalf("pause = %v, %v", changed, err)
	}
	if changed, err := s.SetPaused(ctx, c.ID, true, "u-2"); err != nil || changed {
		t.Fatalf("second pause = %v, %v", changed, err)
	}
	if paused, err := s.IsPaused(ctx, c.ID); err != nil || !paused {
		t.Fatalf("IsPaused = %v, %v", paused, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(pause bool) {
			defer wg.Done()
			if _, err := s.SetPaused(ctx, c.ID, pause, "u-x"); err != nil {
				t.Errorf("SetPaused(%v): %v", pause, err)
			}
		}(i%2 == 0)
	}
	wg.Wait()
	got, err := s.GetCoordinatorByID(ctx, c.ID)
	if err != nil || (got.PausedAt == nil) != (got.PausedBy == "") {
		t.Fatalf("inconsistent final state %+v err=%v", got, err)
	}
}

func TestPause_Postgres_SettlePausedUnsentReturnsWakes(t *testing.T) {
	s := newTestStorePostgres(t)
	ctx := context.Background()
	c := newPausableCoordinator(t, s)
	now := time.Now().UTC()
	mustExec(t, s, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, wake_count, start_ceiling_subcents, started_at)
		VALUES (?, ?, ?, ?, 1, 0, ?)`, "ut-1", c.ID, "conv-1", "sess-1", now)
	insertWakeRow(t, s, c, "w1", "task-1", string(WakeKindQuestion), "q1", "delivered", now)
	mustExec(t, s, `UPDATE coordinator_wakes SET turn_id = 'ut-1' WHERE id = 'w1'`)
	if changed, err := s.settlePausedUnsentTurn(ctx, "ut-1", c.ID); err != nil || changed {
		t.Fatalf("settled while not paused: %v %v", changed, err)
	}
	if _, err := s.SetPaused(ctx, c.ID, true, "u-1"); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.settlePausedUnsentTurn(ctx, "ut-1", c.ID); err != nil || !changed {
		t.Fatalf("settle = %v %v", changed, err)
	}
	var status string
	if err := s.db.QueryRow(s.db.Rebind(`SELECT status FROM coordinator_wakes WHERE id = 'w1'`)).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("wake = %q err=%v", status, err)
	}
}
