package ctxmutex

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestContextMutexDeadlineDoesNotAcquireLater(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mutex Mutex
		mutex.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			err := mutex.LockContext(ctx)
			if err == nil {
				mutex.Unlock()
			}
			done <- err
		}()
		synctest.Wait()
		time.Sleep(time.Second)
		if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("contended acquisition = %v, want deadline exceeded", err)
		}
		mutex.Unlock()
		synctest.Wait()
		if !mutex.TryLock() {
			t.Fatal("the expired waiter retained ownership after the original owner released it")
		}
		mutex.Unlock()
	})
}

func TestContextMutexRejectsCancelledContextWithoutTakingFreeLock(t *testing.T) {
	var mutex Mutex
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mutex.LockContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquisition = %v", err)
	}
	if !mutex.TryLock() {
		t.Fatal("a cancelled caller acquired the free mutex")
	}
	mutex.Unlock()
}

func TestContextMutexSerializesOwnersAndAllowsRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mutex Mutex
		mutex.Lock()
		acquired := make(chan struct{})
		release := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			mutex.Lock()
			close(acquired)
			<-release
			mutex.Unlock()
		}()
		synctest.Wait()
		if mutex.TryLock() {
			t.Fatal("TryLock bypassed the current owner")
		}
		mutex.Unlock()
		<-acquired
		if mutex.TryLock() {
			t.Fatal("two callers acquired the mutex")
		}
		close(release)
		<-done
		if err := mutex.LockContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		mutex.Unlock()
	})
}
