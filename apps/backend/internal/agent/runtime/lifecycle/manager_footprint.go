package lifecycle

import (
	"context"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
)

const footprintSnapshotTimeout = 15 * time.Second

// RuntimeFootprintReader is implemented only by a host-local runtime owner.
type RuntimeFootprintReader interface {
	ReadRuntimeFootprints(context.Context) ([]agentctl.RuntimeFootprint, error)
}

// SnapshotRuntimeFootprint is a bounded request, without an observer or history.
func (m *Manager) SnapshotRuntimeFootprint(ctx context.Context) agentruntime.RuntimeFootprintSnapshot {
	var snapshot agentruntime.RuntimeFootprintSnapshot
	if m == nil || m.executorRegistry == nil {
		return snapshot
	}
	reader, ok := m.executorRegistry.RuntimeFootprintReader()
	if !ok {
		return snapshot
	}
	readCtx, cancel := context.WithTimeout(ctx, footprintSnapshotTimeout)
	defer cancel()
	measured, err := reader.ReadRuntimeFootprints(readCtx)
	if err != nil || readCtx.Err() != nil {
		return snapshot
	}
	snapshot.Complete = true
	for _, footprint := range measured {
		row, attributed := m.resolveFootprintRow(footprint)
		addFootprintReading(&snapshot, footprint, row, attributed)
	}
	return snapshot
}

// Only the current exact task/session/instance binding receives attribution.
func (m *Manager) resolveFootprintRow(footprint agentctl.RuntimeFootprint) (agentruntime.SessionRuntimeFootprint, bool) {
	var row agentruntime.SessionRuntimeFootprint
	if footprint.SessionID == "" || footprint.TaskID == "" || footprint.InstanceID == "" || m.executionStore == nil {
		return row, false
	}
	execution, exists := m.executionStore.GetBySessionID(footprint.SessionID)
	if !exists || execution == nil {
		return row, false
	}
	matched := false
	err := m.executionStore.WithRLock(execution.ID, func(current *AgentExecution) {
		if current != execution || current.SessionID != footprint.SessionID || current.TaskID != footprint.TaskID ||
			current.standaloneInstanceID != footprint.InstanceID {
			return
		}
		row = agentruntime.SessionRuntimeFootprint{
			SessionID: current.SessionID, TaskID: current.TaskID,
			ExecutionID: current.ID, Runtime: string(current.RuntimeName),
		}
		matched = true
	})
	return row, err == nil && matched
}

func addFootprintReading(snapshot *agentruntime.RuntimeFootprintSnapshot, measured agentctl.RuntimeFootprint, row agentruntime.SessionRuntimeFootprint, attributed bool) {
	snapshot.LiveRuntimes++
	snapshot.ProcessCount += measured.Processes
	snapshot.CommittedBytes += measured.CommittedBytes
	snapshot.ResidentBytes += measured.ResidentBytes
	if measured.UnreadableCount > 0 {
		snapshot.UnreadableRuntimes++
	}
	if !attributed {
		return
	}
	row.Status, row.Processes = measured.Status, measured.Processes
	row.CommittedBytes, row.ResidentBytes = measured.CommittedBytes, measured.ResidentBytes
	row.UnreadableProcesses = measured.UnreadableCount
	row.LastActivityAt = measured.LastActivity
	if measured.CreatedAt.After(row.LastActivityAt) {
		row.LastActivityAt = measured.CreatedAt
	}
	snapshot.Runtimes = append(snapshot.Runtimes, row)
}
