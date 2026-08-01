package scheduler_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/runs/scheduler"
)

// fakeProcessor counts how many times Tick is invoked. Each call
// records a timestamp so the latency test can measure how long
// elapsed between the signal landing and the processor running.
type fakeProcessor struct {
	calls atomic.Int64
	last  atomic.Int64 // unix nano
}

func (f *fakeProcessor) Tick(_ context.Context) {
	f.calls.Add(1)
	f.last.Store(time.Now().UnixNano())
}

type blockingProcessor struct {
	calls   atomic.Int64
	entered chan struct{}
	second  chan struct{}
	release chan struct{}
}

func (p *blockingProcessor) Tick(_ context.Context) {
	switch p.calls.Add(1) {
	case 1:
		close(p.entered)
	case 2:
		close(p.second)
	}
	<-p.release
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// TestScheduler_TickFiresProcessor pins that the periodic tick path
// drives the processor at the configured interval even when no
// signals are emitted.
func TestScheduler_TickFiresProcessor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc := &fakeProcessor{}
	s := scheduler.New(proc, nil, 20*time.Millisecond, newTestLogger(t))
	go s.Start(ctx)
	t.Cleanup(func() { _ = s.Stop() })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if proc.calls.Load() >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected ≥2 ticks within 500ms, got %d", proc.calls.Load())
}

// TestScheduler_SignalDrivesClaimUnder100ms is the B3.7 latency
// pin: when a row signals via the channel, the processor's Tick
// runs within 100ms — sub-second latency that wouldn't be possible
// with the 5s tick alone.
func TestScheduler_SignalDrivesClaimUnder100ms(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan struct{}, 1)
	proc := &fakeProcessor{}
	// Long tick interval so we know any Tick we observe came from
	// the signal path, not the safety-net timer.
	s := scheduler.New(proc, signalCh, 5*time.Second, newTestLogger(t))
	go s.Start(ctx)
	t.Cleanup(func() { _ = s.Stop() })

	// Give the goroutine a moment to enter the select.
	time.Sleep(10 * time.Millisecond)

	start := time.Now()
	signalCh <- struct{}{}

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if proc.calls.Load() >= 1 {
			elapsed := time.Since(start)
			if elapsed > 100*time.Millisecond {
				t.Fatalf("signal-driven tick took %s, want <100ms", elapsed)
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("signal did not drive Tick within 100ms (calls=%d)", proc.calls.Load())
}

// TestScheduler_StopsOnContextCancel pins clean shutdown — the
// scheduler exits its loop when the context is cancelled and stops
// invoking the processor afterwards.
func TestScheduler_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	proc := &fakeProcessor{}
	s := scheduler.New(proc, nil, 20*time.Millisecond, newTestLogger(t))
	go s.Start(ctx)
	t.Cleanup(func() { _ = s.Stop() })

	time.Sleep(80 * time.Millisecond)
	cancel()

	pre := proc.calls.Load()
	time.Sleep(120 * time.Millisecond)
	post := proc.calls.Load()
	// Allow at most one extra tick (race between cancel and the
	// next ticker firing); anything more means the loop kept running.
	if post > pre+1 {
		t.Errorf("scheduler kept ticking after cancel: pre=%d post=%d", pre, post)
	}
}

func TestScheduler_StartIsIdempotentAndStopWaitsForActiveTick(t *testing.T) {
	ctx := context.Background()
	signalCh := make(chan struct{}, 2)
	proc := &blockingProcessor{
		entered: make(chan struct{}),
		second:  make(chan struct{}),
		release: make(chan struct{}),
	}
	s := scheduler.New(proc, signalCh, time.Hour, newTestLogger(t))
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(proc.release) }) }
	t.Cleanup(func() {
		release()
		_ = s.Stop()
	})
	go s.Start(ctx)
	go s.Start(ctx)

	signalCh <- struct{}{}
	select {
	case <-proc.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("scheduler did not start the processor")
	}

	signalCh <- struct{}{}
	select {
	case <-proc.second:
		t.Fatal("duplicate Start launched a second scheduler loop")
	case <-time.After(50 * time.Millisecond):
	}

	stopDone := make(chan struct{})
	go func() {
		if err := s.Stop(); err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("Stop returned while a processor tick was still active")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-stopDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop did not join the processor loop")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestScheduler_ParentCancellationDrainsActiveTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	signalCh := make(chan struct{}, 1)
	proc := &blockingProcessor{
		entered: make(chan struct{}),
		second:  make(chan struct{}),
		release: make(chan struct{}),
	}
	s := scheduler.New(proc, signalCh, time.Hour, newTestLogger(t))
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(proc.release) }) }
	t.Cleanup(func() {
		release()
		_ = s.Stop()
	})
	go s.Start(ctx)
	signalCh <- struct{}{}
	select {
	case <-proc.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("scheduler did not start the processor")
	}

	cancel()
	stopDone := make(chan struct{})
	go func() {
		if err := s.Stop(); err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("parent cancellation did not wait for the active tick")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-stopDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop did not finish after the active tick drained")
	}
}
