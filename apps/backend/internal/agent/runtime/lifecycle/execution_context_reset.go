package lifecycle

import (
	"context"
	"errors"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// ErrContextResetFenced means a reset attempt may still have provider-side
// effects, so prompt and session-configuration admission is closed until an
// explicit process restart succeeds.
var ErrContextResetFenced = errors.New("agent context reset requires explicit recovery")

var errContextResetInProgress = errors.New("agent context reset is in progress")

const maxContextResetSetupEvents = 64

const contextResetLeasePoll = 10 * time.Millisecond

func waitForContextResetLease(ctx context.Context, try func() bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if try() {
			return nil
		}
		timer := time.NewTimer(contextResetLeasePoll)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (e *AgentExecution) acquireContextResetOperation(ctx context.Context) (func(), error) {
	if err := waitForContextResetLease(ctx, e.contextResetAdmissionMu.TryRLock); err != nil {
		return nil, err
	}
	if e.idleSuspensionInProgress.Load() {
		e.contextResetAdmissionMu.RUnlock()
		return nil, ErrIdleSuspensionInProgress
	}
	if err := e.contextResetAdmissionError(); err != nil {
		e.contextResetAdmissionMu.RUnlock()
		return nil, err
	}
	return e.contextResetAdmissionMu.RUnlock, nil
}

func (e *AgentExecution) acquireContextResetExclusive(ctx context.Context) (func(), error) {
	if err := waitForContextResetLease(ctx, e.contextResetAdmissionMu.TryLock); err != nil {
		return nil, err
	}
	return e.contextResetAdmissionMu.Unlock, nil
}

func (e *AgentExecution) beginContextReset() error {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	if e.contextResetFenced {
		return ErrContextResetFenced
	}
	if e.contextResetInFlight {
		return errContextResetInProgress
	}
	e.contextResetInFlight = true
	e.contextResetEvents = nil
	e.contextResetSetupEventCount = 0
	return nil
}

// drainContextResetEvents returns buffered events accepted for the session that
// the caller just committed. Events from the old session can arrive while
// session/new is waiting and must not replay into the replacement conversation.
func (e *AgentExecution) drainContextResetEvents(newSessionID string) []agentctl.AgentEvent {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	buffered := e.contextResetEvents
	e.contextResetEvents = nil
	e.contextResetSetupEventCount = 0
	if len(buffered) == 0 {
		return nil
	}
	accepted := make([]agentctl.AgentEvent, 0, len(buffered))
	for _, event := range buffered {
		if event.SessionID == "" || event.SessionID == newSessionID {
			accepted = append(accepted, event)
		}
	}
	return accepted
}

// finishContextReset closes the reset boundary and returns accepted events that
// arrived after the last explicit drain. The transition and snapshot are one
// critical section: an event handler that observed the in-flight boundary is
// either included in the returned batch or, after the transition, proceeds
// through ordinary lifecycle handling. No buffered event is discarded at the
// boundary.
func (e *AgentExecution) finishContextReset(newSessionID string) []agentctl.AgentEvent {
	e.contextResetMu.Lock()
	buffered := e.contextResetEvents
	e.contextResetEvents = nil
	e.contextResetSetupEventCount = 0
	e.contextResetInFlight = false
	e.contextResetMu.Unlock()

	accepted := make([]agentctl.AgentEvent, 0, len(buffered))
	for _, event := range buffered {
		if event.SessionID == "" || event.SessionID == newSessionID {
			accepted = append(accepted, event)
		}
	}
	return accepted
}

func (e *AgentExecution) failContextReset(fence bool, reason string) {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	e.contextResetInFlight = false
	e.contextResetRecoveryInFlight = false
	e.contextResetEvents = nil
	e.contextResetSetupEventCount = 0
	if fence {
		e.contextResetFenced = true
		e.contextResetFenceReason = reason
	}
}

func (e *AgentExecution) beginContextResetRecovery() {
	e.contextResetMu.Lock()
	e.contextResetRecoveryInFlight = true
	e.contextResetMu.Unlock()
}

func (e *AgentExecution) finishContextResetRecovery(success bool, reason string) {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	e.contextResetRecoveryInFlight = false
	if success {
		e.contextResetFenced = false
		e.contextResetFenceReason = ""
		return
	}
	if e.contextResetFenced {
		e.contextResetFenceReason = reason
	}
}

func (e *AgentExecution) contextResetAdmissionError() error {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	if e.contextResetFenced {
		return ErrContextResetFenced
	}
	if e.contextResetInFlight {
		return errContextResetInProgress
	}
	return nil
}

func (e *AgentExecution) contextResetFencedState() bool {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	return e.contextResetFenced
}

// bufferOrDropContextResetEvent returns true when the event was consumed by
// the reset boundary and must not enter ordinary lifecycle handling.
func (e *AgentExecution) bufferOrDropContextResetEvent(event agentctl.AgentEvent) bool {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	if e.contextResetFenced {
		if e.contextResetRecoveryInFlight && isContextResetSetupEvent(event) {
			return false
		}
		return true
	}
	if !e.contextResetInFlight {
		return false
	}
	if isContextResetRetiredWorkProjection(event) {
		e.contextResetEvents = append(e.contextResetEvents, event)
	} else if isContextResetSetupEvent(event) && e.contextResetSetupEventCount < maxContextResetSetupEvents {
		e.contextResetEvents = append(e.contextResetEvents, event)
		e.contextResetSetupEventCount++
	}
	return true
}

func isContextResetRetiredWorkProjection(event agentctl.AgentEvent) bool {
	work := event.BackgroundWork
	return event.Type == streams.EventTypeBackgroundWorkUpdated && event.SessionID != "" &&
		work != nil && (work.Kind == streams.WorkloadKindMonitor || work.Kind == streams.WorkloadKindSubagent) && work.State == streams.RunStateUnknown &&
		work.WorkID != "" && work.RunID != "" && work.SourceCallID != ""
}

func isContextResetSetupEvent(event agentctl.AgentEvent) bool {
	switch event.Type {
	case streams.EventTypeSessionStatus,
		streams.EventTypeSessionMode,
		streams.EventTypeSessionModels,
		streams.EventTypeAgentCapabilities,
		streams.EventTypeAvailableCommands,
		streams.EventTypeContextWindow:
		return true
	default:
		return false
	}
}
