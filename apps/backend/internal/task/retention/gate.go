package retention

import (
	"context"
	"sync"
)

type mutationLease struct {
	mu    sync.Mutex
	users int
}
type mutationContextKey struct{}

var mutationToken = make(chan struct{}, 1)

// AcquireMutation serializes retention publication with physical cleanup in
// this backend. SQL snapshots preserve the protection across process restarts.
// Nested cleanup calls share their caller's lease through the context.
func AcquireMutation(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}
	if lease, ok := ctx.Value(mutationContextKey{}).(*mutationLease); ok && lease.retain() {
		return ctx, lease.releaseFunc(), nil
	}
	select {
	case mutationToken <- struct{}{}:
		lease := &mutationLease{users: 1}
		return context.WithValue(ctx, mutationContextKey{}, lease), lease.releaseFunc(), nil
	case <-ctx.Done():
		return ctx, nil, ctx.Err()
	}
}

func (l *mutationLease) retain() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.users == 0 {
		return false
	}
	l.users++
	return true
}

func (l *mutationLease) releaseFunc() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.users--
			if l.users == 0 {
				<-mutationToken
			}
		})
	}
}
