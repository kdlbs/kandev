package cron

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type recordHandler struct {
	name  string
	calls atomic.Int32
	err   error
}

func (h *recordHandler) Name() string { return h.name }
func (h *recordHandler) Tick(_ context.Context) error {
	h.calls.Add(1)
	return h.err
}

type blockingHandler struct {
	calls   atomic.Int32
	entered chan struct{}
	second  chan struct{}
	release chan struct{}
}

func (h *blockingHandler) Name() string { return "blocking" }
func (h *blockingHandler) Tick(_ context.Context) error {
	switch h.calls.Add(1) {
	case 1:
		close(h.entered)
	case 2:
		close(h.second)
	}
	<-h.release
	return nil
}

func TestNewLoop_FallsBackToDefaultInterval(t *testing.T) {
	l := NewLoop(0, logger.Default())
	if l.interval != DefaultTickInterval {
		t.Fatalf("interval = %v, want %v", l.interval, DefaultTickInterval)
	}
}

func TestNewLoop_IgnoresNilHandlers(t *testing.T) {
	handler := &recordHandler{name: "configured"}
	l := NewLoop(time.Second, logger.Default(), handler, nil)
	if len(l.handlers) != 1 || l.handlers[0] != handler {
		t.Fatalf("handlers = %#v, want only configured handler", l.handlers)
	}
}

func TestLoop_FiresEachHandlerPerTick(t *testing.T) {
	a := &recordHandler{name: "a"}
	b := &recordHandler{name: "b"}
	l := NewLoop(20*time.Millisecond, logger.Default(), a, b)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		l.Start(ctx)
		close(done)
	}()
	t.Cleanup(func() { _ = l.Stop() })

	// Wait for at least two ticks of each handler.
	deadline := time.After(500 * time.Millisecond)
	for a.calls.Load() < 2 || b.calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out: a=%d b=%d", a.calls.Load(), b.calls.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	<-done
}

func TestLoop_ErrorFromHandlerDoesNotStopLoop(t *testing.T) {
	bad := &recordHandler{name: "bad", err: errors.New("boom")}
	good := &recordHandler{name: "good"}
	l := NewLoop(15*time.Millisecond, logger.Default(), bad, good)
	ctx, cancel := context.WithCancel(context.Background())
	go l.Start(ctx)
	t.Cleanup(func() { _ = l.Stop() })
	defer cancel()

	deadline := time.After(500 * time.Millisecond)
	for good.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("good handler did not keep ticking after bad handler error: %d", good.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
	if bad.calls.Load() == 0 {
		t.Fatal("bad handler should have ticked too")
	}
}

func TestLoop_PanicInHandlerIsRecovered(t *testing.T) {
	panicker := handlerFunc{
		name: "panicker",
		tick: func(_ context.Context) error { panic("kaboom") },
	}
	good := &recordHandler{name: "good"}
	l := NewLoop(15*time.Millisecond, logger.Default(), panicker, good)
	ctx, cancel := context.WithCancel(context.Background())
	go l.Start(ctx)
	t.Cleanup(func() { _ = l.Stop() })
	defer cancel()

	deadline := time.After(500 * time.Millisecond)
	for good.calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("good handler did not survive panic of peer: %d", good.calls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestLoop_StartIsIdempotentAndStopWaitsForFanOut(t *testing.T) {
	h := &blockingHandler{
		entered: make(chan struct{}),
		second:  make(chan struct{}),
		release: make(chan struct{}),
	}
	l := NewLoop(time.Millisecond, logger.Default(), h)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(h.release) }) }
	t.Cleanup(func() {
		release()
		_ = l.Stop()
	})
	ctx := context.Background()
	go l.Start(ctx)
	go l.Start(ctx)

	select {
	case <-h.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cron loop did not start the handler")
	}
	select {
	case <-h.second:
		t.Fatal("duplicate Start launched a second cron loop")
	case <-time.After(50 * time.Millisecond):
	}

	stopDone := make(chan struct{})
	go func() {
		if err := l.Stop(); err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("Stop returned while handler fan-out was still active")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-stopDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop did not join cron handler fan-out")
	}
	if err := l.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestLoop_ParentCancellationDrainsFanOut(t *testing.T) {
	h := &blockingHandler{
		entered: make(chan struct{}),
		second:  make(chan struct{}),
		release: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := NewLoop(time.Millisecond, logger.Default(), h)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(h.release) }) }
	t.Cleanup(func() {
		release()
		_ = l.Stop()
	})
	go l.Start(ctx)

	select {
	case <-h.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cron loop did not start the handler")
	}
	cancel()
	stopDone := make(chan struct{})
	go func() {
		if err := l.Stop(); err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("parent cancellation did not wait for handler fan-out")
	case <-time.After(50 * time.Millisecond):
	}

	release()
	select {
	case <-stopDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop did not finish after handler fan-out drained")
	}
}

type cancelBlockingHandler struct {
	entered chan struct{}
	release chan struct{}
}

func (h *cancelBlockingHandler) Name() string { return "cancel_blocking" }
func (h *cancelBlockingHandler) Tick(ctx context.Context) error {
	close(h.entered)
	<-ctx.Done()
	<-h.release
	return nil
}

func TestLoop_ConcurrentStopUsesOneLifecycleTransition(t *testing.T) {
	handler := &cancelBlockingHandler{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	loop := NewLoop(time.Millisecond, logger.Default(), handler)
	loop.Start(context.Background())

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(handler.release) }) }
	t.Cleanup(func() {
		release()
		_ = loop.Stop()
	})

	select {
	case <-handler.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cron loop did not enter handler tick")
	}

	firstCancel := make(chan struct{})
	secondCancel := make(chan struct{})
	var cancelCalls atomic.Int32
	loop.mu.Lock()
	originalCancel := loop.cancel
	loop.cancel = func() {
		switch cancelCalls.Add(1) {
		case 1:
			close(firstCancel)
		case 2:
			close(secondCancel)
		}
		originalCancel()
	}
	stopDone := make(chan error, 2)
	go func() { stopDone <- loop.Stop() }()
	go func() { stopDone <- loop.Stop() }()
	loop.mu.Unlock()

	select {
	case <-firstCancel:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop did not cancel the cron loop")
	}
	select {
	case <-secondCancel:
		t.Fatal("concurrent Stop started a second lifecycle transition")
	case <-time.After(100 * time.Millisecond):
	}

	release()
	for range 2 {
		select {
		case err := <-stopDone:
			if err != nil {
				t.Fatalf("Stop returned error: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("concurrent Stop did not return after fan-out drained")
		}
	}
}

// handlerFunc adapts a function pair to the Handler interface so tests
// can express tiny throwaway handlers without ceremony.
type handlerFunc struct {
	name string
	tick func(context.Context) error
}

func (h handlerFunc) Name() string                   { return h.name }
func (h handlerFunc) Tick(ctx context.Context) error { return h.tick(ctx) }
