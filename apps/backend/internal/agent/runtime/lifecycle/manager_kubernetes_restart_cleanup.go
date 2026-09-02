package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/executor"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// persistedKubernetesCleanupStore is intentionally narrower than the task
// repository. It lets terminal task cleanup recover exact Kubernetes inventory
// after a backend restart without making ordinary lifecycle code depend on the
// repository implementation.
type persistedKubernetesCleanupStore interface {
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
}

// stopPersistedKubernetesExecution handles the restart-only case where the
// authoritative executors_running row survived but process-local lifecycle
// state did not. The caller that owns task cleanup remains responsible for
// deleting the row after this exact remote teardown succeeds.
func (m *Manager) stopPersistedKubernetesExecution(
	ctx context.Context,
	executionID string,
	reason string,
	force bool,
) (bool, error) {
	if !force && !shouldRunExecutorCleanup(reason) {
		return false, nil
	}
	store, ok := m.runningWriter.(persistedKubernetesCleanupStore)
	if !ok {
		return false, nil
	}
	row, err := loadPersistedKubernetesExecution(ctx, store, executionID)
	if err != nil {
		return true, err
	}
	if row == nil || row.Runtime != agentruntime.RuntimeKubernetes {
		return false, nil
	}

	activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStopping)
	if err != nil {
		return true, err
	}
	defer activityLease.Release()
	metadata, err := resolvePersistedKubernetesCleanupMetadata(ctx, store, row)
	if err != nil {
		return true, err
	}
	instance := &ExecutorInstance{
		InstanceID:  row.AgentExecutionID,
		TaskID:      row.TaskID,
		SessionID:   row.SessionID,
		RuntimeName: agentruntime.RuntimeKubernetes,
		Metadata:    metadata,
		StopReason:  reason,
	}
	if err := m.stopPersistedKubernetesBackend(ctx, instance, force); err != nil {
		return true, fmt.Errorf("stop persisted Kubernetes runtime: %w", err)
	}
	if err := m.deleteKubernetesRuntimeSecrets(ctx, metadata); err != nil {
		return true, fmt.Errorf("delete persisted Kubernetes runtime secrets: %w", err)
	}
	return true, nil
}

func loadPersistedKubernetesExecution(
	ctx context.Context,
	store persistedKubernetesCleanupStore,
	executionID string,
) (*models.ExecutorRunning, error) {
	rows, err := store.ListExecutorsRunning(ctx)
	if err != nil {
		return nil, fmt.Errorf("load persisted runtime inventory: %w", err)
	}
	return uniquePersistedKubernetesExecution(rows, executionID)
}

func resolvePersistedKubernetesCleanupMetadata(
	ctx context.Context,
	store persistedKubernetesCleanupStore,
	row *models.ExecutorRunning,
) (map[string]interface{}, error) {
	currentExecutor, err := store.GetExecutor(ctx, strings.TrimSpace(row.ExecutorID))
	if err != nil {
		return nil, fmt.Errorf("load current Kubernetes executor %q: %w", row.ExecutorID, err)
	}
	if currentExecutor == nil {
		return nil, fmt.Errorf("load current Kubernetes executor %q: %w", row.ExecutorID, models.ErrExecutorNotFound)
	}
	if currentExecutor.Type != models.ExecutorTypeKubernetes {
		return nil, fmt.Errorf(
			"persisted Kubernetes runtime executor %q now has type %q", row.ExecutorID, currentExecutor.Type,
		)
	}
	currentConfig, err := kubeexecutor.ParseExecutorConfig(currentExecutor.Config)
	if err != nil {
		return nil, fmt.Errorf("parse current Kubernetes executor %q: %w", row.ExecutorID, err)
	}
	metadata, err := persistedKubernetesCleanupMetadata(row, currentExecutor.Config, currentConfig)
	if err != nil {
		return nil, err
	}
	req := &ExecutorCreateRequest{TaskID: row.TaskID, SessionID: row.SessionID, Metadata: metadata}
	if _, _, err := kubernetesRecordedCleanupInventory(req, currentConfig, true); err != nil {
		return nil, fmt.Errorf("validate persisted Kubernetes runtime inventory: %w", err)
	}
	return metadata, nil
}

func (m *Manager) stopPersistedKubernetesBackend(
	ctx context.Context,
	instance *ExecutorInstance,
	force bool,
) error {
	if m.executorRegistry == nil {
		return errors.New("kubernetes runtime registry is unavailable")
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameKubernetes)
	if err != nil {
		return fmt.Errorf("get Kubernetes runtime: %w", err)
	}
	return backend.StopInstance(ctx, instance, force)
}

func uniquePersistedKubernetesExecution(
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

func persistedKubernetesCleanupMetadata(
	row *models.ExecutorRunning,
	currentConfigValues map[string]string,
	currentConfig kubeexecutor.ExecutorConfig,
) (map[string]interface{}, error) {
	if row == nil || strings.TrimSpace(row.AgentExecutionID) == "" ||
		strings.TrimSpace(row.TaskID) == "" || strings.TrimSpace(row.SessionID) == "" ||
		strings.TrimSpace(row.ExecutorID) == "" {
		return nil, errors.New("persisted Kubernetes runtime row identity is incomplete")
	}
	metadata := overlayCurrentKubernetesConnectionMetadata(row.Metadata, currentConfigValues, currentConfig)
	metadata["executor_id"] = strings.TrimSpace(row.ExecutorID)
	return metadata, nil
}
