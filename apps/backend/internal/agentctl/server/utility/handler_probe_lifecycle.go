package utility

import (
	"context"
	"errors"
	"sync"
	"time"
)

type probeLifecycle struct {
	mu      sync.Mutex
	closed  bool
	nextID  uint64
	active  map[uint64]context.CancelFunc
	drained chan struct{}
}

func (p *probeLifecycle) begin(parent context.Context) (context.Context, func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, errors.New("utility probes are stopping")
	}
	if len(p.active) == 0 {
		p.active = make(map[uint64]context.CancelFunc)
		p.drained = make(chan struct{})
	}
	ctx, cancel := context.WithCancel(parent)
	p.nextID++
	id := p.nextID
	p.active[id] = cancel
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			cancel()
			p.mu.Lock()
			delete(p.active, id)
			if len(p.active) == 0 {
				close(p.drained)
			}
			p.mu.Unlock()
		})
	}, nil
}

func (p *probeLifecycle) close() error {
	p.mu.Lock()
	p.closed = true
	for _, cancel := range p.active {
		cancel()
	}
	drained, active := p.drained, len(p.active)
	p.mu.Unlock()
	if active == 0 {
		return nil
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-drained:
		return nil
	case <-timer.C:
		return errors.New("utility probe cleanup is still in progress")
	}
}
