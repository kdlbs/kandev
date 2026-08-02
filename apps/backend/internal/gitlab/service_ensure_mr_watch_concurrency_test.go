package gitlab

import (
	"context"
	"sync"
	"testing"
)

// EnsureMRWatch is a SELECT-then-INSERT, so concurrent callers for one
// (session, repository, branch) triple can all miss the existing row and race
// into the INSERT, where UNIQUE(session_id, repository_id, branch) rejects
// every loser. That race is reachable in production: push detection runs in
// its own goroutine while the on-demand gitlab.check_session_mr action can be
// answering for the same session. Losers must resolve to the winner's row
// rather than surfacing a constraint error.
func TestEnsureMRWatch_ConcurrentCallsAreSafe(t *testing.T) {
	svc := newServiceWithStore(t)
	store := svc.requireStore()
	ctx := context.Background()

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	ids := make([]string, goroutines)
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			w, err := svc.EnsureMRWatch(ctx, "sess-1", "task-1", "repo-1", "group/proj", 7, "feat/a")
			errs[idx] = err
			if w != nil {
				ids[idx] = w.ID
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: EnsureMRWatch returned %v, want nil", i, err)
		}
	}

	// Every caller must observe the same single row.
	for i, id := range ids {
		if id == "" {
			t.Errorf("goroutine %d: got no watch back", i)
			continue
		}
		if id != ids[0] {
			t.Errorf("goroutine %d returned watch %q, want the single shared row %q", i, id, ids[0])
		}
	}

	var count int
	if err := store.ro.Get(&count, `SELECT COUNT(*) FROM gitlab_mr_watches`); err != nil {
		t.Fatalf("count watches: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 watch row after %d concurrent calls, got %d", goroutines, count)
	}
}
