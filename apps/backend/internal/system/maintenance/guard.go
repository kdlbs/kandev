// Package maintenance serializes database maintenance with retention batches.
package maintenance

import (
	"context"
	"github.com/kandev/kandev/internal/db"
	"sync"
)

// Guard admits one maintenance operation or retention batch at a time.
type Guard struct{ token chan struct{} }

var pools sync.Map

// ForPool returns the process-lifetime admission guard for a shared pool.
func ForPool(pool *db.Pool) *Guard {
	guard, _ := pools.LoadOrStore(pool, &Guard{token: make(chan struct{}, 1)})
	return guard.(*Guard)
}

// Acquire waits for admission, respecting cancellation. Release is idempotent.
func (g *Guard) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case g.token <- struct{}{}:
		release := sync.OnceFunc(func() { <-g.token })
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TryAcquire lets a batch defer immediately while maintenance is running.
func (g *Guard) TryAcquire() (func(), bool) {
	select {
	case g.token <- struct{}{}:
		return sync.OnceFunc(func() { <-g.token }), true
	default:
		return nil, false
	}
}
