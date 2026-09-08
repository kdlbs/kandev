package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// restartSSHInventoryStore is the SSH analogue of restartKubernetesInventoryStore:
// a fake ExecutorRunningWriter that also lists persisted rows, standing in for
// the executors_running table after a backend restart that lost in-memory
// SSHExecutor session state.
type restartSSHInventoryStore struct {
	captureExecutorRunningWriter
	rows []*models.ExecutorRunning
}

func (s *restartSSHInventoryStore) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return s.rows, nil
}

func TestStopAgentWithReasonReapsPersistedSSHAgentctlAfterRestart(t *testing.T) {
	// The remote launch command line is always "<agentctlBin> --workdir
	// <taskDir>" (startRemoteAgentctlOnPort) — sessionDir never appears in
	// the argv. The fake server's ps output mirrors that shape so this test
	// exercises the identity branch that actually fires in production.
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
		sshScriptRule{match: "kill 4242", result: sshOK},
	).handle)
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(NewSSHExecutor(nil, nil, nil, log))

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	row := &models.ExecutorRunning{
		ID: "session-1", SessionID: "session-1", TaskID: "task-1",
		ExecutorID: "executor-1", AgentExecutionID: "execution-1",
		Runtime: agentruntime.RuntimeSSH, Metadata: metadata,
	}
	store := &restartSSHInventoryStore{rows: []*models.ExecutorRunning{row}}

	mgr := NewManager(nil, &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)

	// The execution is not in the in-memory store: this exercises exactly the
	// restart case, where the row survived but SSHExecutor.sessions did not.
	err := mgr.StopAgentWithReason(context.Background(), "execution-1", "startup terminal session cleanup", true)

	require.NoError(t, err)
	if _, ok := server.lastCommandContaining("kill 4242"); !ok {
		t.Fatalf("expected the persisted remote agentctl to be reaped, commands: %v", server.commands())
	}
}

func TestStopAgentWithReasonSkipsNonSSHPersistedRows(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	store := &restartSSHInventoryStore{rows: []*models.ExecutorRunning{{
		AgentExecutionID: "execution-1", Runtime: agentruntime.RuntimeDocker,
	}}}
	mgr := NewManager(nil, &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)

	err := mgr.StopAgentWithReason(context.Background(), "execution-1", "startup terminal session cleanup", true)

	require.ErrorIs(t, err, ErrExecutionNotFound)
}

func TestStopAgentWithReasonNoPersistedRowStillReturnsNotFound(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	store := &restartSSHInventoryStore{}
	mgr := NewManager(nil, &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)

	err := mgr.StopAgentWithReason(context.Background(), "execution-1", "startup terminal session cleanup", true)

	require.ErrorIs(t, err, ErrExecutionNotFound)
}

func TestStopAgentWithReasonPersistedSSHBackendShutdownWithoutForcePreservesRemote(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(NewSSHExecutor(nil, nil, nil, log))

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	row := &models.ExecutorRunning{
		AgentExecutionID: "execution-1", Runtime: agentruntime.RuntimeSSH, Metadata: metadata,
	}
	store := &restartSSHInventoryStore{rows: []*models.ExecutorRunning{row}}
	mgr := NewManager(nil, &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)

	err := mgr.StopAgentWithReason(context.Background(), "execution-1", StopReasonBackendShutdown, false)

	require.NoError(t, err)
	require.Empty(t, server.commands(), "a non-forced graceful shutdown must not touch the remote host")
}

// TestStopAgentWithReasonPropagatesPersistedSSHStopFailure covers F1 at the
// manager level: a failing remote stop command for a restart-recovered SSH
// row must surface as an error from StopAgentWithReason, so the caller
// (startup cleanup) preserves the executors_running row instead of pruning
// it out from under a still-live orphan.
func TestStopAgentWithReasonPropagatesPersistedSSHStopFailure(t *testing.T) {
	server := newFakeSSHServer(t, newSSHScriptedHandler(t,
		sshScriptRule{match: "ps -p 4242 -o command=", result: sshOut("/opt/kandev/bin/agentctl --workdir /remote/task")},
		sshScriptRule{match: "kill 4242", result: sshFail("permission denied")},
	).handle)
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(NewSSHExecutor(nil, nil, nil, log))

	metadata := sshConnectionMetadata(t, server)
	metadata[MetadataKeySSHRemoteAgentctlPID] = "4242"
	metadata[MetadataKeySSHRemoteSessionDir] = "/remote/session"
	metadata[MetadataKeySSHRemoteTaskDir] = "/remote/task"
	row := &models.ExecutorRunning{
		ID: "session-1", SessionID: "session-1", TaskID: "task-1",
		ExecutorID: "executor-1", AgentExecutionID: "execution-1",
		Runtime: agentruntime.RuntimeSSH, Metadata: metadata,
	}
	store := &restartSSHInventoryStore{rows: []*models.ExecutorRunning{row}}

	mgr := NewManager(nil, &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetExecutorRunningWriter(store)

	err := mgr.StopAgentWithReason(context.Background(), "execution-1", "startup terminal session cleanup", true)

	require.Error(t, err)
}
