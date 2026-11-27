package process

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
)

const (
	deliveryWriterBatchBytes = 64 << 10
	deliveryWriterBatchWait  = 20 * time.Millisecond
	deliveryWriterQueueBytes = int(deliverylimits.WriterQueueBytes)
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
	ctx      context.Context
	cancel   context.CancelFunc
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
	parent := manager.lifetimeCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	writer := &deliveryEventWriter{
		ctx: ctx, cancel: cancel,
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
	if err := ctx.Err(); err != nil {
		return update, err
	}
	size, err := deliveryWriteSize(update)
	if err != nil {
		return update, err
	}
	if size > deliveryWriterQueueBytes {
		return update, journal.NewStorageError(journal.StorageReasonEventTooLarge, journal.ErrEventTooLarge,
			journal.WithEventBytes(int64(size)))
	}
	request := deliveryWriteRequest{update: update, size: size, result: make(chan deliveryWriteResult, 1)}
	if err := w.reserve(ctx, size); err != nil {
		return update, err
	}

	// Serialize the send with close. Without this lock, close can close input
	// after the admission check and before the send, which panics in a
	// concurrent teardown.
	w.sendMu.Lock()
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		w.sendMu.Unlock()
		w.release(size)
		return update, errDeliveryWriterClosed
	}
	w.mu.Unlock()
	select {
	case w.input <- request:
		w.sendMu.Unlock()
	case <-w.ctx.Done():
		w.sendMu.Unlock()
		w.release(size)
		return update, errDeliveryWriterClosed
	case <-ctx.Done():
		w.sendMu.Unlock()
		w.release(size)
		return update, ctx.Err()
	}
	// Once the request is in the single ordered writer, the durable write owns
	// its native-session fence. Caller cancellation cannot release that fence
	// while the writer may still commit; wait for the actual commit result.
	result := <-request.result
	return result.update, result.err
}

func (w *deliveryEventWriter) reserve(ctx context.Context, size int) error {
	for {
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return errDeliveryWriterClosed
		}
		if w.queued+size <= deliveryWriterQueueBytes {
			w.queued += size
			w.mu.Unlock()
			return nil
		}
		space := w.space
		w.mu.Unlock()
		select {
		case <-space:
		case <-w.ctx.Done():
			return errDeliveryWriterClosed
		case <-w.closedCh:
			return errDeliveryWriterClosed
		case <-ctx.Done():
			return ctx.Err()
		}
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
		w.cancel()
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
		if len(w.input) > 0 && batchBytes < deliveryWriterBatchBytes && !isTerminalDeliveryEvent(first.update) {
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
	committed, err := w.manager.persistDeliveryBatch(w.ctx, updates)
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

func deliveryWriteSize(update adapter.AgentEvent) (int, error) {
	textBytes := 0
	if update.ShellOutputEligible && update.NormalizedPayload != nil && update.NormalizedPayload.ShellExec() != nil {
		update.NormalizedPayload = update.NormalizedPayload.Snapshot()
		output := update.NormalizedPayload.ShellExec().Output
		if output != nil {
			textBytes = 4*((len(output.Stdout)+2)/3) + 4*((len(output.Stderr)+2)/3)
		}
		update.NormalizedPayload.ShellExec().Output = nil
		update.ToolCallContents = nil
	}
	metadata, err := json.Marshal(update)
	return len(metadata) + textBytes + 1024, err
}
