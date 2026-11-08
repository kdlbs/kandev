package coordinator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

type fakeChangeReader struct {
	calls atomic.Int32
	snap  mergestate.Snapshot
	err   error
	block chan struct{}
}

func (f *fakeChangeReader) ReadChange(_ context.Context, _ string, _ mergestate.ChangeKey) (mergestate.Snapshot, error) {
	f.calls.Add(1)
	if f.block != nil {
		<-f.block
	}
	return f.snap, f.err
}

var observerKey = mergestate.ChangeKey{Provider: "github", Host: "github.com", RepoID: "r1", Number: 42}

func newTestObserver(r ChangeReader) (*ChangeObserver, *time.Time) {
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	o := NewChangeObserver(r)
	o.now = func() time.Time { return now }
	return o, &now
}

func TestObservationReuseUnder60s(t *testing.T) {
	r := &fakeChangeReader{snap: mergestate.Snapshot{HeadSHA: "h1"}}
	o, now := newTestObserver(r)
	for i := 0; i < 3; i++ {
		if _, err := o.Read(context.Background(), "ws", observerKey, false); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(20 * time.Second)
	}
	if got := r.calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1 within 60s", got)
	}
	*now = now.Add(5 * time.Second)
	if _, err := o.Read(context.Background(), "ws", observerKey, false); err != nil {
		t.Fatal(err)
	}
	if got := r.calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want a re-read after 60s", got)
	}
}

func TestFailedReadNotCachedRetryBypasses(t *testing.T) {
	r := &fakeChangeReader{snap: mergestate.Snapshot{HeadSHA: "h1"}}
	o, now := newTestObserver(r)
	if _, err := o.Read(context.Background(), "ws", observerKey, false); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(70 * time.Second)
	r.err = errors.New("rate limited")
	res, err := o.Read(context.Background(), "ws", observerKey, false)
	if err == nil {
		t.Fatal("want the read error")
	}
	if res.LastGood == nil || res.LastGood.HeadSHA != "h1" || !res.LastGoodAt.Equal(now.Add(-70*time.Second)) {
		t.Fatalf("last successful values lost: %+v", res)
	}
	r.err = nil
	if _, err := o.Read(context.Background(), "ws", observerKey, false); err != nil {
		t.Fatal(err)
	}
	if got := r.calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want the failed read not cached", got)
	}
	if _, err := o.Read(context.Background(), "ws", observerKey, true); err != nil {
		t.Fatal(err)
	}
	if got := r.calls.Load(); got != 4 {
		t.Fatalf("calls = %d, want Retry to bypass reuse", got)
	}
}

func TestSingleFlightRead(t *testing.T) {
	r := &fakeChangeReader{snap: mergestate.Snapshot{HeadSHA: "h1"}, block: make(chan struct{})}
	o, _ := newTestObserver(r)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := o.Read(context.Background(), "ws", observerKey, true); err != nil {
				t.Error(err)
			}
		}()
	}
	for r.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(r.block)
	wg.Wait()
	if got := r.calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want one shared call", got)
	}
}

func TestObservationKeyedByWorkspace(t *testing.T) {
	r := &fakeChangeReader{}
	o, _ := newTestObserver(r)
	_, _ = o.Read(context.Background(), "ws-a", observerKey, false)
	_, _ = o.Read(context.Background(), "ws-b", observerKey, false)
	if got := r.calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want separate reads per workspace credential", got)
	}
}
