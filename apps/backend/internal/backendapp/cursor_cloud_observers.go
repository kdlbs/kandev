package backendapp

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
)

// StartManagedRuntimeObservers scans only Kandev's durable bindings. It does
// not enumerate the user's Cursor account or create replacement operations.
func (m *cursorCloudAgentManager) StartManagedRuntimeObservers(ctx context.Context) error {
	m.observerMu.Lock()
	if m.observerCtx != nil {
		m.observerMu.Unlock()
		return nil
	}
	observerCtx, cancel := context.WithCancel(ctx)
	m.observerCtx = observerCtx
	m.observerCancel = cancel
	m.observerCancels = make(map[string]context.CancelFunc)
	m.observerStopping = false
	m.observerMu.Unlock()

	bindings, err := m.repo.ListActiveManagedAgentBindings(ctx)
	if err != nil {
		m.StopManagedRuntimeObservers()
		return err
	}
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		if err := m.recoverManagedBinding(ctx, binding); err != nil {
			m.logger.Warn("Cursor Cloud startup recovery deferred",
				zap.String("task_id", binding.TaskID),
				zap.String("session_id", binding.SessionID),
				zap.String("recovery_class", "retryable"))
		}
	}
	return nil
}

func (m *cursorCloudAgentManager) recoverManagedBinding(ctx context.Context, binding *models.ManagedAgentBinding) error {
	operation, err := m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return err
	}
	if operation.CompletionPending {
		m.startManagedObserver(binding.ExecutionID, "backend_restart")
		return nil
	}
	if operation.State == models.ManagedAgentSubmissionReserved && !m.enabled() {
		return nil
	}
	operation, err = m.recoverCreateSubmission(ctx, binding, operation)
	if err != nil {
		return err
	}
	operation, err = m.recoverFollowupSubmission(ctx, binding, operation)
	if err != nil {
		return err
	}
	operation, err = m.recoverCancellingSubmission(ctx, binding, operation)
	if err != nil {
		return err
	}
	if models.ManagedAgentOperationActive(operation.State) && operation.RemoteRunID != "" {
		m.startManagedObserver(binding.ExecutionID, "backend_restart")
	}
	return nil
}

func (m *cursorCloudAgentManager) recoverCreateSubmission(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentOperation, error) {
	if operation.Kind != models.ManagedAgentOperationCreate || operation.RemoteRunID != "" ||
		(operation.State != models.ManagedAgentSubmissionReserved &&
			operation.State != models.ManagedAgentSubmissionSubmitting &&
			operation.State != models.ManagedAgentSubmissionUnknown) {
		return operation, nil
	}
	err := m.runtime.StartExecution(ctx, binding.ExecutionID)
	if err != nil && !errors.Is(err, cursorcloud.ErrCancellationPending) {
		return nil, err
	}
	return m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
}

func (m *cursorCloudAgentManager) recoverFollowupSubmission(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentOperation, error) {
	if operation.Kind != models.ManagedAgentOperationFollowup {
		return operation, nil
	}
	if operation.State == models.ManagedAgentSubmissionReserved && m.enabled() {
		if err := m.managedRuntime.ResumeReservedFollowup(ctx, binding.ExecutionID); err != nil {
			return nil, err
		}
		return m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	}
	if operation.State == models.ManagedAgentSubmissionSubmitting && operation.RemoteRunID == "" {
		if err := m.managedRuntime.MarkFollowupSubmissionUnknownAfterRestart(ctx, binding.ExecutionID); err != nil {
			return nil, err
		}
		return m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	}
	return operation, nil
}

func (m *cursorCloudAgentManager) recoverCancellingSubmission(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
) (*models.ManagedAgentOperation, error) {
	if operation.State != models.ManagedAgentSubmissionCancelling {
		return operation, nil
	}
	_ = m.managedRuntime.Stop(ctx, binding.ExecutionID, "backend_restart")
	return m.repo.GetManagedAgentLatestOperation(ctx, binding.ID)
}

// StopManagedRuntimeObservers detaches local observers without cancelling the
// remote work. A later backend start resumes from the durable SSE checkpoint.
func (m *cursorCloudAgentManager) StopManagedRuntimeObservers() {
	m.observerMu.Lock()
	m.observerStopping = true
	if m.observerCancel != nil {
		m.observerCancel()
	}
	for _, cancel := range m.observerCancels {
		cancel()
	}
	m.observerMu.Unlock()
	m.observerWG.Wait()

	m.observerMu.Lock()
	m.observerCtx = nil
	m.observerCancel = nil
	m.observerCancels = nil
	m.observerMu.Unlock()
}

func (m *cursorCloudAgentManager) startManagedObserver(executionID, reason string) {
	m.observerMu.Lock()
	if m.observerStopping || m.observerCtx == nil || m.observerCancels[executionID] != nil {
		m.observerMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.observerCtx)
	m.observerCancels[executionID] = cancel
	m.observerWG.Add(1)
	m.observerMu.Unlock()

	go func() {
		defer m.observerWG.Done()
		m.managedRuntime.ObserveContinuously(ctx, executionID, reason)
		m.observerMu.Lock()
		delete(m.observerCancels, executionID)
		m.observerMu.Unlock()
	}()
}

// ManagedRuntimeObserverLifecycle is asserted by orchestrator.Service without
// widening the existing AgentManagerClient contract.
type managedRuntimeObserverLifecycle interface {
	StartManagedRuntimeObservers(context.Context) error
	StopManagedRuntimeObservers()
}

var _ managedRuntimeObserverLifecycle = (*cursorCloudAgentManager)(nil)
