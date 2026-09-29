package coordinator

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestRecordUnattendedDenial_ConcurrentRedeliveryCountsOnce(t *testing.T) {
	s := newTestStore(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)

	const workers = 16
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
	if got := denialRows(t, s, "turn-1"); got != 1 {
		t.Fatalf("denial rows = %d, want 1", got)
	}
}

func TestRecordUnattendedDenial_ConcurrentDistinctPendingsAllCounted(t *testing.T) {
	s := newTestStore(t)
	c := newTestCoordinator(t, s, "ws-1")
	insertBoundTurn(t, s, c, "turn-1", "conv", "sess", "st-1", nil)

	const workers = 12
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, errs[i] = s.RecordUnattendedDenial(context.Background(), "conv", "sess", fmt.Sprintf("p-%d", i), "st-1")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	if got := deniedCount(t, s, "turn-1"); got != workers {
		t.Fatalf("denied_permissions = %d, want %d", got, workers)
	}
}
