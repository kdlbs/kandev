package lifecycle

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"time"
)

// SetExecutorObservationHandler installs the durable task-owned admission path.
func (m *Manager) SetExecutorObservationHandler(handler func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) error) {
	m.executorObservationHandler = handler
}

func (m *Manager) inspectManagedDisconnect(execution *AgentExecution, promptGeneration, startupGeneration uint64) bool {
	if execution.intentionalStopInProgress.Load() || execution.idleSuspensionInProgress.Load() {
		return true
	}
	if m.executorObservationHandler == nil || !execution.isSessionInitialized() || m.IsShuttingDown() || !m.supportsExecutorInspection(execution.RuntimeName) {
		return false
	}
	key := fmt.Sprintf("%s:%d:%d", execution.ID, startupGeneration, promptGeneration)
	if _, loaded := m.executorDisconnectInspections.LoadOrStore(key, struct{}{}); loaded {
		return true
	}
	work := func() {
		defer m.executorDisconnectInspections.Delete(key)
		m.classifyExecutorDisconnect(execution, promptGeneration, startupGeneration)
	}
	if m.streamManager != nil {
		m.streamManager.start(work)
	} else {
		go work()
	}
	return true
}

func (m *Manager) disconnectStillCurrent(execution *AgentExecution, prompt, startup uint64) bool {
	current, ok := m.executionStore.GetBySessionID(execution.SessionID)
	return ok && current == execution && !m.IsShuttingDown() && !execution.intentionalStopInProgress.Load() && !execution.idleSuspensionInProgress.Load() && execution.startupAttemptSnapshot() == startup && execution.promptGenerationSnapshot() == prompt
}

func (m *Manager) executorObservationTarget(ctx context.Context, execution *AgentExecution) (models.ExecutorObservationTarget, error) {
	target := models.ExecutorObservationTarget{TaskID: execution.TaskID, EnvironmentID: execution.TaskEnvironmentID, SessionID: execution.SessionID, ExecutionID: execution.ID, Runtime: string(execution.RuntimeName), ContainerID: execution.ContainerID, ResourceKey: execution.ContainerID, Metadata: execution.MetadataSnapshot()}
	if execution.RuntimeName == agentruntime.RuntimeKubernetes {
		target.ResourceKey = getMetadataString(target.Metadata, MetadataKeyKubernetesPodUID)
	}
	if execution.RuntimeName == agentruntime.RuntimeStandalone {
		return m.localExecutorObservationTarget(ctx, execution, target)
	}

	return m.completeExecutorObservationTarget(ctx, execution, target)
}

func (m *Manager) completeExecutorObservationTarget(ctx context.Context, execution *AgentExecution, target models.ExecutorObservationTarget) (models.ExecutorObservationTarget, error) {
	if target.EnvironmentID == "" {
		return m.legacyExecutorObservationTarget(ctx, execution, target)
	}
	reader, ok := m.runningWriter.(interface {
		GetTaskEnvironment(context.Context, string) (*models.TaskEnvironment, error)
	})
	if !ok {
		return target, fmt.Errorf("executor environment authority unavailable")
	}
	environment, err := reader.GetTaskEnvironment(ctx, target.EnvironmentID)
	if err != nil {
		return target, err
	}
	if environment == nil || environment.TaskID != target.TaskID {
		return target, fmt.Errorf("executor environment ownership changed")
	}
	target.OwnershipGeneration = environment.OwnershipGeneration
	if execution.RuntimeName == agentruntime.RuntimeKubernetes {
		reader, ok := m.runningWriter.(interface {
			GetKubernetesEnvironment(context.Context, string) (*models.KubernetesEnvironment, error)
		})
		if !ok {
			return target, fmt.Errorf("kubernetes inventory authority unavailable")
		}
		inventory, err := reader.GetKubernetesEnvironment(ctx, target.EnvironmentID)
		if err != nil {
			return target, err
		}
		if inventory == nil || inventory.TaskID != target.TaskID || inventory.OwnershipGeneration != target.OwnershipGeneration || inventory.OperationID != "" || getMetadataString(inventory.Metadata, MetadataKeyKubernetesPodUID) != target.ResourceKey {
			return target, fmt.Errorf("kubernetes inventory ownership changed")
		}
		target.InventoryRevision = inventory.Revision
		target.Metadata = inventory.Metadata
	}
	// Shared resource authority is independent of the disconnected session.
	target.SessionID = ""
	return target, nil
}

func (m *Manager) classifyExecutorDisconnect(execution *AgentExecution, prompt, startup uint64) {
	if !m.disconnectStillCurrent(execution, prompt, startup) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	target, err := m.executorObservationTarget(ctx, execution)
	if err != nil {
		m.retainUnverifiedExecutorDisconnect(execution, prompt, startup, target, err)
		return
	}
	for attempt := 0; attempt < 2; attempt++ {
		if !m.disconnectStillCurrent(execution, prompt, startup) {
			return
		}
		observation, err := m.InspectExecutor(ctx, target)
		if !m.disconnectStillCurrent(execution, prompt, startup) {
			return
		}
		if observation.ReportedUnavailable() {
			m.storeRemoteStatus(execution.SessionID, &RemoteStatus{RuntimeName: execution.RuntimeName, State: models.ExecutorOutcomeUnknown, LastCheckedAt: observation.ObservedAt, Observation: observation})
		}
		if executorDisconnectObservationReady(observation, err, attempt == 1) {
			if observation.Outcome == models.ExecutorOutcomeHealthy {
				m.reconnectExecutorAfterDisconnect(execution, prompt, startup, &RemoteStatus{RuntimeName: execution.RuntimeName, State: "running", LastCheckedAt: observation.ObservedAt, Observation: observation})
				return
			}
			if err = m.executorObservationHandler(ctx, target, observation); err != nil {
				m.logger.Debug("executor loss admission deferred", zap.String("execution_id", execution.ID), zap.Error(err))
			}
			return
		}
		if attempt == 0 {
			timer := time.NewTimer(15 * time.Second)
			select {
			case <-timer.C:
			case <-m.stopCh:
				timer.Stop()
				return
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}
	m.storeRemoteStatus(execution.SessionID, &RemoteStatus{RuntimeName: execution.RuntimeName, State: models.ExecutorOutcomeUnknown, LastCheckedAt: time.Now().UTC(), ErrorMessage: "executor status unavailable"})
}

func (m *Manager) retainUnverifiedExecutorDisconnect(execution *AgentExecution, prompt, startup uint64, target models.ExecutorObservationTarget, err error) {
	if m.disconnectStillCurrent(execution, prompt, startup) {
		observedAt := time.Now().UTC()
		observation := &models.ExecutorObservation{Runtime: string(execution.RuntimeName), ResourceKey: target.ResourceKey, Outcome: models.ExecutorOutcomeUnknown, ObservedAt: observedAt, Reason: models.ExecutorReasonStatusUnverified, Workspace: models.ExecutorOutcomeUnknown}
		m.logger.Debug("executor disconnect authority unavailable", zap.String("execution_id", execution.ID), zap.Error(err))
		m.reconnectExecutorAfterDisconnect(execution, prompt, startup, &RemoteStatus{RuntimeName: execution.RuntimeName, State: models.ExecutorOutcomeUnknown, LastCheckedAt: observedAt, Observation: observation})
	}
}

// Reconnection and intentional teardown share the instance lifecycle fence.
func (m *Manager) reconnectExecutorAfterDisconnect(execution *AgentExecution, prompt, startup uint64, status *RemoteStatus) {
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if !m.disconnectStillCurrent(execution, prompt, startup) {
		return
	}
	m.storeRemoteStatus(execution.SessionID, status)
	if m.streamManager != nil {
		m.streamManager.ReconnectAll(execution)
	}
}

func (m *Manager) localExecutorObservationTarget(ctx context.Context, execution *AgentExecution, target models.ExecutorObservationTarget) (models.ExecutorObservationTarget, error) {
	reader, ok := m.runningWriter.(interface {
		GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	})
	if !ok {
		return target, fmt.Errorf("local controller identity unavailable")
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	if err != nil {
		return target, err
	}
	if row == nil || row.AgentExecutionID != execution.ID || row.LocalPID <= 0 {
		return target, fmt.Errorf("local controller ownership changed")
	}
	target.LocalPID = row.LocalPID
	target.AuthoritySessionID = execution.SessionID
	target.ExpectedExecutorUpdatedAt = row.UpdatedAt
	target.ResourceKey = fmt.Sprintf("local-pid:%d", row.LocalPID)
	return m.completeExecutorObservationTarget(ctx, execution, target)
}

func (m *Manager) legacyExecutorObservationTarget(ctx context.Context, execution *AgentExecution, target models.ExecutorObservationTarget) (models.ExecutorObservationTarget, error) {
	reader, ok := m.runningWriter.(executorRunningReader)
	if !ok {
		return target, fmt.Errorf("executor source authority unavailable")
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	if err != nil {
		return target, err
	}
	if row == nil || row.TaskID != target.TaskID || row.AgentExecutionID != execution.ID || row.UpdatedAt.IsZero() {
		return target, fmt.Errorf("executor source ownership changed")
	}
	resource := row.ContainerID
	if execution.RuntimeName == agentruntime.RuntimeKubernetes {
		resource = getMetadataString(row.Metadata, MetadataKeyKubernetesPodUID)
	}
	if execution.RuntimeName == agentruntime.RuntimeStandalone {
		resource = fmt.Sprintf("local-pid:%d", row.LocalPID)
	}
	if resource != target.ResourceKey {
		return target, fmt.Errorf("executor resource ownership changed")
	}
	target.ExpectedExecutorUpdatedAt = row.UpdatedAt
	return target, nil
}

// Explicit worker uncertainty is actionable only after the disconnect grace.
func executorDisconnectObservationReady(observation *models.ExecutorObservation, err error, afterGrace bool) bool {
	return err == nil && observation != nil && (observation.Outcome != models.ExecutorOutcomeUnknown || (afterGrace && observation.ReportedUnavailable()))
}
