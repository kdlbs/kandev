package service

import "sync"

// planLockEntry is one task's serialization point. waiters counts holders and
// queued acquirers so the table knows when it is safe to retire the entry.
type planLockEntry struct {
	mu      sync.Mutex
	waiters int
}

// planLockTable is a per-key mutex table with retirement: an entry exists only
// while at least one goroutine holds or is waiting for it, so the map does not
// grow unboundedly across an unbounded task-id keyspace over the process
// lifetime (unlike the plugin-id / parent-id keyed mutexes elsewhere in this
// codebase, which never delete entries because their keyspace is small and
// long-lived).
//
// Protocol (see docs/specs/tasks/system-design/plan-write-consistency.md,
// "Retiring lock keys"): the outer mutex guards the map; each entry holds its
// own mutex plus a waiter count. acquire takes the outer mutex, finds or
// creates the entry, increments its waiter count, and releases the outer
// mutex before blocking on the entry's own mutex - the count is incremented
// while the map is still guarded, so a concurrent release can never see zero
// and delete the entry out from under an acquirer already en route to it.
// The returned release function unlocks the entry mutex, then takes the outer
// mutex, decrements the count, and deletes the entry only when it reaches
// zero, still under the outer mutex.
type planLockTable struct {
	mu      sync.Mutex
	entries map[string]*planLockEntry
}

func newPlanLockTable() *planLockTable {
	return &planLockTable{entries: make(map[string]*planLockEntry)}
}

// acquire blocks until the per-key lock for key is held and returns a release
// function. The release function is idempotent: only its first call unlocks
// and retires bookkeeping, so a deferred panic-safety release can coexist
// with an earlier explicit release on the success path (the write must
// publish its events after releasing the lock, not before).
func (t *planLockTable) acquire(key string) func() {
	t.mu.Lock()
	entry, ok := t.entries[key]
	if !ok {
		entry = &planLockEntry{}
		t.entries[key] = entry
	}
	entry.waiters++
	t.mu.Unlock()

	entry.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mu.Unlock()
			t.mu.Lock()
			entry.waiters--
			if entry.waiters == 0 {
				delete(t.entries, key)
			}
			t.mu.Unlock()
		})
	}
}
