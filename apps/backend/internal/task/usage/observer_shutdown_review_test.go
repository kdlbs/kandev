package usage

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestObserverShutdownBoundsUncooperativeCallAndKeepsCompletionUnknown(t *testing.T) {
	w := NewWriter(&fakeUsageRepo{}, nil, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	exited := make(chan struct{})
	var calls atomic.Int32
	w.SetRecordedObserver(func(context.Context, string, string) { calls.Add(1); close(entered); <-release; close(exited) })
	w.Start()
	w.admit(sessionPayload("blocked", "task", "session"))
	<-entered
	w.admit(sessionPayload("queued", "task", "session"))
	stopped := make(chan struct{})
	go func() { w.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(drainDeadline + time.Second):
		close(release)
		<-stopped
		t.Fatal("uncooperative callback blocked shutdown past drain bound")
	}
	select {
	case <-exited:
		t.Fatal("shutdown claimed an unfinished callback completed")
	default:
	}
	w.mu.Lock()
	activeDone := w.obsActiveDone
	w.mu.Unlock()
	if activeDone == nil {
		t.Fatal("active callback handle was not retained")
	}
	close(release)
	<-exited
	<-activeDone
	w.Stop()
	if calls.Load() != 1 {
		t.Fatal("another callback admitted after shutdown")
	}
}
