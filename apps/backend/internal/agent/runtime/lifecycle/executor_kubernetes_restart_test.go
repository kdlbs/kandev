package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestKubernetesRefreshRemoteInstanceRehandshakesActiveRestart(t *testing.T) {
	initialControlPort := startKubernetesAgentctlServerWithToken(t, true, 41001, "initial-token")
	initialInstancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): initialControlPort,
		41001:                                    initialInstancePort,
	}}
	executor := newFakeKubernetesExecutorWithForwarder(
		resources, &recordingKubernetesExec{}, forwards,
	)
	req := validKubernetesCreateRequest()
	created, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	initialForward := forwards.lastSession()

	restartedControlPort := startKubernetesAgentctlServerWithToken(t, true, 41002, "rotated-token")
	restartedInstancePort := startKubernetesAgentctlServer(t, false, 0)
	resources.mu.Lock()
	resources.pod.Status.ContainerStatuses[0].RestartCount = 1
	resources.mu.Unlock()
	forwards.mu.Lock()
	forwards.localPorts[uint16(kubeexecutor.DefaultAgentctlPort)] = restartedControlPort
	forwards.localPorts[41002] = restartedInstancePort
	forwards.mu.Unlock()

	refresh, err := executor.RefreshRemoteInstance(context.Background(), &ExecutorInstance{
		InstanceID:     created.InstanceID,
		TaskID:         created.TaskID,
		SessionID:      created.SessionID,
		RuntimeName:    created.RuntimeName,
		Metadata:       created.Metadata,
		AuthToken:      created.AuthToken,
		BootstrapNonce: created.BootstrapNonce,
	})
	require.NoError(t, err)
	require.NotNil(t, refresh)
	require.Equal(t, "rotated-token", refresh.Instance.AuthToken)
	require.Equal(t, "41002", refresh.Instance.Metadata[MetadataKeyKubernetesAgentctlRemotePort])
	require.Equal(t, created.Metadata[MetadataKeyKubernetesPodUID], refresh.Instance.Metadata[MetadataKeyKubernetesPodUID])
	require.False(t, initialForward.isClosed(), "old forward stays live until manager commits")

	require.NoError(t, refresh.Commit(nil))
	require.True(t, initialForward.isClosed())
	require.False(t, forwards.lastSession().isClosed())
	require.Len(t, resources.createdPods, 1)

	again, err := executor.RefreshRemoteInstance(context.Background(), &ExecutorInstance{
		InstanceID:     created.InstanceID,
		TaskID:         created.TaskID,
		SessionID:      created.SessionID,
		RuntimeName:    created.RuntimeName,
		Metadata:       refresh.Instance.Metadata,
		AuthToken:      refresh.Instance.AuthToken,
		BootstrapNonce: refresh.Instance.BootstrapNonce,
	})
	require.NoError(t, err)
	require.Nil(t, again, "recorded restart count prevents duplicate re-handshakes")
}

func TestKubernetesRefreshRemoteInstanceReattachesDeadLocalClientWithoutHandshake(t *testing.T) {
	initialControlPort := startKubernetesAgentctlServerWithToken(t, true, 41001, "initial-token")
	initialInstancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): initialControlPort,
		41001:                                    initialInstancePort,
	}}
	executor := newFakeKubernetesExecutorWithForwarder(
		resources, &recordingKubernetesExec{}, forwards,
	)
	req := validKubernetesCreateRequest()
	created, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	initialRequestCount := len(forwards.remotePorts())

	executor.mu.Lock()
	executor.sessions[created.InstanceID].client = newKubernetesAgentctlClient(
		executor.logger, req, created.InstanceID, created.AuthToken, 1,
	)
	executor.mu.Unlock()
	rotatedControlPort := startKubernetesAgentctlServerWithToken(t, true, 41002, "unexpected-rotated-token")
	rotatedInstancePort := startKubernetesAgentctlServer(t, false, 0)
	forwards.mu.Lock()
	forwards.localPorts[uint16(kubeexecutor.DefaultAgentctlPort)] = rotatedControlPort
	forwards.localPorts[41002] = rotatedInstancePort
	forwards.mu.Unlock()

	refresh, err := executor.RefreshRemoteInstance(context.Background(), &ExecutorInstance{
		InstanceID:     created.InstanceID,
		TaskID:         created.TaskID,
		SessionID:      created.SessionID,
		RuntimeName:    created.RuntimeName,
		Metadata:       created.Metadata,
		AuthToken:      created.AuthToken,
		BootstrapNonce: created.BootstrapNonce,
	})

	require.NoError(t, err)
	require.NotNil(t, refresh)
	require.Equal(t, created.AuthToken, refresh.Instance.AuthToken)
	require.Equal(t, "41001", refresh.Instance.Metadata[MetadataKeyKubernetesAgentctlRemotePort])
	require.Equal(t, []uint16{41001}, forwards.remotePorts()[initialRequestCount:],
		"dead local transport must re-forward the recorded instance port without using the nonce control port")
	require.NoError(t, refresh.Commit(nil))
}

func TestKubernetesRefreshCommitDoesNotFailAfterInstallingReplacement(t *testing.T) {
	initialControlPort := startKubernetesAgentctlServerWithToken(t, true, 41001, "initial-token")
	initialInstancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): initialControlPort,
		41001:                                    initialInstancePort,
	}}
	executor := newFakeKubernetesExecutorWithForwarder(
		resources, &recordingKubernetesExec{}, forwards,
	)
	req := validKubernetesCreateRequest()
	created, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	initialForward := forwards.lastSession()
	initialForward.closeErr = errors.New("old forward close failed")

	restartedControlPort := startKubernetesAgentctlServerWithToken(t, true, 41002, "rotated-token")
	restartedInstancePort := startKubernetesAgentctlServer(t, false, 0)
	resources.mu.Lock()
	resources.pod.Status.ContainerStatuses[0].RestartCount = 1
	resources.mu.Unlock()
	forwards.mu.Lock()
	forwards.localPorts[uint16(kubeexecutor.DefaultAgentctlPort)] = restartedControlPort
	forwards.localPorts[41002] = restartedInstancePort
	forwards.mu.Unlock()

	refresh, err := executor.RefreshRemoteInstance(context.Background(), &ExecutorInstance{
		InstanceID: created.InstanceID, TaskID: created.TaskID, SessionID: created.SessionID,
		RuntimeName: created.RuntimeName, Metadata: created.Metadata,
		AuthToken: created.AuthToken, BootstrapNonce: created.BootstrapNonce,
	})
	require.NoError(t, err)
	require.NotNil(t, refresh)

	require.NoError(t, refresh.Commit(nil), "old-session close is post-commit best effort")
	executor.mu.Lock()
	installed := executor.sessions[created.InstanceID]
	executor.mu.Unlock()
	require.Same(t, refresh.Instance.Client, installed.client)
}

func TestKubernetesPersistedInventorySupportsRestartStatusAndForceCleanup(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	created, err := initial.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, initial.Close())

	stored := cloneKubernetesMetadata(req.Metadata)
	for key, value := range created.Metadata {
		stored[key] = value
	}
	persisted := FilterPersistentMetadata(stored)
	require.NotNil(t, persisted)
	restartedInstance := &ExecutorInstance{
		InstanceID: "new-current-execution", TaskID: req.TaskID, SessionID: req.SessionID,
		Metadata: persisted, StopReason: StopReasonTaskDeleted,
	}
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)

	status, err := restarted.GetRemoteStatus(context.Background(), restartedInstance)

	require.NoError(t, err)
	require.Equal(t, created.Metadata[MetadataKeyKubernetesPodName], status.RemoteName)
	require.Equal(t, "running", status.State)
	require.NoError(t, restarted.StopInstance(context.Background(), restartedInstance, true))
	require.Equal(t, []string{created.Metadata[MetadataKeyKubernetesPodName].(string) + ":pod-uid"}, resources.deletedPods)
	require.Equal(t, []string{created.Metadata[MetadataKeyKubernetesPVCName].(string) + ":pvc-uid"}, resources.deletedPVCs)
}

func TestKubernetesPersistedInventoryRejectsMismatchedInstanceIdentity(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	created, err := initial.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, initial.Close())

	stored := cloneKubernetesMetadata(req.Metadata)
	for key, value := range created.Metadata {
		stored[key] = value
	}
	persisted := FilterPersistentMetadata(stored)
	restartedInstance := &ExecutorInstance{
		InstanceID: "current-execution", TaskID: "other-task", SessionID: req.SessionID,
		Metadata: persisted, StopReason: StopReasonTaskDeleted,
	}
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)

	_, statusErr := restarted.GetRemoteStatus(context.Background(), restartedInstance)
	stopErr := restarted.StopInstance(context.Background(), restartedInstance, true)

	require.ErrorContains(t, statusErr, "does not match")
	require.ErrorContains(t, stopErr, "does not match")
	require.Empty(t, resources.deletedPods)
	require.Empty(t, resources.deletedPVCs)
}

func TestKubernetesLiveExecutionUsesRemoteContainerShellRouting(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	executor := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	instance, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)

	store := NewExecutionStore()
	require.NoError(t, store.Add(instance.ToAgentExecution(req)))
	manager := &Manager{executionStore: store}

	require.True(t, manager.IsRemoteSession(context.Background(), req.SessionID))
	require.True(t, manager.ShouldUseContainerShell(context.Background(), req.SessionID))
}
