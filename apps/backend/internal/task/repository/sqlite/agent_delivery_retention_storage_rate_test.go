package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/deliveryretention"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionStorageSteadyState(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			runRetentionStorageSteadyRate(t, engine, 0)
		})
	}
}

func TestDeliveryRetentionStorageSteadyStateSmallBatches(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			runRetentionStorageSteadyRate(t, engine, 50)
		})
	}
}

func TestDeliveryRetentionStorageSteadyStateMinimumQuantum(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			runRetentionStorageSteadyRate(t, engine, 30)
		})
	}
}

func runRetentionStorageSteadyRate(t *testing.T, engine string, batchRows int) {
	repo, events := seedRetentionBoundsStream(t, engine, "steady-rate", 1, 0)
	assertRetentionStorageToolPolicy(t, repo)
	event := *events[0]
	prepareRetentionWindowSubmission(t, repo, &event)
	base := event.ReceivedAt
	now := base.Add(time.Hour)
	repo.clockNow = func() time.Time { return now }
	const initial = 72_000
	seedRetentionStorageRows(t, repo, &event, initial, 1024)
	for minute := 0; minute < 60; minute++ {
		at := base.Add(time.Duration(minute) * time.Minute)
		if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET received_at=?,projected_at=? WHERE stream_id=? AND sequence BETWEEN ? AND ?`), at, at, event.StreamID, minute*1200+1, (minute+1)*1200); err != nil {
			t.Fatal(err)
		}
	}
	timers := make(chan workerTimer, 2)
	snapshots := make(chan deliveryretention.Snapshot, 1)
	worker := deliveryretention.New(retentionStorageRateRepository{Repository: repo, t: t}, db.NewPool(repo.db, repo.db), deliveryretention.Options{
		// A failed batch retries on the next virtual minute, so the age assertion
		// reports capacity even when the production jitter would retry sooner.
		BatchRows:  batchRows,
		RetryDelay: func(int) time.Duration { return time.Minute },
		After: func(delay time.Duration) <-chan time.Time {
			timer := workerTimer{delay: delay, fire: make(chan time.Time, 1)}
			timers <- timer
			return timer.fire
		}, OnSnapshot: func(snapshot deliveryretention.Snapshot) { snapshots <- snapshot },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop()
	// Drain immediate startup work before advancing the repository clock.
	_ = finishRetentionStorageWorkerPass(t, timers, snapshots)
	payload := []byte(`{"type":"message_chunk","text":"` + strings.Repeat("x", 990) + `"}`)
	for minute := 1; minute <= 8; minute++ {
		now = base.Add(time.Hour + time.Duration(minute)*time.Minute)
		event.ReceivedAt = now
		seedRetentionStorageRange(t, repo, &event, initial+(minute-1)*1200+1, initial+minute*1200, payload)
		advanceRetentionStorageHighWater(t, repo, event.StreamID, initial+minute*1200)
		fireWorkerTimer(t, timers, time.Minute)
		snapshot := finishRetentionStorageWorkerPass(t, timers, snapshots)
		var oldest time.Time
		if err := repo.db.QueryRow(repo.db.Rebind(`SELECT projected_at FROM agent_delivery_inbox WHERE stream_id=? AND payload_pruned_at IS NULL ORDER BY projected_at LIMIT 1`), event.StreamID).Scan(&oldest); err != nil {
			t.Fatal(err)
		}
		age := now.Sub(oldest)
		t.Logf("engine=%s arrival=20events/s minute=%d oldest_raw_age=%s pass=%+v", engine, minute, age, snapshot)
		if age > time.Hour+2*time.Minute {
			t.Fatalf("steady-state oldest eligible raw payload exceeds grace plus two intervals: age=%s", age)
		}
	}
	t.Logf("engine=%s steady-state storage=%+v", engine, measureRetentionStorage(t, repo))
}

type retentionStorageRateRepository struct {
	*Repository
	t *testing.T
}

func (r retentionStorageRateRepository) RunAgentDeliveryRetentionBatch(ctx context.Context, options models.AgentDeliveryRetentionBatchOptions) (models.AgentDeliveryRetentionBatchResult, error) {
	result, err := r.Repository.RunAgentDeliveryRetentionBatch(ctx, options)
	if err != nil {
		r.t.Logf("retention batch failed: %v", err)
	}
	return result, err
}

func (r retentionStorageRateRepository) RunAgentDeliveryOrphanBatch(ctx context.Context, options models.AgentDeliveryOrphanBatchOptions) (models.AgentDeliveryOrphanBatchResult, error) {
	result, err := r.Repository.RunAgentDeliveryOrphanBatch(ctx, options)
	if err != nil {
		r.t.Logf("orphan batch failed: %v", err)
	}
	return result, err
}

func (r retentionStorageRateRepository) RunAgentDeliveryRetirementBatch(ctx context.Context, options models.AgentDeliveryRetirementBatchOptions) (models.AgentDeliveryRetirementBatchResult, error) {
	result, err := r.Repository.RunAgentDeliveryRetirementBatch(ctx, options)
	if err != nil {
		r.t.Logf("retirement batch failed: %v", err)
	}
	return result, err
}

func finishRetentionStorageWorkerPass(t *testing.T, timers chan workerTimer, snapshots <-chan deliveryretention.Snapshot) deliveryretention.Snapshot {
	t.Helper()
	for {
		snapshot := receiveWorkerSnapshot(t, snapshots)
		if snapshot.ContinuationPending {
			continue
		}
		select {
		case timer := <-timers:
			if timer.delay != time.Minute {
				t.Fatalf("unexpected idle timer: %s", timer.delay)
			}
			// Leave the idle timer for the caller's next virtual-minute advance.
			timers <- timer
			return snapshot
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not become idle after completing its pass")
			return deliveryretention.Snapshot{}
		}
	}
}
