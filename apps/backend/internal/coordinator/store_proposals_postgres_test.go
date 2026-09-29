package coordinator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// TestInsertProposal_Postgres_ConcurrentCapEnforcement is the PostgreSQL twin
// of TestInsertProposal_ConcurrentCapEnforcement (Build decision 11's
// mandated 30-concurrent test on both dialects): exercises the real
// SELECT ... FOR UPDATE row lock instead of SQLite's single-connection
// writer pool, and must still land exactly 25 successes and 5 refusals.
func TestInsertProposal_Postgres_ConcurrentCapEnforcement(t *testing.T) {
	store := newTestStorePostgres(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")

	const attempts = 30
	start := make(chan struct{})
	var wg sync.WaitGroup
	var succeeded, capped, other int64
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			<-start
			p := &Proposal{CoordinatorID: c.ID, WorkspaceID: "ws-1", Spec: sampleSpec()}
			err := store.InsertProposal(ctx, p)
			switch {
			case err == nil:
				atomic.AddInt64(&succeeded, 1)
			case errors.Is(err, ErrCoordinatorProposalCapReached):
				atomic.AddInt64(&capped, 1)
			default:
				atomic.AddInt64(&other, 1)
				t.Errorf("InsertProposal: unexpected error %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if other != 0 {
		t.Fatalf("unexpected errors: %d", other)
	}
	if succeeded != maxOpenProposals {
		t.Fatalf("succeeded = %d, want %d", succeeded, maxOpenProposals)
	}
	if capped != attempts-maxOpenProposals {
		t.Fatalf("capped = %d, want %d", capped, attempts-maxOpenProposals)
	}

	count, err := store.CountOpenProposals(ctx, c.ID)
	if err != nil {
		t.Fatalf("CountOpenProposals: %v", err)
	}
	if count != maxOpenProposals {
		t.Fatalf("CountOpenProposals = %d, want %d", count, maxOpenProposals)
	}
}

func TestListProposals_Postgres_PendingReturnsEveryOpenProposalBeyondNewestFifty(t *testing.T) {
	assertListPendingReturnsEveryOpenProposal(t, newTestStorePostgres(t))
}
