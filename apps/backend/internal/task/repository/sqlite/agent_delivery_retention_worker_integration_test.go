package sqlite

import (
	"context"
	"expvar"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/deliveryretention"
)

func TestDeliveryRetentionWorkerRepository(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newDeliveryRetentionDeletionRepo(t, engine)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := repo.CheckAgentDeliveryRetentionReady(ctx); err != nil {
				t.Fatalf("retention readiness check: %v", err)
			}
			const taskID, sessionID = "retention-worker-task", "retention-worker-session"
			session := createRetentionSession(t, repo, ctx, taskID, sessionID, false, "")
			seedRetentionDeliveryEvidence(t, repo, ctx, session)
			if err := repo.DeleteTaskSession(ctx, session); err != nil {
				t.Fatalf("retire session before worker pass: %v", err)
			}

			timers := make(chan workerTimer, 8)
			clock := func(delay time.Duration) <-chan time.Time {
				timer := workerTimer{delay: delay, fire: make(chan time.Time, 1)}
				timers <- timer
				return timer.fire
			}
			snapshots := make(chan deliveryretention.Snapshot, 1)
			worker := deliveryretention.New(repo, db.NewPool(repo.db, repo.db), deliveryretention.Options{
				After: clock, OnSnapshot: func(snapshot deliveryretention.Snapshot) { snapshots <- snapshot },
			})
			if err := worker.Start(ctx); err != nil {
				t.Fatal(err)
			}
			defer worker.Stop()

			fireWorkerTimer(t, timers, time.Minute)
			snapshot := finishRetentionStorageWorkerPass(t, timers, snapshots)
			if snapshot.CommittedBatches < 3 || snapshot.FailedBatches != 0 || snapshot.RetirementJobsCompleted != 1 {
				t.Fatalf("worker snapshot = %+v", snapshot)
			}
			assertDeliveryRetirementEvidencePurged(t, repo, sessionID)
			assertRetentionRowCount(t, repo, "agent_delivery_cleanup_jobs", "session_id = ?", sessionID, 0)
		})
	}
}

func TestDeliveryRetentionMetrics(t *testing.T) {
	repo, _ := seedRetentionBoundsStream(t, "SQLite", "worker-metrics", 3, 0)
	rowsMetric, ok := expvar.Get("agent_delivery_retention_rows_total").(*expvar.Map)
	if !ok {
		t.Fatal("retention row expvar is not registered")
	}
	before := deliveryRetentionMetricValue(t, rowsMetric, "inbox_payload")

	timers := make(chan workerTimer, 8)
	clock := func(delay time.Duration) <-chan time.Time {
		timer := workerTimer{delay: delay, fire: make(chan time.Time, 1)}
		timers <- timer
		return timer.fire
	}
	snapshots := make(chan deliveryretention.Snapshot, 1)
	worker := deliveryretention.New(repo, db.NewPool(repo.db, repo.db), deliveryretention.Options{
		After: clock, OnSnapshot: func(snapshot deliveryretention.Snapshot) { snapshots <- snapshot },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop()

	fireWorkerTimer(t, timers, time.Minute)
	snapshot := finishRetentionStorageWorkerPass(t, timers, snapshots)
	if snapshot.CommittedBatches < 3 || snapshot.FailedBatches != 0 || snapshot.EventPayloadsPruned == 0 {
		t.Fatalf("worker snapshot = %+v", snapshot)
	}
	if got := deliveryRetentionMetricValue(t, rowsMetric, "inbox_payload"); got != before+int64(snapshot.EventPayloadsPruned) {
		t.Fatalf("committed inbox payload metric = %d, want %d", got, before+int64(snapshot.EventPayloadsPruned))
	}
	if !snapshot.BacklogSampleAvailable || snapshot.OldestEligibleAgeSeconds < 0 {
		t.Fatalf("eligible backlog sample unavailable: %+v", snapshot)
	}
}

func deliveryRetentionMetricValue(t *testing.T, metrics *expvar.Map, key string) int64 {
	t.Helper()
	value, ok := metrics.Get(key).(*expvar.Int)
	if !ok {
		return 0
	}
	return value.Value()
}

type workerTimer struct {
	delay time.Duration
	fire  chan time.Time
}

func fireWorkerTimer(t *testing.T, timers <-chan workerTimer, want time.Duration) {
	t.Helper()
	select {
	case timer := <-timers:
		if timer.delay != want {
			t.Fatalf("worker timer delay = %s, want %s", timer.delay, want)
		}
		timer.fire <- time.Now()
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not request timer")
	}
}

func receiveWorkerSnapshot(t *testing.T, snapshots <-chan deliveryretention.Snapshot) deliveryretention.Snapshot {
	t.Helper()
	select {
	case snapshot := <-snapshots:
		return snapshot
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not report a completed pass")
		return deliveryretention.Snapshot{}
	}
}
