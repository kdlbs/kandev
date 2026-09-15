package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

const (
	deliveryWriterBatchBytes = 64 << 10
	deliveryWriterBatchWait  = 20 * time.Millisecond
	deliveryWriterQueueBytes = 4 << 20
)

var errDeliveryWriterClosed = errors.New("durable delivery writer is closed")

type deliveryWriteRequest struct {
	update adapter.AgentEvent
	size   int
	result chan deliveryWriteResult
}

type deliveryWriteResult struct {
	update adapter.AgentEvent
	err    error
}

// deliveryEventWriter is the single ordered journal writer for normalized
// agent events. Its byte budget covers queued requests, while the commit batch
// stays bounded independently.
type deliveryEventWriter struct {
	manager  *Manager
	input    chan deliveryWriteRequest
	space    chan struct{}
	closedCh chan struct{}
	done     chan struct{}

	mu        sync.Mutex
	sendMu    sync.Mutex
	queued    int
	closed    bool
	closeOnce sync.Once
}

func newDeliveryEventWriter(manager *Manager) *deliveryEventWriter {
	writer := &deliveryEventWriter{
		manager:  manager,
		input:    make(chan deliveryWriteRequest, 256),
		space:    make(chan struct{}, 1),
		closedCh: make(chan struct{}),
		done:     make(chan struct{}),
	}
	go writer.run()
	return writer
}

func (w *deliveryEventWriter) persist(ctx context.Context, update adapter.AgentEvent) (adapter.AgentEvent, error) {
	payload, err := json.Marshal(update)
	if err != nil {
		return update, err
	}
	size := len(payload)
	if size > deliveryWriterQueueBytes {
		return update, fmt.Errorf("%w: delivery queue item is %d bytes", journal.ErrJournalFull, size)
	}
	request := deliveryWriteRequest{update: update, size: size, result: make(chan deliveryWriteResult, 1)}
	for {
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return update, errDeliveryWriterClosed
		}
		if w.queued+size <= deliveryWriterQueueBytes {
			w.queued += size
			w.mu.Unlock()
			break
		}
		space := w.space
		w.mu.Unlock()
		select {
		case <-space:
		case <-w.closedCh:
			return update, errDeliveryWriterClosed
		case <-ctx.Done():
			return update, ctx.Err()
		}
	}

	// Serialize the send with close. Without this lock, close can close input
	// after the admission check and before the send, which panics in a
	// concurrent teardown.
	w.sendMu.Lock()
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		w.sendMu.Unlock()
		return update, errDeliveryWriterClosed
	}
	w.mu.Unlock()
	select {
	case w.input <- request:
		w.sendMu.Unlock()
	case <-ctx.Done():
		w.sendMu.Unlock()
		w.release(size)
		return update, ctx.Err()
	}
	select {
	case result := <-request.result:
		return result.update, result.err
	case <-ctx.Done():
		return update, ctx.Err()
	}
}

func (w *deliveryEventWriter) release(size int) {
	w.mu.Lock()
	w.queued -= size
	if w.queued < 0 {
		w.queued = 0
	}
	w.mu.Unlock()
	select {
	case w.space <- struct{}{}:
	default:
	}
}

func (w *deliveryEventWriter) close() {
	w.closeOnce.Do(func() {
		w.sendMu.Lock()
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()
		close(w.closedCh)
		close(w.input)
		w.sendMu.Unlock()
	})
	<-w.done
}

func (w *deliveryEventWriter) run() {
	defer close(w.done)
	for first := range w.input {
		batch := []deliveryWriteRequest{first}
		batchBytes := first.size
		if batchBytes < deliveryWriterBatchBytes && !isTerminalDeliveryEvent(first.update) {
			timer := time.NewTimer(deliveryWriterBatchWait)
			batch = collectDeliveryBatch(w.input, batch, batchBytes, timer.C)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		w.commit(batch)
	}
}

func collectDeliveryBatch(
	input <-chan deliveryWriteRequest,
	batch []deliveryWriteRequest,
	batchBytes int,
	timer <-chan time.Time,
) []deliveryWriteRequest {
	for batchBytes < deliveryWriterBatchBytes {
		select {
		case next, ok := <-input:
			if !ok {
				return batch
			}
			batch = append(batch, next)
			batchBytes += next.size
			if isTerminalDeliveryEvent(next.update) {
				return batch
			}
		case <-timer:
			return batch
		}
	}
	return batch
}

func (w *deliveryEventWriter) commit(batch []deliveryWriteRequest) {
	updates := make([]adapter.AgentEvent, len(batch))
	for i, request := range batch {
		updates[i] = request.update
	}
	committed, err := w.manager.persistDeliveryBatch(context.Background(), updates)
	for i, request := range batch {
		result := deliveryWriteResult{update: request.update, err: err}
		if err == nil {
			result.update = committed[i]
		}
		request.result <- result
		w.release(request.size)
	}
}

func isTerminalDeliveryEvent(event adapter.AgentEvent) bool {
	return event.Type == adapter.EventTypeComplete || event.Type == adapter.EventTypeError
}
