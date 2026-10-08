package storage

import (
	"context"
	"sync"
)

// MutationGate serializes operations that can change one selected Go cache.
type MutationGate struct {
	token chan struct{}
}

func NewMutationGate() *MutationGate {
	return &MutationGate{token: make(chan struct{}, 1)}
}

func (g *MutationGate) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if g == nil {
		return func() {}, nil
	}
	select {
	case g.token <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-g.token
			return nil, err
		}
		var once sync.Once
		return func() {
			once.Do(func() { <-g.token })
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
