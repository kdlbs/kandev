package lifecycle

import (
	"context"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/stretchr/testify/require"
	"testing"
)

type retainedWorkspaceWriter struct {
	ExecutorRunningWriter
	retained bool
	err      error
}

func (w retainedWorkspaceWriter) HasRetainedTaskWorkspace(context.Context, string) (bool, error) {
	return w.retained, w.err
}

func TestRetainedWorkspaceBlocksDestructiveRuntimeStop(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "retained"}[retained], func(t *testing.T) {
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			backend := &MockExecutor{name: executor.NameDocker}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			mgr.runningWriter = retainedWorkspaceWriter{retained: retained}
			err := mgr.stopAgentViaBackend(context.Background(), "execution", &AgentExecution{ID: "execution", TaskID: "task", RuntimeName: executor.NameDocker}, StopReasonTaskDeleted, true, false)
			if retained {
				require.Error(t, err)
				require.Empty(t, backend.stopInstanceCalls)
			} else {
				require.NoError(t, err)
				require.Len(t, backend.stopInstanceCalls, 1)
			}
		})
	}
}

func TestRetainedWorkspaceEmptyContainerIsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr := &Manager{runningWriter: retainedWorkspaceWriter{retained: true}}
	require.NoError(t, mgr.DestroyContainer(ctx, ""))
}
