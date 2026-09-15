package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

const (
	streamProjectionBatchBytes = 64 << 10
	streamProjectionBatchWait  = 20 * time.Millisecond
	streamProjectionQueueSize  = 4096
)

var errStreamEventProcessorQueueFull = fmt.Errorf(
	"%w: durable agent event processor queue is full",
	ErrUncertainPromptDelivery,
)

// streamEventProcessor keeps the transport reader responsive while preserving
// ordered lifecycle delivery. Compatible canonical chunks can accumulate until
// the byte or timer bound and commit through one repository transaction.
type streamEventProcessor struct {
	manager           *StreamManager
	ctx               context.Context
	cancel            context.CancelFunc
	execution         *AgentExecution
	client            *agentctl.Client
	delivery          AgentDeliveryRepository
	startupGeneration uint64
	events            chan agentctl.AgentEvent
	done              chan struct{}

	closeOnce sync.Once
	errMu     sync.Mutex
	err       error
}

type streamEventBatch struct {
	items   []preparedAgentEvent
	bytes   int
	timer   *time.Timer
	timerCh <-chan time.Time
}

func newStreamEventProcessor(
	manager *StreamManager,
	parent context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	delivery AgentDeliveryRepository,
	startupGeneration uint64,
) *streamEventProcessor {
	ctx, cancel := context.WithCancel(parent)
	processor := &streamEventProcessor{
		manager:           manager,
		ctx:               ctx,
		cancel:            cancel,
		execution:         execution,
		client:            client,
		delivery:          delivery,
		startupGeneration: startupGeneration,
		events:            make(chan agentctl.AgentEvent, streamProjectionQueueSize),
		done:              make(chan struct{}),
	}
	go processor.run()
	return processor
}

func (p *streamEventProcessor) enqueue(event agentctl.AgentEvent) bool {
	select {
	case p.events <- event:
		return true
	case <-p.ctx.Done():
		return false
	}
}

func (p *streamEventProcessor) close() error {
	p.closeOnce.Do(func() {
		close(p.events)
	})
	<-p.done
	p.cancel()
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}

func (p *streamEventProcessor) fail(err error) {
	if err == nil {
		return
	}
	p.errMu.Lock()
	if p.err == nil {
		p.err = err
	}
	p.errMu.Unlock()
	p.cancel()
}

func (p *streamEventProcessor) run() {
	defer close(p.done)
	defer p.cancel()

	batch := &streamEventBatch{}
	for {
		select {
		case event, ok := <-p.events:
			if !ok {
				p.fail(p.flushBatch(batch))
				return
			}
			if err := p.handleEvent(batch, event); err != nil {
				p.fail(err)
				return
			}
		case <-batch.timerCh:
			if err := p.flushBatch(batch); err != nil {
				p.fail(err)
				return
			}
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *streamEventProcessor) handleEvent(batch *streamEventBatch, event agentctl.AgentEvent) error {
	prepared, err := p.manager.prepareAgentEvent(p.ctx, p.execution, p.client, p.delivery, event)
	if err != nil {
		return err
	}
	if prepared.duplicate {
		p.manager.scheduleDurableDeliveryAck(p.client, prepared.event)
		return nil
	}
	if !p.batchable(prepared) {
		if err := p.flushBatch(batch); err != nil {
			return err
		}
		return p.manager.processPreparedAgentEvent(
			p.ctx, p.execution, p.client, p.delivery, prepared, p.startupGeneration,
		)
	}
	if p.mustFlushBefore(batch, prepared) {
		if err := p.flushBatch(batch); err != nil {
			return err
		}
	}
	batch.items = append(batch.items, prepared)
	batch.bytes += len(prepared.durableEvent.Payload)
	if len(batch.items) == 1 {
		batch.timer = time.NewTimer(streamProjectionBatchWait)
		batch.timerCh = batch.timer.C
	}
	if batch.bytes >= streamProjectionBatchBytes {
		return p.flushBatch(batch)
	}
	return nil
}

func (p *streamEventProcessor) mustFlushBefore(batch *streamEventBatch, prepared preparedAgentEvent) bool {
	if len(batch.items) == 0 {
		return false
	}
	previous := batch.items[len(batch.items)-1]
	return !compatibleCanonicalBatch(previous, prepared, p.execution) ||
		batch.bytes+len(prepared.durableEvent.Payload) > streamProjectionBatchBytes
}

func (p *streamEventProcessor) flushBatch(batch *streamEventBatch) error {
	if len(batch.items) == 0 {
		return nil
	}
	err := p.manager.projectCanonicalAgentEvents(
		p.ctx, p.execution, batch.items, p.delivery, p.client, p.startupGeneration,
	)
	batch.items = nil
	batch.bytes = 0
	if batch.timer != nil && !batch.timer.Stop() {
		select {
		case <-batch.timer.C:
		default:
		}
	}
	batch.timer = nil
	batch.timerCh = nil
	return err
}

func (p *streamEventProcessor) batchable(prepared preparedAgentEvent) bool {
	if _, ok := p.delivery.(canonicalAgentDeliveryBatchProjector); !ok {
		return false
	}
	return prepared.durableEvent != nil && canonicalAgentDeliveryEvent(prepared.event)
}

func compatibleCanonicalBatch(previous, next preparedAgentEvent, execution *AgentExecution) bool {
	if previous.durableEvent == nil || next.durableEvent == nil {
		return false
	}
	if previous.event.Type != next.event.Type {
		return false
	}
	return canonicalAgentMessageID(execution, previous.event) == canonicalAgentMessageID(execution, next.event)
}

func (sm *StreamManager) processPreparedAgentEvent(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
	delivery AgentDeliveryRepository,
	prepared preparedAgentEvent,
	startupGeneration uint64,
) error {
	if prepared.duplicate {
		sm.scheduleDurableDeliveryAck(client, prepared.event)
		return nil
	}
	event, canonicalProjected, err := sm.projectCanonicalAgentEvent(
		ctx, execution, prepared.event, prepared.durableEvent, delivery, client,
	)
	if err != nil {
		return fmt.Errorf("canonical agent event projection: %w", err)
	}
	if !prepared.skipCallback {
		sm.notifyAgentEvent(execution, event, startupGeneration)
	}
	if prepared.durableEvent == nil || canonicalProjected {
		return nil
	}
	if err := sm.projectDurableAgentDeliveryEventWithEffect(
		ctx, prepared.durableEvent, delivery, prepared.deliveryEffect,
	); err != nil {
		return fmt.Errorf("project durable agent event: %w", err)
	}
	sm.scheduleDurableDeliveryAck(client, event)
	return nil
}
