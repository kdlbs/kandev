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
	if !c.shouldWrite("env-1", "repo", "h1", now) {
		t.Fatal("first write should be allowed")
	}
	// Same hash within throttle window: skip.
	if c.shouldWrite("env-1", "repo", "h1", now.Add(5*time.Second)) {
		t.Fatal("duplicate within throttle window should be skipped")
	}
	// Hash change: write immediately.
	if !c.shouldWrite("env-1", "repo", "h2", now.Add(5*time.Second)) {
		t.Fatal("hash change should bypass throttle")
	}
	// Same hash after interval: write.
	if !c.shouldWrite("env-1", "repo", "h2", now.Add(5*time.Second+gitSnapshotPersistInterval)) {
		t.Fatal("write after persist interval should be allowed")
	}
}

func TestGitSnapshotCacheEvictsOldestWhenFull(t *testing.T) {
	c := newGitSnapshotCache()
	c.maxSize = 3
	base := time.Unix(2_000_000, 0)

	// Fill the cache. Each entry has a strictly increasing lastWrite.
	for i := 0; i < 3; i++ {
		if !c.shouldWrite("env-"+strconv.Itoa(i), "repo", "h", base.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("fill #%d should write", i)
		}
	}
	if got := len(c.byID); got != 3 {
		t.Fatalf("expected cache size 3, got %d", got)
	}

	// Adding a 4th distinct session should evict the oldest (session-0).
	if !c.shouldWrite("env-3", "repo", "h", base.Add(10*time.Second)) {
		t.Fatal("4th distinct session should write")
	}
	if got := len(c.byID); got != 3 {
		t.Fatalf("expected cache size to remain at 3, got %d", got)
	}
	if _, ok := c.byID[gitSnapshotCacheKey("env-0", "repo")]; ok {
		t.Error("env-0 should have been evicted as the oldest")
	}
	if _, ok := c.byID[gitSnapshotCacheKey("env-3", "repo")]; !ok {
		t.Error("env-3 should be present after insert")
	}
}

func TestGitSnapshotCacheForget(t *testing.T) {
	c := newGitSnapshotCache()
	now := time.Unix(3_000_000, 0)
	c.shouldWrite("env-1", "repo", "h", now)
	c.forget("env-1")
	if _, ok := c.byID[gitSnapshotCacheKey("env-1", "repo")]; ok {
		t.Error("forget did not remove the entry")
	}
	// Forgetting an unknown session is a no-op.
	c.forget("unknown")
}

func TestGitSnapshotCacheScopesSiblingSessionsByEnvironmentAndRepository(t *testing.T) {
	c := newGitSnapshotCache()
	now := time.Unix(4_000_000, 0)
	if !c.shouldWrite("env-shared", "repo", "hash", now) {
		t.Fatal("first environment observation should write")
	}
	if c.shouldWrite("env-shared", "repo", "hash", now.Add(time.Second)) {
		t.Fatal("same environment and repository should be throttled")
	}
	if !c.shouldWrite("env-shared", "other-repo", "hash", now.Add(time.Second)) {
		t.Fatal("a different repository should have its own throttle entry")
	}
	if !c.shouldWrite("env-sibling", "repo", "hash", now.Add(time.Second)) {
		t.Fatal("a different environment should have its own throttle entry")
	}
}
