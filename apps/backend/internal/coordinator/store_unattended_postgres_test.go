package coordinator

import (
	"context"
	"sync"
	"testing"
)

func TestRecordUnattendedDenialPostgres_CountsOncePerPendingID(t *testing.T) {
	s := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		id, matched, err := s.RecordUnattendedDenial(ctx, "conv", "sess", "pending-1", "st-1")
		if err != nil || !matched || id != "turn-1" {
			t.Fatalf("attempt %d: id=%q matched=%v err=%v", i, id, matched, err)
		}
	}
	if got := deniedCount(t, s, "turn-1"); got != 1 {
		t.Fatalf("denied_permissions = %d, want 1", got)
	}
	open, err := s.ListOpenDenials(ctx, c.ID)
	if err != nil || len(open) != 1 || open[0].PendingID != "pending-1" {
		t.Fatalf("ListOpenDenials = %+v err=%v", open, err)
	}
}

func TestRecordUnattendedDenialPostgres_UnboundWindowAndSettledTurn(t *testing.T) {
	s := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-open", "conv", "sess", nil, nil)
	insertBoundTurn(t, s, c, "turn-done", "conv2", "sess2", "st-2", "completed")
	ctx := context.Background()

	if id, matched, err := s.RecordUnattendedDenial(ctx, "conv", "sess", "p1", "any"); err != nil || !matched || id != "turn-open" {
		t.Fatalf("unbound window: id=%q matched=%v err=%v", id, matched, err)
	}
	if _, matched, err := s.RecordUnattendedDenial(ctx, "conv2", "sess2", "p1", "st-2"); err != nil || matched {
		t.Fatalf("settled turn must not match: matched=%v err=%v", matched, err)
	}
	if got := denialRows(t, s, "turn-done"); got != 0 {
		t.Fatalf("settled turn recorded %d denials", got)
	}
}

func TestRecordUnattendedDenialPostgres_ConcurrentRedeliveryCountsOnce(t *testing.T) {
	s := newMultiConnStorePostgres(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	matches := make([]bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, matches[i], errs[i] = s.RecordUnattendedDenial(context.Background(), "conv", "sess", "pending-1", "st-1")
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil || !matches[i] {
			t.Fatalf("worker %d: matched=%v err=%v", i, matches[i], errs[i])
		}
	}
	if got := deniedCount(t, s, "turn-1"); got != 1 {
		t.Fatalf("denied_permissions = %d, want 1", got)
	}
}
