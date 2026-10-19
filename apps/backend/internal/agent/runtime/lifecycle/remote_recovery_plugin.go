package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) remoteRecoveryAttemptRecord(ctx context.Context, entry *remoteRecoveryRetry) (*models.ExecutorRunning, error) {
	if entry == nil || entry.record == nil {
		return nil, errors.New("remote recovery attempt record is unavailable")
	}
	record := cloneRemoteRecoveryRecord(entry.record)
	if record.Runtime != agentruntime.RuntimePluginRemote {
		return record, nil
	}
	return m.currentPluginRemoteRecoveryAttemptRecord(ctx, record)
}

func (m *Manager) currentPluginRemoteRecoveryAttemptRecord(
	ctx context.Context,
	record *models.ExecutorRunning,
) (*models.ExecutorRunning, error) {
	reader, ok := m.runningWriter.(pluginExecutorInventoryReader)
	if !ok {
		return nil, errors.New("plugin remote recovery inventory reader is unavailable")
	}
	current, err := reader.GetExecutorRunningBySessionID(ctx, record.SessionID)
	if err != nil {
		return nil, fmt.Errorf("read current plugin remote recovery inventory: %w", err)
	}
	if !samePluginRecoveryRecordOwner(record, current) {
		return nil, errors.New("plugin remote recovery inventory owner changed")
	}
	if err := validatePluginRecoveryCheckpoint(record, current); err != nil {
		return nil, err
	}
	if record.Metadata == nil {
		record.Metadata = make(map[string]interface{})
	}
	record.Metadata[MetadataKeyPluginExecutor] = current.Metadata[MetadataKeyPluginExecutor]
	return record, nil
}

func samePluginRecoveryRecordOwner(record, current *models.ExecutorRunning) bool {
	return current != nil && current.SessionID == record.SessionID && current.TaskID == record.TaskID &&
		current.AgentExecutionID == record.AgentExecutionID && current.Runtime == record.Runtime
}

func validatePluginRecoveryCheckpoint(record, current *models.ExecutorRunning) error {
	initialInventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return fmt.Errorf("decode pinned plugin recovery inventory: %w", err)
	}
	currentInventory, err := decodePluginExecutorInventory(current.Metadata)
	if err != nil {
		return fmt.Errorf("decode current plugin recovery inventory: %w", err)
	}
	if !samePluginRecoveryCheckpointOwner(initialInventory, currentInventory) ||
		currentInventory.Revision < initialInventory.Revision {
		return errors.New("plugin remote recovery checkpoint identity changed")
	}
	return nil
}

func samePluginRecoveryCheckpointOwner(initial, current pluginExecutorInventory) bool {
	if !samePluginRecoveryCheckpointBase(initial, current) {
		return false
	}
	return samePluginRecoveryRuntimeIdentity(initial, current) && samePluginRecoveryResource(initial, current)
}

func samePluginRecoveryCheckpointBase(initial, current pluginExecutorInventory) bool {
	return initial.PluginID == current.PluginID && initial.InstallationID == current.InstallationID &&
		initial.ProviderKey == current.ProviderKey && initial.ProviderIdentity == current.ProviderIdentity &&
		initial.EnvironmentID == current.EnvironmentID && initial.EnvironmentGeneration == current.EnvironmentGeneration &&
		initial.ContractVersion == current.ContractVersion && initial.ProfileID == current.ProfileID &&
		initial.OperationID == current.OperationID && initial.InputDigest == current.InputDigest
}

func samePluginRecoveryRuntimeIdentity(initial, current pluginExecutorInventory) bool {
	return initial.RuntimeIdentity == "" || current.RuntimeIdentity == initial.RuntimeIdentity
}

func samePluginRecoveryResource(initial, current pluginExecutorInventory) bool {
	if initial.Resource == nil {
		return true
	}
	return current.Resource != nil && current.Resource.GetResourceHandle() == initial.Resource.GetResourceHandle()
}
