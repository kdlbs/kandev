package lifecycle

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	executionActivityPrefix   = "execution:"
	processActivityPrefix     = "process:"
	managedGoCacheMetadataKey = "managed_go_cache_path"
)

func (m *Manager) acquireActivity(ctx context.Context, kind activity.Kind) (*activity.TaskLease, error) {
	m.activityMu.Lock()
	coordinator := m.activityCoordinator
	m.activityMu.Unlock()
	if coordinator == nil {
		return nil, nil
	}
	return coordinator.AcquireTask(ctx, kind)
}

func (m *Manager) trackActivity(key string, lease *activity.TaskLease) {
	if lease == nil {
		return
	}
	m.activityMu.Lock()
	if m.activityLeases == nil {
		m.activityLeases = make(map[string]*activity.TaskLease)
	}
	delete(m.activityPending, key)
	previous := m.activityLeases[key]
	m.activityLeases[key] = lease
	m.activityMu.Unlock()
	previous.Release()
}

func (m *Manager) releaseActivity(key string) {
	m.activityMu.Lock()
	lease := m.activityLeases[key]
	delete(m.activityLeases, key)
	delete(m.activityPending, key)
	m.activityMu.Unlock()
	lease.Release()
}

func (m *Manager) ensureExecutionActivity(ctx context.Context, executionID string, kind activity.Kind) error {
	key := executionActivityKey(executionID)
	m.activityMu.Lock()
	existing := m.activityLeases[key]
	if existing != nil {
		m.activityMu.Unlock()
		existing.SetKind(kind)
		return nil
	}
	if m.activityPending == nil {
		m.activityPending = make(map[string]uint64)
	}
	m.activityGeneration++
	generation := m.activityGeneration
	m.activityPending[key] = generation
	m.activityMu.Unlock()
	lease, err := m.acquireActivity(ctx, kind)
	if err != nil {
		m.clearPendingActivity(key, generation)
		return err
	}
	if lease == nil {
		m.clearPendingActivity(key, generation)
		return nil
	}
	m.finishPendingActivity(key, generation, lease)
	return nil
}

func (m *Manager) clearPendingActivity(key string, generation uint64) {
	m.activityMu.Lock()
	if m.activityPending[key] == generation {
		delete(m.activityPending, key)
	}
	m.activityMu.Unlock()
}

func (m *Manager) finishPendingActivity(key string, generation uint64, lease *activity.TaskLease) {
	m.activityMu.Lock()
	if m.activityPending[key] != generation {
		m.activityMu.Unlock()
		lease.Release()
		return
	}
	delete(m.activityPending, key)
	previous := m.activityLeases[key]
	m.activityLeases[key] = lease
	m.activityMu.Unlock()
	previous.Release()
}

func executionActivityKey(executionID string) string {
	return executionActivityPrefix + executionID
}

func processActivityKey(processID string) string {
	return processActivityPrefix + processID
}

func processActivityKind(kind string) activity.Kind {
	switch strings.ToLower(kind) {
	case string(streams.ProcessKindSetup):
		return activity.KindSetupScript
	case string(agentctltypes.ProcessKindCleanup):
		return activity.KindCleanupScript
	case "test":
		return activity.KindTestCommand
	default:
		return activity.KindShellCommand
	}
}

func (m *Manager) releaseTerminalProcessActivity(status *agentctltypes.ProcessStatusUpdate) {
	if status == nil {
		return
	}
	switch status.Status {
	case agentctltypes.ProcessStatusExited,
		agentctltypes.ProcessStatusFailed,
		agentctltypes.ProcessStatusStopped:
		m.releaseActivity(processActivityKey(status.ProcessID))
	}
}
