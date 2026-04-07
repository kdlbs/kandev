package orchestrator

import (
	"strconv"
	"testing"
	"time"
)

func TestGitSnapshotCacheShouldWrite(t *testing.T) {
	c := newGitSnapshotCache()
	now := time.Unix(1_000_000, 0)

	// First call always writes.
	if !c.shouldWrite("s1", "h1", now) {
		t.Fatal("first write should be allowed")
	}
	// Same hash within throttle window: skip.
	if c.shouldWrite("s1", "h1", now.Add(5*time.Second)) {
		t.Fatal("duplicate within throttle window should be skipped")
	}
	// Hash change: write immediately.
	if !c.shouldWrite("s1", "h2", now.Add(5*time.Second)) {
		t.Fatal("hash change should bypass throttle")
	}
	// Same hash after interval: write.
	if !c.shouldWrite("s1", "h2", now.Add(5*time.Second+gitSnapshotPersistInterval)) {
		t.Fatal("write after persist interval should be allowed")
	}
}

func TestGitSnapshotCacheEvictsOldestWhenFull(t *testing.T) {
	c := newGitSnapshotCache()
	c.maxSize = 3
	base := time.Unix(2_000_000, 0)

	// Fill the cache. Each entry has a strictly increasing lastWrite.
	for i := 0; i < 3; i++ {
		if !c.shouldWrite("session-"+strconv.Itoa(i), "h", base.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("fill #%d should write", i)
		}
	}
	if got := len(c.byID); got != 3 {
		t.Fatalf("expected cache size 3, got %d", got)
	}

	// Adding a 4th distinct session should evict the oldest (session-0).
	if !c.shouldWrite("session-3", "h", base.Add(10*time.Second)) {
		t.Fatal("4th distinct session should write")
	}
	if got := len(c.byID); got != 3 {
		t.Fatalf("expected cache size to remain at 3, got %d", got)
	}
	if _, ok := c.byID["session-0"]; ok {
		t.Error("session-0 should have been evicted as the oldest")
	}
	if _, ok := c.byID["session-3"]; !ok {
		t.Error("session-3 should be present after insert")
	}
}

func TestGitSnapshotCacheForget(t *testing.T) {
	c := newGitSnapshotCache()
	now := time.Unix(3_000_000, 0)
	c.shouldWrite("s1", "h", now)
	c.forget("s1")
	if _, ok := c.byID["s1"]; ok {
		t.Error("forget did not remove the entry")
	}
	// Forgetting an unknown session is a no-op.
	c.forget("unknown")
}
