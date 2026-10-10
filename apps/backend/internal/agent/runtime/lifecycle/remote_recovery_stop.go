package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) stopPendingRemoteRecovery(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	reason string,
	force bool,
) (bool, error) {
	if entry == nil || entry.record == nil || entry.record.AgentExecutionID == "" || entry.record.SessionID == "" {
		return false, errors.New("pending remote recovery identity is incomplete")
	}
	if reason == StopReasonBackendShutdown || reason == StopReasonRecoverableAgentFailure {
		return false, nil
	}
	if entry.stopRuntimeCompleted {
		return m.settlePendingRemoteStop(ctx, entry, reason, force)
	}
	if handled, err := m.stopPersistedPendingRemoteOwner(ctx, entry, reason, force); handled || err != nil {
		return handled, err
	}
	if m.executorRegistry == nil {
		return false, errors.New("remote runtime registry is unavailable")
	}
	if err := m.validatePendingRemoteStopOwner(ctx, entry); err != nil {
		return false, err
	}
	backend, err := m.executorRegistry.GetBackend(entry.record.Runtime)
	if err != nil {
		return false, fmt.Errorf("get persisted remote runtime %s: %w", entry.record.Runtime, err)
	}
	ri, outcome, err := m.recoverPendingRemoteStopInstance(ctx, backend, entry)
	if err != nil {
		return false, fmt.Errorf("attach exact remote execution for stop: %w", err)
	}
	if ri == nil {
		return m.stopAbsentRemoteOwner(ctx, backend, entry, reason, force, outcome)
	}
	return m.stopAttachedRemoteOwner(ctx, backend, entry, ri, reason, force)
}

func (m *Manager) recoverPendingRemoteStopInstance(
	ctx context.Context,
	backend ExecutorBackend,
	entry *remoteRecoveryRetry,
) (*ExecutorInstance, RecoveryCandidateOutcome, error) {
	attemptRecord, err := m.remoteRecoveryAttemptRecord(ctx, entry)
	if err != nil {
		return nil, "", err
	}
	record := m.revealRemoteRecoveryCredentials(ctx, attemptRecord)
	var recovered []*ExecutorInstance
	var outcomes map[string]RecoveryCandidateOutcome
	if detailed, ok := backend.(DetailedRecoveryBackend); ok {
		recovered, outcomes, err = detailed.RecoverInstancesDetailed(ctx, []*models.ExecutorRunning{record})
	} else {
		recovered, err = backend.RecoverInstances(ctx, []*models.ExecutorRunning{record})
	}
	if err != nil {
		m.discardRemoteRecoveryInstances(recovered)
		return nil, "", err
	}
	ri := exactRecoveredRemoteInstance(recovered, entry.record)
	for _, candidate := range recovered {
		if candidate != ri {
			m.discardRemoteRecoveryInstance(candidate)
		}
	}
	return ri, outcomes[entry.record.SessionID], nil
}

func (m *Manager) stopAbsentRemoteOwner(
	ctx context.Context,
	backend ExecutorBackend,
	entry *remoteRecoveryRetry,
	reason string,
	force bool,
	outcome RecoveryCandidateOutcome,
) (bool, error) {
	if outcome != RecoveryOutcomeNoMatchingInstance {
		return false, fmt.Errorf("remote stop could not prove the persisted agent instance is absent: %s", outcome)
	}
	if err := m.validatePendingRemoteStopOwner(ctx, entry); err != nil {
		return false, err
	}
	if !remoteRecoveryNoMatchProvesStopped(entry.record.Runtime) {
		return false, errors.New("runtime did not prove the exact agent process or resource is absent")
	}
	if force || shouldRunExecutorCleanup(reason) {
		instance := persistedRemoteStopInstance(ctx, m, entry.record, reason)
		instance.AgentStopFailed = true
		if err := backend.StopInstance(ctx, instance, force); err != nil {
			return false, fmt.Errorf("clean up exact absent remote agent resource: %w", err)
		}
	}
	entry.stopRuntimeCompleted = true
	return m.settlePendingRemoteStop(ctx, entry, reason, force)
}

func (m *Manager) stopAttachedRemoteOwner(
	ctx context.Context,
	backend ExecutorBackend,
	entry *remoteRecoveryRetry,
	ri *ExecutorInstance,
	reason string,
	force bool,
) (bool, error) {
	if ri.RuntimeName != entry.record.Runtime || ri.SessionID != entry.record.SessionID ||
		ri.InstanceID != entry.record.AgentExecutionID || ri.TaskID != entry.record.TaskID || ri.Client == nil {
		m.discardRemoteRecoveryInstance(ri)
		return false, errors.New("runtime returned a different or unauthenticated remote execution")
	}
	if err := m.captureAuthenticatedAgentSessionEvidence(ctx, ri); err != nil {
		m.discardRemoteRecoveryInstance(ri)
		return false, fmt.Errorf("prove authenticated remote owner before stop: %w", err)
	}
	if err := validateRemoteRecoveryDeliveryEvidence(entry.snapshot, ri); err != nil {
		m.discardRemoteRecoveryInstance(ri)
		return false, err
	}
	if err := m.validatePendingRemoteStopOwner(ctx, entry); err != nil {
		m.discardRemoteRecoveryInstance(ri)
		return false, err
	}
	ri.StopReason = reason
	stopErr := ri.Client.Stop(ctx)
	if stopErr != nil {
		if !shouldRunExecutorCleanup(reason) && !remoteRuntimeForceStopsAgent(entry.record.Runtime, force) {
			m.discardRemoteRecoveryInstance(ri)
			return false, fmt.Errorf("stop authenticated remote agent process: %w", stopErr)
		}
		ri.AgentStopFailed = true
	}
	ri.Client.Close()
	ri.Client = nil
	if ri.RuntimeName == agentruntime.RuntimeKubernetes {
		metadata, metadataErr := m.currentKubernetesConnectionMetadata(ctx, ri.Metadata)
		if metadataErr != nil {
			m.discardRemoteRecoveryInstance(ri)
			return false, fmt.Errorf("resolve current Kubernetes cleanup connection: %w", metadataErr)
		}
		ri.Metadata = metadata
	}
	if err := backend.StopInstance(ctx, ri, force); err != nil {
		m.discardRemoteRecoveryInstance(ri)
		return false, fmt.Errorf("stop exact remote runtime execution %q: %w", entry.record.AgentExecutionID, err)
	}
	// StopInstance owns runtime-specific session disposal. The authenticated
	// client was already closed after agentctl accepted the exact stop.
	ri.DiscardRecovery = nil
	entry.stopRuntimeCompleted = true
	return m.settlePendingRemoteStop(ctx, entry, reason, force)
}

func (m *Manager) stopPersistedPendingRemoteOwner(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	reason string,
	force bool,
) (bool, error) {
	var handled bool
	var err error
	switch entry.record.Runtime {
	case agentruntime.RuntimeSSH:
		if err = m.validatePendingRemoteStopOwner(ctx, entry); err == nil {
			handled, err = m.stopPersistedSSHExecution(ctx, entry.record.AgentExecutionID, reason, force)
		}
	case agentruntime.RuntimeKubernetes:
		if err = m.validatePendingRemoteStopOwner(ctx, entry); err == nil {
			handled, err = m.stopPersistedKubernetesExecution(ctx, entry.record.AgentExecutionID, reason, force)
		}
	default:
		return false, nil
	}
	if err != nil || !handled {
		return handled, err
	}
	entry.stopRuntimeCompleted = true
	m.deleteExecutorRunning(ctx, entry.record.SessionID, entry.record.AgentExecutionID)
	return true, nil
}

func (m *Manager) settlePendingRemoteStop(
	ctx context.Context,
	entry *remoteRecoveryRetry,
	reason string,
	force bool,
) (bool, error) {
	if entry.record.Runtime == agentruntime.RuntimeKubernetes && (force || shouldRunExecutorCleanup(reason)) {
		metadata, err := m.currentKubernetesConnectionMetadata(ctx, entry.record.Metadata)
		if err != nil {
			return false, fmt.Errorf("resolve current Kubernetes cleanup connection: %w", err)
		}
		cleanupCtx, cancel := kubernetesDurableContext(ctx)
		cleanupErr := m.deleteKubernetesRuntimeSecrets(cleanupCtx, metadata)
		cancel()
		if cleanupErr != nil {
			return false, fmt.Errorf("delete Kubernetes runtime secrets: %w", cleanupErr)
		}
	}
	m.deleteExecutorRunning(ctx, entry.record.SessionID, entry.record.AgentExecutionID)
	return true, nil
}

func (m *Manager) validatePendingRemoteStopOwner(ctx context.Context, entry *remoteRecoveryRetry) error {
	current, err := m.remoteRecoverySnapshot(ctx, entry.record.SessionID)
	if err != nil {
		return fmt.Errorf("read remote owner before explicit stop: %w", err)
	}
	if entry.snapshot == nil {
		if !remoteRecoverySnapshotMatchesRecord(current, entry.record) {
			return errRemoteRecoveryFenced
		}
		if !m.pinRemoteRecoverySnapshot(entry, current) {
			return errRemoteRecoveryFenced
		}
	}
	if !sameRemoteRecoveryOwner(entry.snapshot, current, entry.record) {
		m.removeRemoteRecovery(entry, true)
		return errRemoteRecoveryFenced
	}
	return nil
}

func remoteRecoveryNoMatchProvesStopped(runtime agentruntime.Runtime) bool {
	switch runtime {
	case agentruntime.RuntimeRemoteDocker, agentruntime.RuntimeSprites:
		return true
	default:
		return false
	}
}

func remoteRuntimeForceStopsAgent(runtime agentruntime.Runtime, force bool) bool {
	if !force {
		return false
	}
	switch runtime {
	case agentruntime.RuntimeSSH, agentruntime.RuntimeRemoteDocker, agentruntime.RuntimeKubernetes:
		return true
	default:
		return false
	}
}

func persistedRemoteStopInstance(
	ctx context.Context,
	manager *Manager,
	record *models.ExecutorRunning,
	reason string,
) *ExecutorInstance {
	instance := &ExecutorInstance{
		InstanceID:     record.AgentExecutionID,
		TaskID:         record.TaskID,
		SessionID:      record.SessionID,
		RuntimeName:    record.Runtime,
		ContainerID:    record.ContainerID,
		WorkspacePath:  record.WorktreePath,
		Metadata:       record.Metadata,
		StopReason:     reason,
		AuthToken:      manager.revealRuntimeSecret(ctx, record.Metadata, MetadataKeyAuthTokenSecret),
		BootstrapNonce: manager.revealRuntimeSecret(ctx, record.Metadata, MetadataKeyBootstrapNonceSecret),
	}
	return instance
}
