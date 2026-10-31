package lifecycle

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

type deliveryInstanceReconnector interface {
	ReconnectDeliveryInstance(context.Context, *ExecutorInstance) (*RemoteInstanceRefresh, error)
}

func (m *Manager) reconnectSSHDelivery(ctx context.Context, execution *AgentExecution) error {
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if current, ok := m.executionStore.Get(execution.ID); !ok || current != execution {
		return ErrDeliveryOwnerMismatch
	}
	backend, err := m.executorRegistry.GetBackend(execution.RuntimeName)
	if err != nil {
		return err
	}
	reconnector, ok := backend.(deliveryInstanceReconnector)
	if !ok {
		return ErrDeliveryTransportUnavailable
	}
	refresh, err := reconnector.ReconnectDeliveryInstance(ctx, m.remoteStatusInstance(ctx, execution))
	if err != nil || refresh == nil {
		return err
	}
	defer refresh.Abort()
	if refresh.Instance == nil || refresh.Instance.Client == nil || refresh.Commit == nil || refresh.ProcessRestarted {
		return ErrDeliveryOwnerMismatch
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	rollback := execution.mergeMetadataWithRollback(refresh.Instance.Metadata)
	if err = m.persistExecutorRunningResult(ctx, execution); err != nil {
		rollback()
		return err
	}
	committed, err := m.commitTrackedRemoteRefresh(ctx, execution, refresh, "", func(rollbackCtx context.Context) error {
		rollback()
		return m.persistExecutorRunningResult(rollbackCtx, execution)
	})
	if err != nil {
		return err
	}
	if !committed {
		return ErrDeliveryOwnerMismatch
	}
	return m.persistExecutorRunningResult(ctx, execution)
}

func (m *Manager) recoverSavedSSHDelivery(ctx context.Context, requested AgentDeliveryRecoveryIdentity) (bool, error) {
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return false, nil
	}
	record, err := reader.GetExecutorRunningBySessionID(ctx, requested.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if record == nil || record.Runtime != agentruntime.RuntimeSSH {
		return false, nil
	}
	if record.TaskID != requested.TaskID || record.AgentExecutionID != requested.ExecutionID {
		return true, ErrDeliveryOwnerMismatch
	}
	if m.recoveryGuard == nil {
		return true, ErrDeliveryTransportUnavailable
	}
	candidates := m.establishRemoteRecoveryCandidates(ctx, []*models.ExecutorRunning{record})
	if len(candidates) == 0 {
		return true, ErrDeliveryTransportUnavailable
	}
	m.retryRemoteRecovery(ctx, m.pendingRemoteRecoveryForSession(requested.SessionID))
	if _, exists := m.GetExecutionBySessionID(requested.SessionID); !exists {
		return true, ErrDeliveryTransportUnavailable
	}
	return true, nil
}
