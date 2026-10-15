package lifecycle

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
)

const remotePassthroughSubmitted = "submitted"

func (m *Manager) observeRemotePassthrough(execution *AgentExecution) {
	if !isSSHPassthrough(execution) || m.IsShuttingDown() {
		return
	}
	execution.remotePassthroughMu.Lock()
	snapshot := execution.remotePassthrough.snapshot
	if snapshot.ProcessID == "" || execution.remotePassthrough.observerTarget == snapshot.PassthroughTarget {
		execution.remotePassthroughMu.Unlock()
		return
	}
	if cancel := execution.remotePassthrough.observerCancel; cancel != nil {
		cancel()
	}
	parent := m.stopContext
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	execution.remotePassthrough.observerGeneration++
	generation := execution.remotePassthrough.observerGeneration
	execution.remotePassthrough.observerCancel = cancel
	execution.remotePassthrough.observerTarget = snapshot.PassthroughTarget
	execution.remotePassthroughMu.Unlock()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		defer execution.releaseRemotePassthroughObserver(generation)
		m.runRemotePassthroughEvents(ctx, execution, snapshot)
	}()
}

func (m *Manager) runRemotePassthroughEvents(ctx context.Context, execution *AgentExecution, snapshot types.PassthroughSnapshot) {
	after := snapshot.StartSequence
	record := remotePassthroughRecordFor(execution)
	if record.PassthroughTarget == snapshot.PassthroughTarget && record.EventSequence > after {
		after = record.EventSequence
	}
	for ctx.Err() == nil {
		client, release := execution.AcquireAgentCtlClient()
		if client == nil {
			release()
			return
		}
		conn, err := client.StreamPassthroughEvents(ctx, after)
		release()
		if err == nil {
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			after = m.consumeRemotePassthroughEvents(conn, execution, client, snapshot, after)
			stop()
			_ = conn.Close()
		}
		if waitPassthroughChunkDelay(ctx, time.Second) != nil {
			return
		}
	}
}

func (m *Manager) consumeRemotePassthroughEvents(conn *websocket.Conn, execution *AgentExecution, client *agentctl.Client, snapshot types.PassthroughSnapshot, after uint64) uint64 {
	conn.SetReadLimit(1024 * 1024)
	for {
		var page types.PassthroughEventPage
		if err := conn.ReadJSON(&page); err != nil {
			return after
		}
		if page.Gap {
			return m.reconcileRemotePassthroughGap(execution, client, snapshot, after)
		}
		for _, event := range page.Events {
			if event.Sequence <= after {
				continue
			}
			after = event.Sequence
			if event.InstanceID != snapshot.InstanceID || event.SessionID != snapshot.SessionID || event.PassthroughTarget != snapshot.PassthroughTarget {
				continue
			}
			execution.withAgentCtlClient(client, func() { m.handleRemotePassthroughEvent(execution, event) })
		}
	}
}

func (m *Manager) reconcileRemotePassthroughGap(execution *AgentExecution, client *agentctl.Client, source types.PassthroughSnapshot, after uint64) uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fresh, err := client.GetPassthrough(ctx)
	if err != nil || fresh.InstanceID != source.InstanceID || fresh.SessionID != source.SessionID || fresh.PassthroughTarget != source.PassthroughTarget {
		return after
	}
	accepted := after
	execution.withAgentCtlClient(client, func() {
		if !m.passthroughProcessMatches(execution, source.ProcessID) {
			return
		}
		if err := m.updateRemotePassthroughRecord(ctx, execution, func(record *remotePassthroughRecord) {
			if record.PassthroughTarget == fresh.PassthroughTarget && record.InstanceID == fresh.InstanceID && fresh.Sequence >= record.EventSequence {
				record.EventSequence = fresh.Sequence
				accepted = fresh.Sequence
			}
		}); err != nil {
			accepted = after
			return
		}
		if accepted == fresh.Sequence && fresh.Status != types.ProcessStatusRunning {
			m.handlePassthroughStatus(&types.ProcessStatusUpdate{SessionID: fresh.SessionID, ProcessID: fresh.ProcessID, Status: fresh.Status, ExitCode: fresh.ExitCode, Timestamp: time.Now().UTC()})
		}
		// An idle snapshot cannot attribute a lost completion to the active turn.
	})
	return accepted
}

func (m *Manager) handleRemotePassthroughEvent(execution *AgentExecution, event types.PassthroughEvent) {
	current, exists := m.executionStore.GetBySessionID(event.SessionID)
	if !exists || current != execution || !m.passthroughProcessMatches(execution, event.ProcessID) {
		return
	}
	record := remotePassthroughRecordFor(execution)
	if record.PassthroughTarget != event.PassthroughTarget || record.InstanceID != event.InstanceID || event.Sequence <= record.EventSequence {
		return
	}
	switch event.Kind {
	case "idle":
		m.handlePassthroughTurnCompleteAfter(event.SessionID, event.ProcessID, event.Sequence)
	case remotePassthroughSubmitted:
		_ = m.MarkPassthroughRunning(event.SessionID)
	case "status":
		m.handlePassthroughStatus(&types.ProcessStatusUpdate{SessionID: event.SessionID, ProcessID: event.ProcessID, Status: event.Status, ExitCode: event.ExitCode, Timestamp: event.Timestamp})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = m.updateRemotePassthroughRecord(ctx, execution, func(current *remotePassthroughRecord) {
		if current.PassthroughTarget == event.PassthroughTarget && current.EventSequence < event.Sequence {
			current.EventSequence = event.Sequence
		}
	})
}

func (execution *AgentExecution) releaseRemotePassthroughObserver(generation uint64) {
	execution.remotePassthroughMu.Lock()
	defer execution.remotePassthroughMu.Unlock()
	if execution.remotePassthrough.observerGeneration == generation {
		execution.remotePassthrough.observerTarget = types.PassthroughTarget{}
		execution.remotePassthrough.observerCancel = nil
	}
}
