package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/replay"
)

func retentionRowCount(t *testing.T, store *Store, coordinatorID string) int {
	t.Helper()
	return len(listActivity(t, store, coordinatorID))
}

func TestActivityRetention_DeletesOnlyPastCutoffAcrossBatches(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	old := svc.store.now().UTC().Add(-activityRetentionAge - time.Hour)
	fresh := svc.store.now().UTC().Add(-activityRetentionAge + time.Hour)
	orig := activityRetentionBatch
	activityRetentionBatch = 2
	t.Cleanup(func() { activityRetentionBatch = orig })
	for i, id := range []string{"o1", "o2", "o3", "o4", "o5"} {
		seedActivity(t, store, c.ID, id, ActionCreateTask, ActivityProposed, old.Add(time.Duration(i)*time.Second))
	}
	seedActivity(t, store, c.ID, "keep", ActionCreateTask, ActivityProposed, fresh)
	svc.runActivityRetention(context.Background())
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].ID != "keep" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestActivityRetention_StartRunsPassAndTickerThenStops(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	old := svc.store.now().UTC().Add(-activityRetentionAge - time.Hour)
	seedActivity(t, store, c.ID, "o1", ActionMove, ActivityProposed, old)
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	stopped := make(chan struct{})
	svc.startActivityRetention(ctx, tick, func() { close(stopped) })
	deadline := time.After(5 * time.Second)
	for retentionRowCount(t, store, c.ID) != 0 {
		select {
		case <-deadline:
			t.Fatal("startup pass did not delete")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for svc.retentionRunning.Load() {
		select {
		case <-deadline:
			t.Fatal("startup pass did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
	seedActivity(t, store, c.ID, "o2", ActionMove, ActivityProposed, old)
	tick <- time.Now()
	for retentionRowCount(t, store, c.ID) != 0 {
		select {
		case <-deadline:
			t.Fatal("tick pass did not delete")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	svc.WaitActivityRetentionStopped()
	select {
	case <-stopped:
	default:
		t.Fatal("ticker not stopped")
	}
}

func TestActivityRetention_PhaseOffStartsNothing(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, false)
	old := svc.store.now().UTC().Add(-activityRetentionAge - time.Hour)
	seedActivity(t, store, c.ID, "o1", ActionMove, ActivityProposed, old)
	svc.StartActivityRetention(context.Background())
	svc.WaitActivityRetentionStopped()
	if retentionRowCount(t, store, c.ID) != 1 {
		t.Fatal("row deleted with phase 2 off")
	}
}

func TestActivityRetention_OverlappingRunReturnsAtOnce(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	old := svc.store.now().UTC().Add(-activityRetentionAge - time.Hour)
	seedActivity(t, store, c.ID, "o1", ActionMove, ActivityProposed, old)
	svc.retentionRunning.Store(true)
	svc.runActivityRetention(context.Background())
	if retentionRowCount(t, store, c.ID) != 1 {
		t.Fatal("overlapping run deleted rows")
	}
	if !svc.retentionRunning.Load() {
		t.Fatal("overlapping run cleared the running flag")
	}
}

func TestActivityRetention_CancelledContextDeletesNothing(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	old := svc.store.now().UTC().Add(-activityRetentionAge - time.Hour)
	seedActivity(t, store, c.ID, "o1", ActionMove, ActivityProposed, old)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.runActivityRetention(ctx)
	if retentionRowCount(t, store, c.ID) != 1 {
		t.Fatal("cancelled run deleted rows")
	}
	if svc.retentionRunning.Load() {
		t.Fatal("running flag left set")
	}
}

func TestActivityRetention_PrunesOutcomeRowsPastCutoff(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	old := svc.store.now().UTC().Add(-outcomesRetention - time.Hour)
	fresh := svc.store.now().UTC().Add(-outcomesRetention + time.Hour)
	seedOutcomeRows(t, store, c.ID, old)
	mustExec(t, store, `INSERT INTO coordinator_outcomes (proposal_id, coordinator_id, kind, decision, decided_at, graded_at)
		VALUES ('p-keep', ?, 'create_task', 'approved', ?, ?)`, c.ID, fresh, fresh)
	svc.runActivityRetention(context.Background())
	if n := count(t, store, "coordinator_outcomes"); n != 1 {
		t.Fatalf("outcome rows = %d, want the fresh one", n)
	}
	if n := count(t, store, "coordinator_feedback") + count(t, store, "coordinator_moveback_seen"); n != 0 {
		t.Fatalf("old feedback and seen rows left: %d", n)
	}
}

func TestActivityRetention_PrunesReplayResultsPastCutoff(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	now := svc.store.now().UTC()
	mustExec(t, store, `INSERT INTO coordinator_replay_results (id, coordinator_id, status, created_at) VALUES ('r-old', ?, 'done', ?)`,
		c.ID, now.Add(-replay.Retention-time.Hour))
	mustExec(t, store, `INSERT INTO coordinator_replay_results (id, coordinator_id, status, created_at) VALUES ('r-new', ?, 'done', ?)`,
		c.ID, now.Add(-replay.Retention+time.Hour))
	svc.runActivityRetention(context.Background())
	if n := count(t, store, "coordinator_replay_results"); n != 1 {
		t.Fatalf("replay rows = %d, want the fresh one", n)
	}
}
