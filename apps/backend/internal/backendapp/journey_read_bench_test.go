package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
)

func BenchmarkJourneyReadLoad(b *testing.B) {
	for _, workload := range []struct {
		name      string
		taskCount int
		writers   bool
	}{
		{name: "idle", taskCount: 100},
		{name: "large_message_write", taskCount: 1, writers: true},
	} {
		b.Run(workload.name, func(b *testing.B) {
			fixture := newJourneyReadFixture(b, workload.taskCount)
			fixture.assertHomeSnapshot(b, fixture.homeSnapshot(context.Background()))
			stopWriters := func() {}
			var writerErrors <-chan error
			if workload.writers {
				stopWriters, writerErrors = fixture.startLargeMessageWriters(b)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				started := time.Now()
				state := fixture.homeSnapshot(context.Background())
				elapsed := time.Since(started)
				fixture.assertHomeSnapshot(b, state)
				payload, err := json.Marshal(state)
				if err != nil {
					b.Fatalf("marshal homepage snapshot: %v", err)
				}
				b.ReportMetric(float64(len(payload)), "payload_B/op")
				b.Logf("JOURNEY_READ_SAMPLE workload=%s tasks=%d elapsed_ms=%.3f payload_bytes=%d",
					workload.name, workload.taskCount, float64(elapsed)/float64(time.Millisecond), len(payload))
			}
			b.StopTimer()
			stopWriters()
			if writerErrors != nil {
				for err := range writerErrors {
					if err != nil {
						b.Fatalf("large-message writer: %v", err)
					}
				}
			}
		})
	}
}

func BenchmarkJourneyReadHealthLoad(b *testing.B) {
	for _, writers := range []bool{false, true} {
		name := "idle"
		if writers {
			name = "large_message_write"
		}
		b.Run(name, func(b *testing.B) {
			fixture := newJourneyReadFixture(b, 1)
			fixture.assertHomeSnapshot(b, fixture.homeSnapshot(context.Background()))
			tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{ID: "journey", OwnerPackage: "journey-fixture", RequiredTables: []string{"workspaces", "tasks"}, Sweep: startup.StepStoresRepositories}})
			if err != nil {
				b.Fatal(err)
			}
			if err := tracker.RecordSuccess("journey"); err != nil {
				b.Fatal(err)
			}
			health := requiredstores.NewHealth(tracker, db.NewPool(fixture.writer, fixture.reader), nil)
			if err := health.Check(context.Background()); err != nil {
				b.Fatalf("fixture health: %v", err)
			}
			stop := func() {}
			var writerErrors <-chan error
			if writers {
				stop, writerErrors = fixture.startLargeMessageWriters(b)
			}
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				before := fixture.writer.Stats()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				probe := make(chan struct {
					err     error
					elapsed time.Duration
				}, 1)
				go func() {
					started := time.Now()
					err := health.Check(ctx)
					probe <- struct {
						err     error
						elapsed time.Duration
					}{err, time.Since(started)}
				}()
				routeStarted := time.Now()
				fixture.assertHomeSnapshot(b, fixture.homeSnapshot(context.Background()))
				routeElapsed := time.Since(routeStarted)
				result := <-probe
				cancel()
				after := fixture.writer.Stats()
				fmt.Printf("JOURNEY_HEALTH_SAMPLE iterations=%d iteration=%d workload=%s probe_ms=%.3f probe_failed=%t route_ms=%.3f writer_waits=%d writer_wait_ms=%.3f error=%v\n", b.N, index, name, float64(result.elapsed)/float64(time.Millisecond), result.err != nil, float64(routeElapsed)/float64(time.Millisecond), after.WaitCount-before.WaitCount, float64(after.WaitDuration-before.WaitDuration)/float64(time.Millisecond), result.err)
			}
			b.StopTimer()
			stop()
			if writerErrors != nil {
				for err := range writerErrors {
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
