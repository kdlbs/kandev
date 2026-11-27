package orchestrator

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCancelInFlightMutexContextWaitCanBeCancelled(t *testing.T) {
	mutex := newCancelInFlightMutex()
	mutex.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- mutex.LockContext(ctx) }()
	waitForCancelInFlightMutexWaiters(t, mutex, 1)
	cancel()
	select {
	case err := <-result:
		if err != context.Canceled {
			t.Fatalf("context-aware lock error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("context-aware lock did not stop waiting after cancellation")
	}
	mutex.Unlock()
	if !mutex.TryLock() {
		t.Fatal("cancelled waiter acquired the lock after returning")
	}
	mutex.Unlock()
}

func TestCancelInFlightMutexHandsOffInQueueOrder(t *testing.T) {
	mutex := newCancelInFlightMutex()
	mutex.Lock()
	acquired := make(chan int, 2)
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	var firstReleaseOnce, secondReleaseOnce sync.Once
	releaseFirst := func() { firstReleaseOnce.Do(func() { close(firstRelease) }) }
	releaseSecond := func() { secondReleaseOnce.Do(func() { close(secondRelease) }) }
	t.Cleanup(func() {
		releaseFirst()
		releaseSecond()
		<-firstDone
		<-secondDone
	})
	go lockForTest(mutex, 1, acquired, firstRelease, firstDone)
	waitForCancelInFlightMutexWaiters(t, mutex, 1)
	go lockForTest(mutex, 2, acquired, secondRelease, secondDone)
	waitForCancelInFlightMutexWaiters(t, mutex, 2)

	mutex.Unlock()
	select {
	case got := <-acquired:
		if got != 1 {
			t.Fatalf("first lock owner = %d, want first queued waiter", got)
		}
	case <-time.After(time.Second):
		t.Fatal("first queued waiter did not acquire the lock")
	}
	select {
	case got := <-acquired:
		t.Fatalf("lock passed to waiter %d before the first owner released it", got)
	default:
	}
	releaseFirst()
	select {
	case got := <-acquired:
		if got != 2 {
			t.Fatalf("second lock owner = %d, want second queued waiter", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second queued waiter did not acquire the lock")
	}
	releaseSecond()
}

func lockForTest(
	mutex *cancelInFlightMutex,
	id int,
	acquired chan<- int,
	release <-chan struct{},
	done chan<- struct{},
) {
	if err := mutex.LockContext(context.Background()); err != nil {
		close(done)
		return
	}
	acquired <- id
	<-release
	mutex.Unlock()
	close(done)
}

func waitForCancelInFlightMutexWaiters(t *testing.T, mutex *cancelInFlightMutex, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mutex.stateMu.Lock()
		got := len(mutex.waiters)
		mutex.stateMu.Unlock()
		if got == want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("cancel-in-flight lock waiters did not reach %d", want)
}
