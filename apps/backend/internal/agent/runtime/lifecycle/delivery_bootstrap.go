package lifecycle

import (
	"context"
	"sync"
	"time"
)

type deliveryNegotiationGate struct {
	ready chan struct{}
	once  sync.Once
	err   error
}

func (e *AgentExecution) beginDeliveryNegotiation() *deliveryNegotiationGate {
	gate := &deliveryNegotiationGate{ready: make(chan struct{})}
	e.deliveryNegotiation.Store(gate)
	return gate
}

func (g *deliveryNegotiationGate) finish(err error) {
	if g != nil {
		g.once.Do(func() { g.err = err; close(g.ready) })
	}
}

func waitForDeliveryNegotiation(ctx context.Context, execution *AgentExecution) error {
	if execution == nil {
		return nil
	}
	gate := execution.deliveryNegotiation.Load()
	if gate == nil {
		return nil
	}
	select {
	case <-gate.ready:
		return gate.err
	default:
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case <-gate.ready:
		return gate.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
