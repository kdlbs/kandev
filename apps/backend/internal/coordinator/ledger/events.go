package ledger

import (
	"context"

	"github.com/kandev/kandev/internal/events/bus"
)

const turnEventQueueSize = 256

type turnJob struct {
	handle func(context.Context, *bus.Event)
	event  *bus.Event
}

// enqueueEvent hands a turn event to the single event worker. It never blocks
// the publisher: a full queue drops the event and counts a start failure.
func (l *Ledger) enqueueEvent(j turnJob) {
	select {
	case l.events <- j:
	default:
		countFailure(StageStart)
		l.log.Warn("coordinator ledger: turn event queue full, event dropped")
	}
}

// runTurnEvents applies turn events one at a time in arrival order, so a start
// is always handled before the completion that follows it.
func (l *Ledger) runTurnEvents() {
	defer l.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-l.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	for {
		select {
		case <-l.stopCh:
			return
		case j := <-l.events:
			j.handle(ctx, j.event)
		}
	}
}
