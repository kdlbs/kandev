package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
)

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerPostAttachmentLaunchFailuresPreserveReusedRuntime(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason string
		run    func(*Manager, ExecutorBackend, *ExecutorInstance, *AgentExecution, string)
	}{
		{
			name:   "repository projection failure before execution build",
			reason: "reconstruct remote workspace repositories",
			run: func(m *Manager, backend ExecutorBackend, instance *ExecutorInstance, _ *AgentExecution, _ string) {
				_ = m.stopLaunchRuntimeOnRollback(context.Background(), backend, instance, nil)
			},
		},
		{
			name:   "command resolution failure before execution build",
			reason: "build execution from runtime instance",
			run: func(m *Manager, backend ExecutorBackend, instance *ExecutorInstance, _ *AgentExecution, _ string) {
				_ = m.stopLaunchRuntimeOnRollback(context.Background(), backend, instance, nil)
			},
		},
		{
			name:   "project MCP materialization failure",
			reason: "project MCP materialization failed",
			run: func(m *Manager, backend ExecutorBackend, instance *ExecutorInstance, execution *AgentExecution, reason string) {
				m.rollbackLaunchExecution(context.Background(), backend, instance, execution, reason)
			},
		},
		{
			name:   "owner admission failure",
			reason: "owner admission failed",
			run: func(m *Manager, backend ExecutorBackend, instance *ExecutorInstance, execution *AgentExecution, reason string) {
				m.rollbackLaunchExecution(context.Background(), backend, instance, execution, reason)
			},
		},
		{
			name:   "duplicate execution race",
			reason: "session conflict",
			run: func(m *Manager, backend ExecutorBackend, instance *ExecutorInstance, execution *AgentExecution, _ string) {
				m.rollbackRacedExecution(context.Background(), backend, instance, execution)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, server := newRecordingDockerEngine(t)
			backend := newRemoteDockerTestExecutor(t, server)
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			instance := &ExecutorInstance{
				InstanceID:      "instance-1",
				TaskID:          "task-1",
				ContainerID:     "container-1",
				RuntimeName:     agentruntime.RuntimeRemoteDocker,
				ReusedWorkspace: true,
			}
			execution := &AgentExecution{
				ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
				RuntimeName:             executor.NameRemoteDocker,
				attachedExistingCompute: true,
			}
			tc.run(mgr, backend, instance, execution, tc.reason)
			require.Empty(t, engine.snapshot(), "a retained container must receive no destroy request")
		})
	}
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestKubernetesPreExecutionRollbackUsesReuseProvenance(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reused    bool
		wantForce bool
	}{
		{"existing pod", true, false},
		{"fresh pod", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			instance := &ExecutorInstance{
				InstanceID:      "instance-1",
				TaskID:          "task-1",
				RuntimeName:     agentruntime.RuntimeKubernetes,
				ReusedWorkspace: tc.reused,
			}
			require.NoError(t, mgr.stopLaunchRuntimeOnRollback(context.Background(), backend, instance, nil))
			require.Equal(t, []bool{tc.wantForce}, backend.forces)
		})
	}
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerFreshLaunchRollbackRemainsDestructive(t *testing.T) {
	engine, server := newRecordingDockerEngine(t)
	backend := newRemoteDockerTestExecutor(t, server)
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	instance := &ExecutorInstance{
		InstanceID:  "instance-1",
		TaskID:      "task-1",
		ContainerID: "container-1",
		RuntimeName: agentruntime.RuntimeRemoteDocker,
	}
	require.NoError(t, mgr.stopLaunchRuntimeOnRollback(context.Background(), backend, instance, nil))
	requests := engine.snapshot()
	require.True(t, containsDockerRequest(requests, "POST", "/kill"), "requests = %v", requests)
	require.True(t, containsDockerRequest(requests, "DELETE", "/containers/container-1"), "requests = %v", requests)
}
