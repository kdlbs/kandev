package lifecycle

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"time"
)

// InspectExecutor reads recorded resource status without refreshing control,
// reconnecting agent streams, creating resources, or loading provider conversations.
func (m *Manager) InspectExecutor(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
	now := time.Now().UTC()
	unknown := &models.ExecutorObservation{Outcome: models.ExecutorOutcomeUnknown, Runtime: target.Runtime, ResourceKey: target.ResourceKey, ObservedAt: now, Workspace: models.ExecutorOutcomeUnknown}
	if target.Runtime == string(agentruntime.RuntimeStandalone) && m.localExecutorInspector != nil {
		if observation := m.localExecutorInspector(target); observation != nil {
			return observation.Clone(), nil
		}
		return unknown, nil
	}
	if m.executorRegistry == nil {
		return unknown, nil
	}
	backend, err := m.executorRegistry.GetBackend(agentruntime.Runtime(target.Runtime))
	if err != nil {
		return unknown, routingerr.SanitizeError(err)
	}
	provider, ok := backend.(RemoteStatusProvider)
	if !ok {
		return unknown, nil
	}
	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	instanceID := target.ExecutionID
	if instanceID == "" {
		instanceID = target.EnvironmentID
	}
	status, err := provider.GetRemoteStatus(inspectCtx, &ExecutorInstance{InstanceID: instanceID, TaskID: target.TaskID, SessionID: target.SessionID, RuntimeName: agentruntime.Runtime(target.Runtime), ContainerID: target.ContainerID, Metadata: target.Metadata})
	if err != nil {
		return unknown, routingerr.SanitizeError(err)
	}
	if status == nil || status.Observation == nil {
		return unknown, nil
	}
	observation := status.Observation.Clone()
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = now
	}
	return observation, nil
}

// RetireExecutorLoss releases only the observed dead execution. Resource and
// provider inventory remain owned by their existing recovery/cleanup paths.
func (m *Manager) RetireExecutorLoss(ctx context.Context, target models.ExecutorObservationTarget, observation *models.ExecutorObservation) (bool, error) {
	if !observation.ConfirmedLoss() || observation.ResourceKey != target.ResourceKey {
		return false, nil
	}
	unlock := m.taskRuntimeFences.acquireSweep(target.TaskID)
	defer unlock()
	execution, ok := m.executionStore.GetBySessionID(target.SessionID)
	if !ok {
		return true, nil
	}
	if execution.ID != target.ExecutionID || execution.TaskID != target.TaskID || execution.TaskEnvironmentID != target.EnvironmentID {
		return false, nil
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	current, err := m.executorLossAuthorityCurrent(ctx, target)
	if err != nil || !current {
		return false, err
	}

	resourceKey := execution.ContainerID
	if execution.RuntimeName == agentruntime.RuntimeStandalone {
		resourceKey = fmt.Sprintf("local-pid:%d", m.resolveLocalPID(execution))
	}
	if execution.RuntimeName == agentruntime.RuntimeKubernetes {
		resourceKey = getMetadataString(execution.MetadataSnapshot(), MetadataKeyKubernetesPodUID)
	}
	if resourceKey != target.ResourceKey {
		return false, nil
	}
	startup := execution.startupAttemptSnapshot()
	prompt := execution.promptGenerationSnapshot()
	accepted := execution.withStartupAttempt(startup, func(_ string) {
		execution.signalPromptCompletionForStartupGenerationLeased(startup, PromptCompletionSignal{IsError: true, ExecutorInterrupted: true, PromptGeneration: prompt})
	})
	if !accepted {
		return false, nil
	}
	m.flushMessageBuffer(execution, prompt, "")
	m.flushAssistantHistory(execution)
	m.RemoveExecution(execution.ID)
	m.clearRemoteStatus(target.SessionID)
	return true, nil
}

// SetLocalExecutorInspector installs the existing controller exit source. It
// never probes a PID belonging to a remote runtime or infers loss from restart.
func (m *Manager) SetLocalExecutorInspector(inspect func(models.ExecutorObservationTarget) *models.ExecutorObservation) {
	m.localExecutorInspector = inspect
}

func (m *Manager) executorLossAuthorityCurrent(ctx context.Context, target models.ExecutorObservationTarget) (bool, error) {
	if target.ExpectedExecutorUpdatedAt.IsZero() {
		return true, nil
	}
	reader, ok := m.runningWriter.(interface {
		GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	})
	if !ok {
		return false, nil
	}
	row, err := reader.GetExecutorRunningBySessionID(ctx, target.SessionID)
	if err != nil {
		return false, err
	}
	if row == nil || row.AgentExecutionID != target.ExecutionID || !row.UpdatedAt.Equal(target.ExpectedExecutorUpdatedAt) {
		return false, nil
	}
	return true, nil
}

// Inspect recorded compute before duplicate cleanup can obscure its cause. This
// guard does not retire or recreate anything while launch owns the runtime fence.
func (m *Manager) existingExecutorLaunchError(ctx context.Context, execution *AgentExecution) error {
	duplicate := fmt.Errorf("%w: session %q (execution: %s)", ErrAgentAlreadyRunning, execution.SessionID, execution.ID)
	if !m.supportsExecutorInspection(execution.RuntimeName) {
		return duplicate
	}
	target, err := m.executorObservationTarget(ctx, execution)
	if err != nil {
		return &ExecutorUnavailableError{Observation: &models.ExecutorObservation{Runtime: string(execution.RuntimeName), ResourceKey: target.ResourceKey, Outcome: models.ExecutorOutcomeUnknown, ObservedAt: time.Now().UTC(), Reason: models.ExecutorReasonStatusUnverified, Workspace: models.ExecutorOutcomeUnknown}}
	}
	observation, err := m.InspectExecutor(ctx, target)
	if observation == nil || observation.Outcome == models.ExecutorOutcomeHealthy {
		return duplicate
	}
	if err != nil || (observation.Outcome == models.ExecutorOutcomeUnknown && !observation.ReportedUnavailable()) {
		observation.Reason = models.ExecutorReasonStatusUnverified
		observation.Message = ""
		return &ExecutorUnavailableError{Observation: observation}
	}
	if !observation.ConfirmedLoss() && !observation.ReportedUnavailable() {
		return duplicate
	}
	if m.executorObservationHandler != nil && m.streamManager != nil && !m.IsShuttingDown() {
		m.streamManager.start(func() {
			recordCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = m.executorObservationHandler(recordCtx, target, observation)
		})
	}
	return &ExecutorUnavailableError{Observation: observation}
}

func (m *Manager) supportsExecutorInspection(runtime agentruntime.Runtime) bool {
	if runtime == agentruntime.RuntimeStandalone {
		return m.localExecutorInspector != nil
	}
	if runtime != agentruntime.RuntimeDocker && runtime != agentruntime.RuntimeRemoteDocker && runtime != agentruntime.RuntimeKubernetes {
		return false
	}
	if m.executorRegistry == nil {
		return false
	}
	backend, err := m.executorRegistry.GetBackend(runtime)
	if err != nil {
		return false
	}
	_, ok := backend.(RemoteStatusProvider)
	return ok
}
