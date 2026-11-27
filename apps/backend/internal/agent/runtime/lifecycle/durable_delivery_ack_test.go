package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestDurableDeliveryAckWorkerRetriesHighestCursor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var calls []uint64
	var inFlight int
	var maxInFlight int
	worker := &durableDeliveryAckWorker{
		ctx:  ctx,
		wake: make(chan struct{}, 1),
		send: func(_ context.Context, sequence uint64) error {
			mu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			calls = append(calls, sequence)
			attempt := len(calls)
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
			if attempt == 1 {
				return errors.New("temporary ACK failure")
			}
			return nil
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.run()
	}()
	for sequence := uint64(1); sequence <= durableDeliveryAckBatchSize; sequence++ {
		worker.schedule(sequence, false)
	}

	deadline := time.After(time.Second)
	for {
		worker.mu.Lock()
		acknowledged := worker.acknowledged
		worker.mu.Unlock()
		if acknowledged == durableDeliveryAckBatchSize {
			break
		}
		select {
		case <-deadline:
			t.Fatal("ACK worker did not retry the highest cursor")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if maxInFlight != 1 {
		t.Fatalf("max in-flight ACK requests = %d, want 1", maxInFlight)
	}
	if len(calls) < 2 || calls[len(calls)-1] != durableDeliveryAckBatchSize {
		t.Fatalf("ACK calls = %v, want a retry ending at %d", calls, durableDeliveryAckBatchSize)
	}
}
