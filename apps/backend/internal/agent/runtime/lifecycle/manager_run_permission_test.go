package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunPermission_UnknownExecutionIsNotFound(t *testing.T) {
	manager := newTestManager(t)

	err := manager.ResolveRunPermission(t.Context(), "missing", "req", "pending", "opt")
	require.ErrorIs(t, err, ErrExecutionNotFound)
	err = manager.CancelRunPermission(t.Context(), "missing", "req", "pending")
	require.ErrorIs(t, err, ErrExecutionNotFound)
}

// Task-owned permissions are decided by the orchestrator through the task
// session; the run-keyed entry point must not answer them.
func TestRunPermission_RejectsTaskOwnedExecution(t *testing.T) {
	manager := newTestManager(t)
	execution := createTestExecution("exec-task", "task-1", "session-1")
	require.NoError(t, manager.executionStore.Add(execution))

	err := manager.ResolveRunPermission(t.Context(), "exec-task", "req", "pending", "opt")
	require.ErrorIs(t, err, ErrNotRunOwnedExecution)
	err = manager.CancelRunPermission(t.Context(), "exec-task", "req", "pending")
	require.ErrorIs(t, err, ErrNotRunOwnedExecution)
}
