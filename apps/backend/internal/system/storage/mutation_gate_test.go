package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMutationGateSerializesAndHonorsCancellation(t *testing.T) {
	gate := NewMutationGate()
	releaseFirst, err := gate.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	acquired := make(chan error, 1)
	go func() {
		release, acquireErr := gate.Acquire(ctx)
		if release != nil {
			release()
		}
		acquired <- acquireErr
	}()
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
