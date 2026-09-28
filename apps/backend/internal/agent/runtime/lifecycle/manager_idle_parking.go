package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

const StopReasonIdleSuspension = "idle suspension"

var ErrIdleSuspensionRejected = errors.New("idle suspension candidate is no longer eligible")
var ErrIdleSuspensionInProgress = errors.New("idle suspension is in progress")

type idleSuspensionStageError struct {
	stage string
	err   error
}

func (e *idleSuspensionStageError) Error() string                      { return e.err.Error() }
func (e *idleSuspensionStageError) Unwrap() error                      { return e.err }
func (e *idleSuspensionStageError) IdleSuspensionFailureStage() string { return e.stage }

// IdleSuspensionFailureStage returns a bounded lifecycle stage for diagnostics.
func IdleSuspensionFailureStage(err error) (string, bool) {
	var stageErr *idleSuspensionStageError
	if !errors.As(err, &stageErr) {
		return "", false
	}
	return stageErr.stage, true
}

// IdleSuspensionIdentity binds a scan result to the exact session turn and
// Kandev-observed activity that made it eligible.
type IdleSuspensionIdentity struct {
	ExecutionID      string
	SessionID        string
	WorkspaceID      string
	PolicyUpdatedAt  time.Time
	PromptGeneration uint64
	ActivityEpoch    uint64
}

// SuspendIdle stops one settled task-owned ACP process while retaining its
// durable conversation inventory and task-owned workspace resources.
func (m *Manager) SuspendIdle(ctx context.Context, identity IdleSuspensionIdentity) (resultErr error) {
	stage := "identity"
	defer func() {
		if resultErr != nil {
			resultErr = &idleSuspensionStageError{stage: stage, err: resultErr}
		}
	}()
	if identity.ExecutionID == "" || identity.SessionID == "" {
		return fmt.Errorf("idle suspension identity is incomplete")
	}
	stage = "inventory"
	store, ok := m.runningWriter.(idleSuspensionInventory)
	if !ok {
		return fmt.Errorf("idle suspension inventory is unavailable")
	}
	stage = "execution_lookup"
	execution, exists := m.executionStore.Get(identity.ExecutionID)
	if !exists || execution == nil {
		stage = "restart_reconciliation"
		return m.finishPersistedIdleSuspension(ctx, store, identity)
	}
	return m.suspendLiveIdleExecution(ctx, store, execution, identity, &stage)
}

func (m *Manager) suspendLiveIdleExecution(
	ctx context.Context,
	store idleSuspensionInventory,
	execution *AgentExecution,
	identity IdleSuspensionIdentity,
	stage *string,
) error {
	*stage = "execution_lifecycle_lock"
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if current, currentExists := m.executionStore.Get(identity.ExecutionID); !currentExists || current != execution {
		return ErrExecutionNotFound
	}
	running, err := m.claimIdleSuspension(ctx, store, execution, identity, stage)
	if err != nil {
		return err
	}
	return m.stopAndPersistIdleSuspension(ctx, store, execution, running, identity, stage)
}

func (m *Manager) claimIdleSuspension(
	ctx context.Context,
	store idleSuspensionInventory,
	execution *AgentExecution,
	identity IdleSuspensionIdentity,
	stage *string,
) (*models.ExecutorRunning, error) {
	*stage = "execution_admission"
	releaseAdmission, err := execution.acquireContextResetExclusive(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseAdmission()
	*stage, err = validateIdleSuspensionCandidate(execution, identity)
	if err != nil {
		return nil, err
	}
	if m.hasTrackedExecutionActivity(execution.ID) {
		*stage = "active_execution_work"
		return nil, fmt.Errorf("known execution activity is active: %w", ErrIdleSuspensionRejected)
	}
	*stage = "running_inventory"
	running, err := store.GetExecutorRunningBySessionID(ctx, identity.SessionID)
	if err != nil {
		return nil, fmt.Errorf("read idle suspension inventory: %w", err)
	}
	if running == nil || running.AgentExecutionID != execution.ID || running.TaskID != execution.TaskID ||
		!running.Resumable || strings.TrimSpace(running.ResumeToken) == "" ||
		running.IdleSuspensionState == models.ExecutorIdleSuspensionSuspended {
		*stage = "restore_data"
		return nil, fmt.Errorf("idle suspension restore data or identity is unavailable: %w", ErrIdleSuspensionRejected)
	}
	if running.IdleSuspensionState == models.ExecutorIdleSuspensionNone {
		if err := m.claimIdleSuspensionInventory(ctx, store, execution, running, identity, stage); err != nil {
			return nil, err
		}
	} else if running.IdleSuspensionState != models.ExecutorIdleSuspensionInProgress &&
		running.IdleSuspensionState != models.ExecutorIdleSuspensionAgentStopped {
		*stage = "suspension_state"
		return nil, fmt.Errorf("idle suspension state %q is not resumable: %w", running.IdleSuspensionState, ErrIdleSuspensionRejected)
	}
	execution.idleSuspensionInProgress.Store(true)
	return running, nil
}

func (m *Manager) claimIdleSuspensionInventory(
	ctx context.Context,
	store idleSuspensionInventory,
	execution *AgentExecution,
	running *models.ExecutorRunning,
	identity IdleSuspensionIdentity,
	stage *string,
) error {
	*stage = "suspension_claim"
	execution.idleSuspensionInProgress.Store(true)
	claimed, err := store.ClaimExecutorRunningIdleSuspension(
		ctx, identity.SessionID, identity.ExecutionID, running.UpdatedAt,
		identity.WorkspaceID, identity.PolicyUpdatedAt,
	)
	if err != nil {
		execution.idleSuspensionInProgress.Store(false)
		return fmt.Errorf("claim idle suspension: %w", err)
	}
	if !claimed {
		*stage = "claim_conflict"
		execution.idleSuspensionInProgress.Store(false)
		return fmt.Errorf("idle suspension policy or execution changed before claim: %w", ErrIdleSuspensionRejected)
	}
	running.IdleSuspensionState = models.ExecutorIdleSuspensionInProgress
	return nil
}

func (m *Manager) stopAndPersistIdleSuspension(
	ctx context.Context,
	store idleSuspensionInventory,
	execution *AgentExecution,
	running *models.ExecutorRunning,
	identity IdleSuspensionIdentity,
	stage *string,
) error {
	if running.IdleSuspensionState != models.ExecutorIdleSuspensionAgentStopped {
		if err := m.stopIdleACPProcess(ctx, store, execution, running, identity, stage); err != nil {
			return err
		}
	}
	*stage = "runtime_stop"
	if err := m.stopAgentViaBackend(ctx, execution.ID, execution, StopReasonIdleSuspension, false, false); err != nil {
		return fmt.Errorf("stop ACP runtime for idle suspension: %w", err)
	}
	execution.agentctlLifecycleMu.Lock()
	execution.detachAgentctlClient()
	execution.agentctlLifecycleMu.Unlock()
	*stage = "suspension_persistence"
	if err := store.CompareAndSetExecutorRunningIdleSuspension(
		ctx, identity.SessionID, identity.ExecutionID, time.Time{},
		models.ExecutorIdleSuspensionAgentStopped, models.ExecutorIdleSuspensionSuspended,
	); err != nil {
		return fmt.Errorf("persist idle suspension result: %w", err)
	}
	_ = m.executionStore.WithLock(execution.ID, func(exec *AgentExecution) {
		exec.Status = v1.AgentStatusStopped
		now := time.Now().UTC()
		exec.FinishedAt = &now
	})
	execution.EndSessionSpan()
	m.RemoveExecution(execution.ID)
	m.clearRemoteStatus(execution.SessionID)
	m.logger.Info("suspended idle ACP execution",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("task_id", execution.TaskID),
		zap.Stringer("runtime", execution.RuntimeName))
	return nil
}

func (m *Manager) stopIdleACPProcess(
	ctx context.Context,
	store idleSuspensionInventory,
	execution *AgentExecution,
	running *models.ExecutorRunning,
	identity IdleSuspensionIdentity,
	stage *string,
) error {
	if !execution.idleSuspensionAgentStopped.Load() {
		*stage = "agentctl_stop"
		if m.stopExecutionAgentctl(ctx, execution.ID, execution, false) {
			resetErr := store.CompareAndSetExecutorRunningIdleSuspension(
				ctx, identity.SessionID, identity.ExecutionID, time.Time{},
				models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionNone,
			)
			if resetErr == nil {
				execution.idleSuspensionInProgress.Store(false)
				execution.idleSuspensionAgentStopped.Store(false)
			}
			return errors.Join(fmt.Errorf("stop ACP process for idle suspension: agentctl rejected stop"), resetErr)
		}
		execution.idleSuspensionAgentStopped.Store(true)
	}
	if running.IdleSuspensionState != models.ExecutorIdleSuspensionInProgress {
		return nil
	}
	*stage = "agent_stopped_persistence"
	if err := store.CompareAndSetExecutorRunningIdleSuspension(
		ctx, identity.SessionID, identity.ExecutionID, time.Time{},
		models.ExecutorIdleSuspensionInProgress, models.ExecutorIdleSuspensionAgentStopped,
	); err != nil {
		return fmt.Errorf("persist ACP stop for idle suspension: %w", err)
	}
	return nil
}

// CancelIdleSuspension releases a provisional event/prompt fence after the
// durable claim was atomically returned to the unsuspended state.
func (m *Manager) CancelIdleSuspension(ctx context.Context, sessionID, executionID string) error {
	store, ok := m.runningWriter.(idleSuspensionInventory)
	if !ok {
		return fmt.Errorf("idle suspension inventory is unavailable")
	}
	running, err := store.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		return err
	}
	if running.AgentExecutionID != executionID || running.IdleSuspensionState != models.ExecutorIdleSuspensionNone {
		return ErrIdleSuspensionRejected
	}
	execution, exists := m.executionStore.Get(executionID)
	if !exists || execution == nil {
		return nil
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	if current, currentExists := m.executionStore.Get(executionID); !currentExists || current != execution {
		return ErrExecutionNotFound
	}
	execution.idleSuspensionAgentStopped.Store(false)
	execution.idleSuspensionInProgress.Store(false)
	return nil
}

func validateIdleSuspensionCandidate(execution *AgentExecution, identity IdleSuspensionIdentity) (string, error) {
	if err := validateIdleSuspensionIdentity(execution, identity); err != nil {
		return "candidate_identity", err
	}
	if err := validateIdleSuspensionTaskOwner(execution); err != nil {
		return "task_ownership", err
	}
	if stage, err := validateIdleSuspensionRuntime(execution); err != nil {
		return stage, err
	}
	if stage, err := validateIdleSuspensionSettledState(execution, identity); err != nil {
		return stage, err
	}
	return "", nil
}

func validateIdleSuspensionIdentity(execution *AgentExecution, identity IdleSuspensionIdentity) error {
	if execution == nil || execution.ID != identity.ExecutionID || execution.SessionID != identity.SessionID {
		return ErrIdleSuspensionRejected
	}
	return nil
}

func validateIdleSuspensionTaskOwner(execution *AgentExecution) error {
	ownerKind := execution.Owner.Kind
	if ownerKind == "" {
		ownerKind = executionOwnerKind(execution)
	}
	ownerTaskID := execution.Owner.TaskID
	if ownerTaskID == "" {
		ownerTaskID = execution.TaskID
	}
	if execution.TaskID == "" || ownerKind != ExecutionOwnerTask || ownerTaskID != execution.TaskID {
		return ErrIdleSuspensionRejected
	}
	return nil
}

func validateIdleSuspensionRuntime(execution *AgentExecution) (string, error) {
	if execution.IsPassthrough {
		return "passthrough_execution", ErrIdleSuspensionRejected
	}
	if execution.RuntimeName == "" {
		return "runtime_unavailable", ErrIdleSuspensionRejected
	}
	return "", nil
}

func validateIdleSuspensionSettledState(execution *AgentExecution, identity IdleSuspensionIdentity) (string, error) {
	if execution.Status != v1.AgentStatusReady && execution.Status != v1.AgentStatusCompleted {
		return "execution_not_settled", ErrIdleSuspensionRejected
	}
	if execution.dispatchedPromptPending.Load() {
		return "prompt_dispatch_pending", ErrIdleSuspensionRejected
	}
	if execution.activeToolSnapshot() != nil {
		return "active_tool", ErrIdleSuspensionRejected
	}
	if execution.promptGenerationSnapshot() != identity.PromptGeneration {
		return "prompt_generation_changed", ErrIdleSuspensionRejected
	}
	if execution.promptActivityEpochSnapshot() != identity.ActivityEpoch {
		return "prompt_activity_changed", ErrIdleSuspensionRejected
	}
	return "", nil
}

func (m *Manager) finishPersistedIdleSuspension(
	ctx context.Context,
	store idleSuspensionInventory,
	identity IdleSuspensionIdentity,
) error {
	running, err := store.GetExecutorRunningBySessionID(ctx, identity.SessionID)
	if err != nil {
		return fmt.Errorf("read persisted idle suspension: %w", err)
	}
	if running.AgentExecutionID != identity.ExecutionID || running.TaskID == "" ||
		!running.Resumable || strings.TrimSpace(running.ResumeToken) == "" ||
		(running.IdleSuspensionState != models.ExecutorIdleSuspensionInProgress &&
			running.IdleSuspensionState != models.ExecutorIdleSuspensionAgentStopped) {
		return fmt.Errorf("persisted idle suspension identity changed: %w", ErrIdleSuspensionRejected)
	}
	if m.executorRegistry == nil {
		return fmt.Errorf("idle suspension runtime registry is unavailable")
	}
	runtime, err := m.executorRegistry.GetBackend(running.Runtime)
	if err != nil {
		return fmt.Errorf("get persisted idle suspension runtime: %w", err)
	}
	instance := &ExecutorInstance{
		InstanceID:           running.AgentExecutionID,
		TaskID:               running.TaskID,
		SessionID:            running.SessionID,
		RuntimeName:          running.Runtime,
		ContainerID:          running.ContainerID,
		StandaloneInstanceID: running.AgentExecutionID,
		StandalonePort:       running.AgentctlPort,
		WorkspacePath:        running.WorktreePath,
		Metadata:             running.Metadata,
		StopReason:           StopReasonIdleSuspension,
	}
	if instance.RuntimeName == "kubernetes" {
		metadata, resolveErr := m.currentKubernetesConnectionMetadata(ctx, instance.Metadata)
		if resolveErr != nil {
			return fmt.Errorf("resolve persisted idle suspension Kubernetes connection: %w", resolveErr)
		}
		instance.Metadata = metadata
	}
	if err := runtime.StopInstance(ctx, instance, false); err != nil {
		return fmt.Errorf("stop persisted ACP runtime for idle suspension: %w", err)
	}
	if err := store.CompareAndSetExecutorRunningIdleSuspension(
		ctx, identity.SessionID, identity.ExecutionID, time.Time{},
		running.IdleSuspensionState, models.ExecutorIdleSuspensionSuspended,
	); err != nil {
		return fmt.Errorf("persist recovered idle suspension: %w", err)
	}
	return nil
}

func (m *Manager) hasTrackedExecutionActivity(executionID string) bool {
	key := executionActivityKey(executionID)
	m.activityMu.Lock()
	defer m.activityMu.Unlock()
	return m.activityLeases[key] != nil || len(m.activityPending[key]) > 0
}
