package service

import (
	"context"
	"errors"
	"sync"
)

var ErrFormatterCapture = errors.New("notification configuration capture in progress")

type formatterConfigurationGate struct {
	mu      sync.Mutex
	active  int
	frozen  bool
	drained chan struct{}
}

// BeginFormatterManagement fences configuration writes against a coherent
// capture. Notification rendering and ordinary task activity do not use it.
func (s *Service) BeginFormatterManagement(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g := &s.formatterConfig
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.frozen {
		return nil, ErrFormatterCapture
	}
	g.active++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.active--
			if g.frozen && g.active == 0 {
				close(g.drained)
			}
		})
	}, nil
}

// CaptureFormatterConfiguration drains admitted mutations and rejects new ones
// until release. Cancellation always reopens the gate without a waiter goroutine.
func (s *Service) CaptureFormatterConfiguration(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g := &s.formatterConfig
	g.mu.Lock()
	if g.frozen {
		g.mu.Unlock()
		return nil, ErrFormatterCapture
	}
	g.frozen = true
	drained := make(chan struct{})
	g.drained = drained
	if g.active == 0 {
		close(drained)
	}
	g.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.drained == drained {
				g.frozen = false
			}
		})
	}
	select {
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	case <-drained:
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		if runtime := s.runtimeFormatter(); runtime != nil {
			capturer, ok := runtime.(interface {
				CaptureFormatterLifecycle(context.Context) (func(), error)
			})
			if !ok {
				release()
				return nil, ErrFormatterCapture
			}
			finish, err := capturer.CaptureFormatterLifecycle(ctx)
			if err != nil {
				release()
				return nil, err
			}
			var lifecycleOnce sync.Once
			return func() { lifecycleOnce.Do(func() { finish(); release() }) }, nil
		}
		return release, nil
	}
}
