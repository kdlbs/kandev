package lifecycle

import (
	"context"
	"sync"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	durableDeliveryAckDelay       = 20 * time.Millisecond
	durableDeliveryAckBatchSize   = 256
	durableDeliveryAckRequestTime = 3 * time.Second
)

// durableDeliveryAckWorker coalesces projected cursors for one transport
// stream. Projection has already committed before schedule is called, so an
// ACK failure only delays the transport watermark and never hides output.
type durableDeliveryAckWorker struct {
	ctx    context.Context
	cancel context.CancelFunc
	send   func(context.Context, uint64) error
	wake   chan struct{}

	mu               sync.Mutex
	pending          uint64
	acknowledged     uint64
	pendingEvents    int
	flushImmediately bool
}

func newDurableDeliveryAckWorker(client *agentctl.Client, streamID string) *durableDeliveryAckWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &durableDeliveryAckWorker{
		ctx:    ctx,
		cancel: cancel,
		send: func(ctx context.Context, sequence uint64) error {
			return client.AcknowledgeDelivery(ctx, streamID, sequence)
		},
		wake: make(chan struct{}, 1),
	}
}

func (w *durableDeliveryAckWorker) schedule(sequence uint64, terminal bool) {
	w.mu.Lock()
	if sequence > w.pending {
		w.pending = sequence
	}
	w.pendingEvents++
	if terminal || w.pendingEvents >= durableDeliveryAckBatchSize {
		w.flushImmediately = true
	}
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *durableDeliveryAckWorker) snapshot() (uint64, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending <= w.acknowledged {
		return 0, false
	}
	return w.pending, w.flushImmediately
}

func (w *durableDeliveryAckWorker) clearFlush(sequence uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if sequence > w.acknowledged {
		w.acknowledged = sequence
	}
	w.pendingEvents = 0
	w.flushImmediately = false
}

func (w *durableDeliveryAckWorker) run() {
	for {
		sequence, immediate := w.snapshot()
		if sequence == 0 {
			if !w.waitForWake() {
				return
			}
			continue
		}
		if !immediate && !w.waitForDelay() {
			return
		}
		// A timer may have expired while more projected events arrived. Take
		// the highest cursor immediately before the one in-flight request.
		sequence, _ = w.snapshot()
		if sequence == 0 {
			continue
		}
		if !w.sendPending(sequence) {
			return
		}
	}
}

func (w *durableDeliveryAckWorker) waitForWake() bool {
	select {
	case <-w.wake:
		return true
	case <-w.ctx.Done():
		return false
	}
}

func (w *durableDeliveryAckWorker) waitForDelay() bool {
	timer := time.NewTimer(durableDeliveryAckDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-w.wake:
		return true
	case <-w.ctx.Done():
		return false
	}
}

func (w *durableDeliveryAckWorker) sendPending(sequence uint64) bool {
	requestCtx, cancel := context.WithTimeout(w.ctx, durableDeliveryAckRequestTime)
	err := w.send(requestCtx, sequence)
	cancel()
	if err == nil {
		w.clearFlush(sequence)
		return true
	}
	// Keep pending at the highest projected sequence. The next bounded retry is
	// independent of event notification and prompt dispatch.
	return w.waitForDelay()
}

func (sm *StreamManager) scheduleDurableDeliveryAck(client *agentctl.Client, event agentctl.AgentEvent) {
	if client == nil || event.DeliveryStreamID == "" || event.DeliverySequence == 0 {
		return
	}
	streamID := event.DeliveryStreamID
	sm.ackMu.Lock()
	worker := sm.ackWorkers[streamID]
	if worker == nil {
		worker = newDurableDeliveryAckWorker(client, streamID)
		sm.ackWorkers[streamID] = worker
		sm.ackWG.Add(1)
		go func() {
			defer sm.ackWG.Done()
			worker.run()
		}()
	}
	sm.ackMu.Unlock()
	terminal := event.Type == streams.EventTypeComplete || event.Type == streams.EventTypeError
	worker.schedule(event.DeliverySequence, terminal)
}
