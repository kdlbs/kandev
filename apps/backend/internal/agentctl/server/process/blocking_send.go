package process

import "github.com/kandev/kandev/internal/agentctl/server/adapter"

// sendUpdateBlocking sends event on the updates channel, parking the caller
// while the channel is full rather than discarding the event
// (AC-EXECUTORS-SURVIVAL-001.5/.6). The park selects against the instance's
// current stop signal, mirroring forwardUpdates's own send: stopping the
// instance releases every producer parked on it immediately, without
// waiting for a backend that may never attach. Returns false when the
// instance stopped (or never started) before the event could be delivered.
//
// The fast path never depends on a stop channel: when there is room, the
// send just happens, matching the un-parked behaviour every one of these
// call sites had before conversion. Only a send that would actually block
// needs a stop channel to select against; one that was never established
// (Start has not run) is treated as already stopped rather than parking
// forever with no lifecycle that could ever release it.
func (m *Manager) sendUpdateBlocking(event adapter.AgentEvent) bool {
	m.recordTerminalOutcome(&event)

	select {
	case m.updatesCh <- event:
		return true
	default:
	}

	stopChVal := m.stopChSnapshot.Load()
	if stopChVal == nil {
		return false
	}
	stopCh := stopChVal.(chan struct{})

	select {
	case m.updatesCh <- event:
		return true
	case <-stopCh:
		return false
	}
}
