package coordinator

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

const (
	// observationReuse is how long a successful read of a change is reused.
	observationReuse = 60 * time.Second
	// providerReadTimeout bounds one shared provider read.
	providerReadTimeout = 30 * time.Second
)

// ChangeReader reads a change through the workspace's provider credentials. A
// rate-limited, partial or failed response is an error, never a snapshot.
type ChangeReader interface {
	ReadChange(ctx context.Context, workspaceID string, key mergestate.ChangeKey) (mergestate.Snapshot, error)
}

// ReadResult is the outcome of an observer read.
type ReadResult struct {
	Snapshot  mergestate.Snapshot
	StartedAt time.Time
	// LastGood is the last successful snapshot, set only when this read failed.
	LastGood   *mergestate.Snapshot
	LastGoodAt time.Time
}

type observed struct {
	snap      mergestate.Snapshot
	startedAt time.Time
}

type flight struct {
	done chan struct{}
	res  observed
	err  error
}

// ChangeObserver reads changes with a 60-second reuse of successful reads,
// single flight per (workspace, change) and no caching of failures. The last
// successful snapshot stays available beside a failure.
type ChangeObserver struct {
	reader ChangeReader
	now    func() time.Time

	mu       sync.Mutex
	good     map[string]observed
	inflight map[string]*flight
}

// NewChangeObserver builds an observer over a provider reader.
func NewChangeObserver(r ChangeReader) *ChangeObserver {
	return &ChangeObserver{reader: r, now: time.Now, good: map[string]observed{}, inflight: map[string]*flight{}}
}

func observerKeyOf(workspaceID string, key mergestate.ChangeKey) string {
	return workspaceID + "|" + key.String()
}

// Read returns a fresh-enough snapshot. force bypasses the reuse (Retry and
// every pre-dispatch recheck) but still joins a read already in flight.
func (o *ChangeObserver) Read(ctx context.Context, workspaceID string, key mergestate.ChangeKey, force bool) (ReadResult, error) {
	k := observerKeyOf(workspaceID, key)
	o.mu.Lock()
	if g, ok := o.good[k]; ok && !force && o.now().Sub(g.startedAt) < observationReuse {
		o.mu.Unlock()
		return ReadResult{Snapshot: g.snap, StartedAt: g.startedAt}, nil
	}
	f, joined := o.inflight[k]
	if !joined {
		f = &flight{done: make(chan struct{})}
		o.inflight[k] = f
		go o.run(workspaceID, key, k, f)
	}
	o.mu.Unlock()
	select {
	case <-f.done:
	case <-ctx.Done():
		return ReadResult{}, ctx.Err()
	}
	if f.err != nil {
		return o.failure(k), f.err
	}
	return ReadResult{Snapshot: f.res.snap, StartedAt: f.res.startedAt}, nil
}

func (o *ChangeObserver) run(workspaceID string, key mergestate.ChangeKey, k string, f *flight) {
	started := o.now()
	ctx, cancel := context.WithTimeout(context.Background(), providerReadTimeout)
	defer cancel()
	snap, err := o.reader.ReadChange(ctx, workspaceID, key)
	o.mu.Lock()
	if err == nil {
		f.res = observed{snap: snap, startedAt: started}
		o.good[k] = f.res
	}
	f.err = err
	delete(o.inflight, k)
	o.mu.Unlock()
	close(f.done)
}

func (o *ChangeObserver) failure(k string) ReadResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	g, ok := o.good[k]
	if !ok {
		return ReadResult{}
	}
	snap := g.snap
	return ReadResult{LastGood: &snap, LastGoodAt: g.startedAt}
}
