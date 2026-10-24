package process

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
)

const passthroughEventPageLimit = 256

func (o *PassthroughOwner) emitLocked(kind string, exitCode *int) {
	o.current.Sequence++
	o.events = append(o.events, types.PassthroughEvent{
		PassthroughTarget: o.current.PassthroughTarget, InstanceID: o.manager.cfg.InstanceID,
		SessionID: o.manager.cfg.SessionID, Sequence: o.current.Sequence, Kind: kind,
		Status: o.current.Status, ExitCode: exitCode, Timestamp: time.Now().UTC(),
	})
	if len(o.events) > o.limit {
		o.events = append([]types.PassthroughEvent(nil), o.events[len(o.events)-o.limit:]...)
	}
	close(o.wake)
	o.wake = make(chan struct{})
}

func (o *PassthroughOwner) onStatus(status *types.ProcessStatusUpdate) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if status.ProcessID != o.current.ProcessID {
		return
	}
	o.current.Status, o.current.ExitCode = status.Status, status.ExitCode
	o.emitLocked("status", status.ExitCode)
}

func (o *PassthroughOwner) onIdle(_, processID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if processID != o.current.ProcessID || !o.liveLocked() {
		return
	}
	o.current.FirstIdle = true
	o.current.WaitingForInput = true
	o.emitLocked("idle", nil)
}

func (o *PassthroughOwner) Events(ctx context.Context, after uint64, wait bool) (types.PassthroughEventPage, error) {
	for {
		o.mu.Lock()
		page := types.PassthroughEventPage{Sequence: o.current.Sequence}
		if len(o.events) > 0 && after < o.events[0].Sequence-1 {
			page.Gap = true
		}
		for _, event := range o.events {
			if event.Sequence > after {
				page.Events = append(page.Events, event)
				page.Sequence = event.Sequence
				if len(page.Events) == passthroughEventPageLimit {
					break
				}
			}
		}
		wake := o.wake
		o.mu.Unlock()
		if !wait || page.Gap || len(page.Events) > 0 {
			return page, nil
		}
		select {
		case <-ctx.Done():
			return page, ctx.Err()
		case <-o.manager.lifetimeCtx.Done():
			return page, context.Canceled
		case <-wake:
		}
	}
}
