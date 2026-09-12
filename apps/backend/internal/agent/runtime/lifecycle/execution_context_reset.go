package lifecycle

import (
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// ErrContextResetFenced means a reset attempt may still have provider-side
// effects, so prompt and session-configuration admission is closed until an
// explicit process restart succeeds.
var ErrContextResetFenced = errors.New("agent context reset requires explicit recovery")

var errContextResetInProgress = errors.New("agent context reset is in progress")

const maxContextResetEvents = 64

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
	return nil
}

// finishContextReset commits the attempt boundary and returns only setup
// events for the session that the caller just committed. Events from the old
// session can be queued while session/new is waiting and must never be replayed
// into the replacement conversation.
func (e *AgentExecution) finishContextReset(newSessionID string) []agentctl.AgentEvent {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	e.contextResetInFlight = false
	buffered := e.contextResetEvents
	e.contextResetEvents = nil
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

func (e *AgentExecution) failContextReset(fence bool, reason string) {
	e.contextResetMu.Lock()
	defer e.contextResetMu.Unlock()
	e.contextResetInFlight = false
	e.contextResetRecoveryInFlight = false
	e.contextResetEvents = nil
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
	if isContextResetSetupEvent(event) && len(e.contextResetEvents) < maxContextResetEvents {
		e.contextResetEvents = append(e.contextResetEvents, event)
	}
	return true
}

func isContextResetSetupEvent(event agentctl.AgentEvent) bool {
	switch event.Type {
	case streams.EventTypeSessionStatus,
		streams.EventTypeSessionMode,
		streams.EventTypeSessionModels,
		"agent_capabilities",
		"available_commands",
		"context_window":
		return true
	default:
		return false
	}
}
