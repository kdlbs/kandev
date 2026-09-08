package service

import (
	"sync"
	"testing"
	"time"
)

// TestPlanLockTableMutualExclusion asserts that two acquires for the same key
// never overlap: the second acquire only completes once the first release runs.
func TestPlanLockTableMutualExclusion(t *testing.T) {
	table := newPlanLockTable()

	release1 := table.acquire("task-a")

	acquired2 := make(chan struct{})
	go func() {
		release2 := table.acquire("task-a")
		close(acquired2)
		release2()
	}()

	select {
	case <-acquired2:
		t.Fatal("second acquire completed while the first holder still holds the lock")
	case <-time.After(50 * time.Millisecond):
	}

	release1()

	select {
	case <-acquired2:
	case <-time.After(time.Second):
		t.Fatal("second acquire never completed after the first release")
	}
}

// TestPlanLockTableIndependentKeysDoNotBlock asserts distinct keys never wait
// on each other (AC-002.6: a lock bounds one operation, not the system).
func TestPlanLockTableIndependentKeysDoNotBlock(t *testing.T) {
	table := newPlanLockTable()
	releaseA := table.acquire("task-a")
	defer releaseA()

	done := make(chan struct{})
	go func() {
		releaseB := table.acquire("task-b")
		releaseB()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("acquire for an unrelated key blocked on a held key")
	}
}

// TestPlanLockTableEntryRetiredAtZeroRefcount asserts the map entry for a key
// is deleted once every acquirer has released it, so the map does not grow
// unboundedly across an unbounded task-id keyspace.
func TestPlanLockTableEntryRetiredAtZeroRefcount(t *testing.T) {
	table := newPlanLockTable()
	release := table.acquire("task-a")

	table.mu.Lock()
	_, present := table.entries["task-a"]
	table.mu.Unlock()
	if !present {
		t.Fatal("entry missing while held")
	}

	release()

	table.mu.Lock()
	_, present = table.entries["task-a"]
	table.mu.Unlock()
	if present {
		t.Fatal("entry not retired after the only holder released it")
	}
}

// TestPlanLockTableReleaseIsIdempotent asserts calling the returned release
// function more than once is a no-op after the first call, so a deferred
// safety-net release can coexist with an earlier explicit release on the
// success path (needed so publishing can happen after the lock is released,
// per docs/specs/tasks/system-design/plan-write-consistency.md).
func TestPlanLockTableReleaseIsIdempotent(t *testing.T) {
	table := newPlanLockTable()
	release := table.acquire("task-a")
	release()
	release()
	release()

	// The entry must still be gone (refcount not double-decremented into negative).
	table.mu.Lock()
	_, present := table.entries["task-a"]
	table.mu.Unlock()
	if present {
		t.Fatal("entry present after idempotent releases")
	}

	// And the key must still be acquirable (mutex not left locked).
	done := make(chan struct{})
	go func() {
		r := table.acquire("task-a")
		r()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("key could not be re-acquired after idempotent releases")
	}
}

// TestPlanLockTableNoEntryLeakUnderConcurrentWaiters exercises many concurrent
// acquire/release cycles on the same key and asserts the map is empty at the
// end - i.e. no waiter's increment/decrement raced the retirement delete.
func TestPlanLockTableNoEntryLeakUnderConcurrentWaiters(t *testing.T) {
	table := newPlanLockTable()
	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 20
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				release := table.acquire("task-shared")
				release()
			}
		}()
	}
	wg.Wait()

	table.mu.Lock()
	remaining := len(table.entries)
	table.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("entries remaining = %d, want 0", remaining)
	}
}
