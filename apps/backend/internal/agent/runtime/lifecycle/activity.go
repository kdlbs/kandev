package lifecycle

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	executionActivityPrefix = "execution:"
	processActivityPrefix   = "process:"
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
	previous := m.activityLeases[key]
	m.activityLeases[key] = lease
	m.activityMu.Unlock()
	previous.Release()
}

func (m *Manager) releaseActivity(key string) {
	m.activityMu.Lock()
	lease := m.activityLeases[key]
	delete(m.activityLeases, key)
	m.activityMu.Unlock()
	lease.Release()
}

func (m *Manager) ensureExecutionActivity(ctx context.Context, executionID string, kind activity.Kind) error {
	key := executionActivityKey(executionID)
	m.activityMu.Lock()
	existing := m.activityLeases[key]
	m.activityMu.Unlock()
	if existing != nil {
		existing.SetKind(kind)
		return nil
	}
	lease, err := m.acquireActivity(ctx, kind)
	if err != nil {
		return err
	}
	m.trackActivity(key, lease)
	return nil
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
