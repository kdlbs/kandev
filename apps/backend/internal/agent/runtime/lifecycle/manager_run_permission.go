package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// ErrNotRunOwnedExecution rejects a run-keyed permission call for an
// execution that a task session owns.
var ErrNotRunOwnedExecution = errors.New("execution is not run-owned")

const runPermissionRequestTimeout = 30 * time.Second

// ResolveRunPermission selects one exact option on a pending request of a
// run-owned execution. Run-owned executions have no task session, so the
// session-keyed permission calls cannot reach them.
func (m *Manager) ResolveRunPermission(ctx context.Context, executionID, requestID, pendingID, optionID string) error {
	return m.withRunPermissionClient(ctx, executionID, func(ctx context.Context, client *agentctl.Client) error {
		_, err := client.ResolvePermission(ctx, requestID, pendingID, optionID)
		return err
	})
}

// CancelRunPermission cancels a pending request of a run-owned execution.
func (m *Manager) CancelRunPermission(ctx context.Context, executionID, requestID, pendingID string) error {
	return m.withRunPermissionClient(ctx, executionID, func(ctx context.Context, client *agentctl.Client) error {
		_, err := client.CancelPermission(ctx, requestID, pendingID)
		return err
	})
}

func (m *Manager) withRunPermissionClient(
	ctx context.Context, executionID string, call func(context.Context, *agentctl.Client) error,
) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("%w: %s", ErrExecutionNotFound, executionID)
	}
	if executionOwnerKind(execution) != ExecutionOwnerRun {
		return fmt.Errorf("%w: %s", ErrNotRunOwnedExecution, executionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("agent execution has no agentctl client: %s", executionID)
	}
	requestCtx, cancel := context.WithTimeout(ctx, runPermissionRequestTimeout)
	defer cancel()
	return call(requestCtx, client)
}
