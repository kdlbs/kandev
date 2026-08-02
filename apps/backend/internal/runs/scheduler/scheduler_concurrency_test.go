package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type cancelBlockingProcessor struct {
	entered chan struct{}
	release chan struct{}
}

func (p *cancelBlockingProcessor) Tick(ctx context.Context) {
	close(p.entered)
	<-ctx.Done()
	<-p.release
}

func TestScheduler_ConcurrentStopUsesOneLifecycleTransition(t *testing.T) {
	signalCh := make(chan struct{}, 1)
	processor := &cancelBlockingProcessor{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	scheduler := New(processor, signalCh, time.Hour, logger.Default())
	scheduler.Start(context.Background())

	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(processor.release) }) }
	t.Cleanup(func() {
		release()
		_ = scheduler.Stop()
	})

	signalCh <- struct{}{}
	select {
	case <-processor.entered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("scheduler did not enter processor tick")
	}

	firstCancel := make(chan struct{})
	secondCancel := make(chan struct{})
	var cancelCalls atomic.Int32
	scheduler.mu.Lock()
	originalCancel := scheduler.cancel
	scheduler.cancel = func() {
		switch cancelCalls.Add(1) {
		case 1:
			close(firstCancel)
		case 2:
			close(secondCancel)
		}
		originalCancel()
	}
	stopDone := make(chan error, 2)
	go func() { stopDone <- scheduler.Stop() }()
	go func() { stopDone <- scheduler.Stop() }()
	scheduler.mu.Unlock()

	select {
	case <-firstCancel:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop did not cancel the scheduler")
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
			t.Fatal("concurrent Stop did not return after the tick drained")
		}
	}
}
