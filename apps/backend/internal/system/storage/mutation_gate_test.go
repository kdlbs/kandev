package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMutationGateSerializesAndHonorsCancellation(t *testing.T) {
	gate := NewMutationGate()
	releaseFirst, err := gate.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	base, cancel := context.WithCancel(context.Background())
	ctx := &signallingDoneContext{Context: base, entered: make(chan struct{})}
	acquired := make(chan error, 1)
	go func() {
		release, acquireErr := gate.Acquire(ctx)
		if release != nil {
			release()
		}
		acquired <- acquireErr
	}()
	select {
	case <-ctx.entered:
	case <-time.After(time.Second):
		t.Fatal("waiter did not enter the blocking acquire")
	}
	cancel()
	select {
	case err := <-acquired:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("second Acquire error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled mutation wait did not finish")
	}

	releaseFirst()
	release, err := gate.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	release()
}

type signallingDoneContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *signallingDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}
