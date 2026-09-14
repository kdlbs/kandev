package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// persistedSSHCleanupStore is intentionally narrower than the task
// repository. It lets a stop request recover the executors_running row after
// a backend restart without making ordinary lifecycle code depend on the
// repository implementation.
type persistedSSHCleanupStore interface {
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
}

// stopPersistedSSHExecution handles the restart-only case where the
// authoritative executors_running row survived but process-local lifecycle
// state (the SSH client, forwarder, and remote pid tracked by SSHExecutor)
// did not. Unlike Kubernetes, an SSH connection is fully described by the
// row's own persisted metadata (host, port, user, fingerprint, remote pid,
// remote session dir — see persistentMetadataKeys), so no reconciliation
// against a "current" executor config is needed before dispatching the stop.
// The caller that owns task cleanup remains responsible for deleting the row
// after this exact remote teardown succeeds.
func (m *Manager) stopPersistedSSHExecution(
	ctx context.Context,
	executionID string,
	reason string,
	force bool,
) (bool, error) {
	store, ok := m.runningWriter.(persistedSSHCleanupStore)
	if !ok {
		return false, nil
	}
	row, err := loadPersistedSSHExecution(ctx, store, executionID)
	if err != nil {
		return true, err
	}
	if row == nil || row.Runtime != agentruntime.RuntimeSSH {
		return false, nil
	}

	activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStopping)
	if err != nil {
		return true, err
	}
	defer activityLease.Release()

	instance := &ExecutorInstance{
		InstanceID:  row.AgentExecutionID,
		TaskID:      row.TaskID,
		SessionID:   row.SessionID,
		RuntimeName: agentruntime.RuntimeSSH,
		Metadata:    row.Metadata,
		StopReason:  reason,
	}
	if err := m.stopPersistedSSHBackend(ctx, instance, force); err != nil {
		return true, fmt.Errorf("stop persisted SSH runtime: %w", err)
	}
	return true, nil
}

func loadPersistedSSHExecution(
	ctx context.Context,
	store persistedSSHCleanupStore,
	executionID string,
) (*models.ExecutorRunning, error) {
	rows, err := store.ListExecutorsRunning(ctx)
	if err != nil {
		return nil, fmt.Errorf("load persisted runtime inventory: %w", err)
	}
	return uniquePersistedSSHExecution(rows, executionID)
}

func (m *Manager) stopPersistedSSHBackend(
	ctx context.Context,
	instance *ExecutorInstance,
	force bool,
) error {
	if m.executorRegistry == nil {
		return errors.New("ssh runtime registry is unavailable")
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameSSH)
	if err != nil {
		return fmt.Errorf("get SSH runtime: %w", err)
	}
	return backend.StopInstance(ctx, instance, force)
}

func uniquePersistedSSHExecution(
	rows []*models.ExecutorRunning,
	executionID string,
) (*models.ExecutorRunning, error) {
	var found *models.ExecutorRunning
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.AgentExecutionID) != executionID {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("multiple persisted runtime rows match execution %q", executionID)
		}
		found = row
	}
	return found, nil
}
