package service

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// TestBranchFetcher_CooldownSkipsRepeatedFetches verifies that a second call
// inside the cooldown window returns the cached result instead of re-running
// `git fetch`.
func TestBranchFetcher_CooldownSkipsRepeatedFetches(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepoWithCommit(t, filepath.Join(root, "repo"))

	f := newBranchFetcher(nil)
	first := f.Fetch(context.Background(), repoPath)
	if first.Skipped {
		t.Fatalf("first fetch should not be skipped")
	}
	if first.FetchedAt.IsZero() {
		t.Fatalf("first fetch should set FetchedAt")
	}

	second := f.Fetch(context.Background(), repoPath)
	if !second.Skipped {
		t.Fatalf("second fetch within cooldown should be skipped")
	}
	if !second.FetchedAt.Equal(first.FetchedAt) {
		t.Fatalf("skipped fetch should report the prior FetchedAt: got %v, want %v",
			second.FetchedAt, first.FetchedAt)
	}
}

// TestBranchFetcher_SingleflightCoalesces verifies that concurrent callers
// share a single fetch invocation.
func TestBranchFetcher_SingleflightCoalesces(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepoWithCommit(t, filepath.Join(root, "repo"))

	f := newBranchFetcher(nil)
	const N = 8
	var wg sync.WaitGroup
	results := make([]BranchRefreshResult, N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = f.Fetch(context.Background(), repoPath)
		}(i)
	}
	close(start)
	wg.Wait()

	// All results should share the same FetchedAt — if singleflight is working,
	// only one runFetch ran and its timestamp is broadcast to every caller.
	first := results[0].FetchedAt
	for i := 1; i < N; i++ {
		if !results[i].FetchedAt.Equal(first) {
			t.Fatalf("result %d FetchedAt mismatch: got %v, want %v", i, results[i].FetchedAt, first)
		}
	}
}

// TestBranchFetcher_RecordsErrorOnInvalidRepo verifies that fetch failures
// are reported in the result without panicking.
func TestBranchFetcher_RecordsErrorOnInvalidRepo(t *testing.T) {
	root := t.TempDir() // no .git inside

	f := newBranchFetcher(nil)
	res := f.Fetch(context.Background(), root)
	if res.Err == nil {
		t.Fatalf("expected error fetching non-git directory")
	}
	if res.FetchedAt.IsZero() {
		t.Fatalf("FetchedAt should be set even on error so the cooldown applies")
	}
}

// TestBranchFetcher_BothCooldownAndError verifies that callers within the
// cooldown window after a failed fetch get the cached error instead of
// triggering a new fetch attempt.
func TestBranchFetcher_BothCooldownAndError(t *testing.T) {
	root := t.TempDir()
	f := newBranchFetcher(nil)
	first := f.Fetch(context.Background(), root)
	if first.Err == nil {
		t.Fatalf("first fetch should have errored")
	}
	second := f.Fetch(context.Background(), root)
	if !second.Skipped {
		t.Fatalf("second call within cooldown should be skipped")
	}
	if second.Err == nil {
		t.Fatalf("skipped result should preserve the prior error")
	}
}

// TestBranchFetcher_FirstCallerCancelDoesNotPoisonCache verifies that a
// caller cancelling its context after triggering the singleflight does not
// cause the cached result to record a context.Canceled error. The fetch must
// run to completion using a detached context so subsequent waiters see a real
// outcome.
func TestBranchFetcher_FirstCallerCancelDoesNotPoisonCache(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepoWithCommit(t, filepath.Join(root, "repo"))

	f := newBranchFetcher(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Fetch even runs

	res := f.Fetch(ctx, repoPath)
	if errors.Is(res.Err, context.Canceled) {
		t.Fatalf("cached result must not record context.Canceled from caller's ctx; got %v", res.Err)
	}
}

// makeRepoWithCommit initializes a real git repo with a single commit so that
// `git fetch --all` succeeds (it has nothing to do but exits cleanly).
func makeRepoWithCommit(t *testing.T, path string) string {
	t.Helper()
	if err := exec.Command("git", "init", "-q", path).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	for _, args := range [][]string{
		{"-C", path, "config", "user.email", "t@t.test"},
		{"-C", path, "config", "user.name", "t"},
		{"-C", path, "commit", "--allow-empty", "-q", "-m", "init"},
	} {
		if err := exec.Command("git", args...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return path
}
