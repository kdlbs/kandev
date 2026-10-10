package sqlite

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkWriterWorkload(b *testing.B) {
	for _, sessions := range []int{1, 4, 8} {
		for _, size := range []struct{ bytes, batch int }{{128, 1}, {16384, 16}} {
			b.Run(fmt.Sprintf("sessions=%d/bytes=%d/batch=%d", sessions, size.bytes, size.batch), func(b *testing.B) {
				benchmarkWriterWorkload(b, sessions, size.bytes, size.batch, false)
			})
		}
	}
	b.Run("growing/sessions=8/bytes=16384/batch=16", func(b *testing.B) {
		benchmarkWriterWorkload(b, 8, 16384, 16, true)
	})
}

func benchmarkWriterWorkload(b *testing.B, sessions, bytes, batch int, growing bool) {
	w := newWriterWorkload(b, sessions, bytes, batch)
	w.growing = growing
	o := newWriterWorkloadObservation()
	ctx := context.WithValue(context.Background(), writerWorkloadObservationKey{}, o)
	// Prime both pools and the sidebar plan before measuring steady activity.
	if err := w.sidebar(ctx); err != nil {
		b.Fatal(err)
	}
	beforeWriter, beforeReader := w.repo.db.Stats(), w.repo.ro.Stats()
	var next atomic.Int64
	errors := make(chan error, sessions+2)
	var workers sync.WaitGroup
	b.ResetTimer()
	started := time.Now()
	stopReaders := startWriterWorkloadReaders(ctx, w)
	for session := 0; session < sessions; session++ {
		workers.Add(1)
		go func(session int) {
			defer workers.Done()
			for round := 0; ; round++ {
				if next.Add(1) > int64(b.N) {
					return
				}
				if err := w.round(ctx, session, round); err != nil {
					errors <- err
					return
				}
			}
		}(session)
	}
	workers.Wait()
	elapsed := time.Since(started)
	b.StopTimer()
	stopReaders()
	close(errors)
	for err := range errors {
		b.Fatal(err)
	}
	afterWriter, afterReader := w.repo.db.Stats(), w.repo.ro.Stats()
	var pages, pageSize int64
	if err := w.repo.ro.Get(&pages, "PRAGMA page_count"); err != nil {
		b.Fatal(err)
	}
	if err := w.repo.ro.Get(&pageSize, "PRAGMA page_size"); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(b.N*batch)/elapsed.Seconds(), "chunks/s")
	o.report(b, map[string]any{"sessions": sessions, "payload_bytes": bytes, "batch": batch, "rounds": b.N, "growing": growing,
		"elapsed_seconds": elapsed.Seconds(), "db_logical_bytes": pages * pageSize,
		"writer_wait_count": afterWriter.WaitCount - beforeWriter.WaitCount,
		"writer_wait_ms":    float64(afterWriter.WaitDuration-beforeWriter.WaitDuration) / float64(time.Millisecond),
		"reader_wait_count": afterReader.WaitCount - beforeReader.WaitCount,
		"reader_wait_ms":    float64(afterReader.WaitDuration-beforeReader.WaitDuration) / float64(time.Millisecond)})
}

func (w *writerWorkload) round(ctx context.Context, session, round int) error {
	events, effects := w.events(session, round)
	if err := w.receive(ctx, events); err != nil {
		return err
	}
	if err := w.project(ctx, events, effects); err != nil {
		return err
	}
	return writerWorkloadMeasure(ctx, "terminal_lifecycle", func() error {
		event, err := w.prepareTerminal(ctx, session, round)
		if err != nil {
			return err
		}
		return w.settle(ctx, event)
	})
}

func startWriterWorkloadReaders(ctx context.Context, w *writerWorkload) func() {
	stop := make(chan struct{})
	var observers sync.WaitGroup
	for _, interval := range []time.Duration{100 * time.Millisecond, time.Second} {
		observers.Add(1)
		go func(interval time.Duration) {
			defer observers.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
					if interval == time.Second {
						_ = writerWorkloadMeasure(probeCtx, "health", func() error { return w.health.Check(probeCtx) })
					} else {
						_ = w.sidebar(probeCtx)
					}
					cancel()

				}
			}
		}(interval)
	}
	return func() { close(stop); observers.Wait() }
}
