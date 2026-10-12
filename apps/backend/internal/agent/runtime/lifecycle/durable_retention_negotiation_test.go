package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type observedNegotiationContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedNegotiationContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestDurableRetentionNegotiationBuffersRetainedFrame(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ctx := &observedNegotiationContext{Context: parent, waiting: make(chan struct{})}
	execution, event := retentionBootstrapOwner()
	execution.DeliveryMode = ""
	gate := execution.beginDeliveryNegotiation()
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	repo.generation.Store(retentionBootstrapGeneration())
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	result := make(chan error, 1)
	go func() {
		_, err := manager.prepareAgentEvent(ctx, execution, nil, repo, event)
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("retained frame processed before negotiation: %v", err)
	case <-ctx.waiting:
	case <-parent.Done():
		t.Fatal("retained frame did not wait for negotiation")
	}
	if repo.receipts.Load() != 0 {
		t.Fatal("receipt before negotiation")
	}
	execution.DeliveryMode = DurableDeliveryV1
	gate.finish(nil)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if repo.receipts.Load() != 1 {
		t.Fatal("negotiated retained frame was not admitted")
	}
}

func TestDurableRetentionNegotiationCancellationFailsClosed(t *testing.T) {
	execution, event := retentionBootstrapOwner()
	execution.DeliveryMode = ""
	execution.beginDeliveryNegotiation()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	_, err := manager.prepareAgentEvent(ctx, execution, nil, repo, event)
	if !errors.Is(err, context.Canceled) || repo.receipts.Load() != 0 {
		t.Fatalf("canceled negotiation admitted evidence: %v", err)
	}
}
