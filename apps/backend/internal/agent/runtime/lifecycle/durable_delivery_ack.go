package lifecycle

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

var errDeliveryAckOwnerRetired = errors.New("delivery acknowledgment owner retired")

const (
	durableDeliveryAckDelay       = 20 * time.Millisecond
	durableDeliveryAckBatchSize   = 256
	durableDeliveryAckRequestTime = 3 * time.Second
)

// durableDeliveryAckWorker coalesces projected cursors for one transport
// stream. Projection has already committed before schedule is called, so an
// ACK failure only delays the transport watermark and never hides output.
type durableDeliveryAckWorker struct {
	ctx       context.Context
	cancel    context.CancelFunc
	send      func(context.Context, uint64) error
	wake      chan struct{}
	execution *AgentExecution
	done      chan struct{}
	failures  uint
	onFailure func(error, time.Duration)

	mu               sync.Mutex
	pending          uint64
	acknowledged     uint64
	pendingEvents    int
	flushImmediately bool
	pendingSince     time.Time
	lastSuccess      time.Time
}

func newDurableDeliveryAckWorker(send func(context.Context, uint64) error) *durableDeliveryAckWorker {
	ctx, cancel := context.WithCancel(context.Background())
	return &durableDeliveryAckWorker{
		ctx:    ctx,
		cancel: cancel,
		send:   send,
		wake:   make(chan struct{}, 1),
	}
}

func (w *durableDeliveryAckWorker) schedule(sequence uint64, terminal bool) {
	w.mu.Lock()
	if sequence > w.pending {
		if w.pending <= w.acknowledged {
			w.pendingSince = time.Now()
		}
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
	w.lastSuccess = time.Now()
	if w.pending <= w.acknowledged {
		w.pendingSince = time.Time{}
	}
	w.flushImmediately = false
}

func (w *durableDeliveryAckWorker) run() {
	for {
		if w.ctx.Err() != nil {
			return
		}
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
	for {
		select {
		case <-timer.C:
			return true
		case <-w.wake:
			if _, immediate := w.snapshot(); immediate {
				return true
			}
		case <-w.ctx.Done():
			return false
		}
	}
}

func (w *durableDeliveryAckWorker) sendPending(sequence uint64) bool {
	requestCtx, cancel := context.WithTimeout(w.ctx, durableDeliveryAckRequestTime)
	err := w.send(requestCtx, sequence)
	cancel()
	if err == nil {
		if w.ctx.Err() != nil {
			return false
		}
		w.failures = 0
		w.clearFlush(sequence)
		return true
	}
	// Keep pending at the highest projected sequence. The next bounded retry is
	// independent of event notification and prompt dispatch.
	if w.ctx.Err() != nil || errors.Is(err, errDeliveryAckOwnerRetired) {
		return false
	}
	if errors.Is(err, journal.ErrOwnerMismatch) {
		if w.onFailure != nil {
			w.onFailure(err, 0)
		}
		w.cancel()
		return false
	}
	w.failures++
	delay := 100 * time.Millisecond << min(w.failures-1, 5)
	delay = min(5*time.Second, delay+time.Duration(rand.Int64N(int64(delay/4)+1)))
	if w.onFailure != nil {
		w.onFailure(err, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-w.ctx.Done():
		return false
	}
}

func (sm *StreamManager) scheduleDurableDeliveryAck(execution *AgentExecution, client *agentctl.Client, event agentctl.AgentEvent) {
	if !validDeliveryAckTarget(execution, client, event) {
		return
	}
	if sm.isExecutionCurrent != nil && !sm.isExecutionCurrent(execution) {
		return
	}
	execution.withAgentCtlClient(client, func() {
		sm.ackMu.Lock()
		defer sm.ackMu.Unlock()
		if sm.ackStopped {
			return
		}
		streamID := event.DeliveryStreamID
		worker := sm.ackWorkers[streamID]
		if worker == nil || worker.execution != execution || worker.ctx.Err() != nil {
			var previous <-chan struct{}
			if worker != nil {
				worker.cancel()
				previous = worker.done
			}
			worker = newDurableDeliveryAckWorker(sm.deliveryAckSender(execution, streamID))
			worker.execution, worker.done = execution, make(chan struct{})
			worker.onFailure = func(err error, delay time.Duration) {
				worker.mu.Lock()
				pendingSince, lastSuccess := worker.pendingSince, worker.lastSuccess
				worker.mu.Unlock()
				sm.logger.Warn("durable delivery acknowledgment deferred", zap.String("stream_id", streamID),
					zap.String("execution_id", execution.ID), zap.String("reason", deliveryAckFailureReason(err)), zap.Duration("retry_after", delay), zap.Time("pending_since", pendingSince), zap.Time("last_success", lastSuccess))
			}
			sm.ackWorkers[streamID] = worker
			sm.ackWG.Add(1)
			go func(w *durableDeliveryAckWorker) {
				defer sm.ackWG.Done()
				defer close(w.done)
				if previous != nil {
					select {
					case <-previous:
					case <-w.ctx.Done():
						return
					}
				}
				w.run()
			}(worker)
		}
		terminal := event.Type == streams.EventTypeComplete || event.Type == streams.EventTypeError
		worker.schedule(event.DeliverySequence, terminal)
	})
}

// Each request pins the execution's current authenticated connection. Pending
// cursors survive connection replacement, including streams with no new output.
func (sm *StreamManager) deliveryAckSender(execution *AgentExecution, streamID string) func(context.Context, uint64) error {
	return func(ctx context.Context, sequence uint64) error {
		if sm.isExecutionCurrent != nil && !sm.isExecutionCurrent(execution) {
			return errDeliveryAckOwnerRetired
		}
		client, release := execution.AcquireAgentCtlClient()
		if client == nil {
			return errors.New("delivery acknowledgment connection unavailable")
		}
		defer release()
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := client.GetDeliveryStatus(ctx, streamID)
		if err != nil {
			return err
		}
		if !deliveryAckDescriptorMatches(status, execution, streamID) {
			return journal.ErrOwnerMismatch
		}
		if err := validateRecoveredReplayStream(*status.Stream, execution, streamID, sequence); err != nil {
			return journal.ErrOwnerMismatch
		}
		if status.Stream.Acknowledged >= sequence {
			return nil
		}
		if sm.isExecutionCurrent != nil && !sm.isExecutionCurrent(execution) {
			return context.Canceled
		}
		return client.AcknowledgeDelivery(ctx, streamID, sequence)
	}
}

func deliveryAckFailureReason(err error) string {
	var response *agentctl.DeliveryHTTPError
	switch {
	case errors.Is(err, journal.ErrOwnerMismatch):
		return "owner_mismatch"
	case errors.Is(err, context.Canceled):
		return "owner_cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return cursorInventoryTimeoutReason
	case errors.As(err, &response):
		return "http_error"
	default:
		return "transport"
	}
}

func validDeliveryAckTarget(execution *AgentExecution, client *agentctl.Client, event agentctl.AgentEvent) bool {
	return execution != nil && client != nil && event.DeliveryStreamID != "" && event.DeliverySequence > 0
}

func deliveryAckDescriptorMatches(status *agentctl.DeliveryStatus, execution *AgentExecution, streamID string) bool {
	return status != nil && status.Durable && status.Version == journal.CurrentVersion && status.Stream != nil && status.SessionID == execution.SessionID && status.IncarnationID == execution.DeliveryIncarnationID && status.HarnessGeneration == execution.DeliveryHarnessGeneration && status.StreamID == streamID
}
