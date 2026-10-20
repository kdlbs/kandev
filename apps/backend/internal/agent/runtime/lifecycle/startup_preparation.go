package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// PrepareStartupRecovery records unresolved owners without contacting their runtimes.
func (m *Manager) PrepareStartupRecovery(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	local, localErr := m.ListLiveStandaloneExecutorsRunning(ctx)
	remote, remoteErr := m.ListLiveRemoteExecutorsRunning(ctx)
	plugins, pluginErr := m.ListLivePluginExecutorsRunning(ctx)
	if err := errors.Join(localErr, remoteErr, pluginErr); err != nil {
		return err
	}
	records := append(append([]*models.ExecutorRunning(nil), local...), remote...)
	records = append(records, plugins...)
	lookup := m.passthroughLookup
	if lookup == nil {
		lookup = func(context.Context, string) (bool, bool) { return false, false }
	}
	for _, id := range SessionsToGuard(ctx, sessionIDsFromExecutorRunning(records), lookup) {
		m.recoveryGuard.AcquireOrObserve(id)
	}
	for _, record := range append(remote, plugins...) {
		if record != nil && !remoteRecoveryOwnerUnsupported(record) &&
			(record.Runtime != agentruntime.RuntimePluginRemote || !isTerminalExecutorRunningStatus(record.Status)) {
			m.queueRemoteRecovery(record, nil)
		}
	}
	return ctx.Err()
}

func (m *Manager) checkStartupRuntimeHealth(ctx context.Context, deferRemote bool) {
	// Check health of all registered runtimes
	healthResults := map[executor.Name]error{}
	if !deferRemote {
		healthResults = m.executorRegistry.HealthCheckAll(ctx)
	} else if local, err := m.executorRegistry.GetBackend(executor.NameStandalone); err == nil {
		healthResults[executor.NameStandalone] = local.HealthCheck(ctx)
	}
	for name, err := range healthResults {
		if err != nil {
			m.logger.Warn("runtime health check failed",
				zap.String("runtime", string(name)),
				zap.Error(err))
		} else {
			m.logger.Info("runtime is healthy", zap.String("runtime", string(name)))
		}
	}

}

func (m *Manager) initialRemoteRecoveryRecords(ctx context.Context, pluginRecords, remoteRecords []*models.ExecutorRunning) []*models.ExecutorRunning {
	remoteRecordsForRecovery := make([]*models.ExecutorRunning, 0, len(pluginRecords)+len(remoteRecords))
	pluginTerminalRecords := make([]*models.ExecutorRunning, 0, len(pluginRecords))
	pluginLiveRecords := make([]*models.ExecutorRunning, 0, len(pluginRecords))
	for _, record := range pluginRecords {
		if record == nil || remoteRecoveryOwnerUnsupported(record) {
			continue
		}
		copyRecord := cloneRemoteRecoveryRecord(record)
		copyRecord.TransientAuthToken = m.revealRuntimeSecret(ctx, copyRecord.Metadata, MetadataKeyAuthTokenSecret)
		copyRecord.TransientBootstrapNonce = m.revealRuntimeSecret(ctx, copyRecord.Metadata, MetadataKeyBootstrapNonceSecret)
		if isTerminalExecutorRunningStatus(copyRecord.Status) {
			pluginTerminalRecords = append(pluginTerminalRecords, copyRecord)
		} else {
			pluginLiveRecords = append(pluginLiveRecords, copyRecord)
		}
	}
	eligibleRemote := m.establishRemoteRecoveryCandidates(ctx, append(remoteRecords, pluginLiveRecords...))
	eligiblePlugin := make([]*models.ExecutorRunning, 0, len(eligibleRemote))
	eligibleBuiltInRemote := make([]*models.ExecutorRunning, 0, len(eligibleRemote))
	for _, record := range eligibleRemote {
		if record.Runtime == agentruntime.RuntimePluginRemote {
			credentialed := m.revealRemoteRecoveryCredentials(ctx, record)
			eligiblePlugin = append(eligiblePlugin, credentialed)
		} else {
			eligibleBuiltInRemote = append(eligibleBuiltInRemote, m.revealRemoteRecoveryCredentials(ctx, record))
		}
	}
	remoteRecordsForRecovery = append(remoteRecordsForRecovery, pluginTerminalRecords...)
	remoteRecordsForRecovery = append(remoteRecordsForRecovery, eligiblePlugin...)
	remoteRecordsForRecovery = append(remoteRecordsForRecovery, eligibleBuiltInRemote...)
	return remoteRecordsForRecovery
}

// ReconcileStartupPluginInventory repairs unfinished terminal operations after plugin boot.
// Stable stopped environments remain available for explicit resumption without a probe.
func (m *Manager) ReconcileStartupPluginInventory(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	records, err := m.ListLivePluginExecutorsRunning(ctx)
	if err != nil {
		return err
	}
	pending := make([]*models.ExecutorRunning, 0)
	for _, record := range records {
		if record == nil || !isTerminalExecutorRunningStatus(record.Status) || remoteRecoveryOwnerUnsupported(record) {
			continue
		}
		inventory, err := decodePluginExecutorInventory(record.Metadata)
		if err != nil {
			m.logger.Warn("skipping invalid plugin executor inventory during startup repair", zap.String("session_id", record.SessionID), zap.Error(err))
			continue
		}
		if inventory.Phase != pluginExecutorPhaseAllocating && inventory.Phase != pluginExecutorPhaseCleanupPending {
			continue
		}
		pending = append(pending, m.revealRemoteRecoveryCredentials(ctx, record))
	}
	if len(pending) == 0 {
		return nil
	}
	backend, err := m.executorRegistry.GetBackend(executor.NamePluginRemote)
	if err != nil {
		return err
	}
	_, err = backend.RecoverInstances(ctx, pending)
	return err
}
