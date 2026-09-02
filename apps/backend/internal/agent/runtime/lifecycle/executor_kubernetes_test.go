package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type kubernetesAmbiguousCreateControl struct {
	createErr error
	info      *agentctl.InstanceInfo
	getErr    error
	getCalls  int
}

func (c *kubernetesAmbiguousCreateControl) CreateInstance(
	context.Context,
	*agentctl.CreateInstanceRequest,
) (*agentctl.CreateInstanceResponse, error) {
	return nil, c.createErr
}

func (c *kubernetesAmbiguousCreateControl) GetInstance(
	ctx context.Context,
	_ string,
) (*agentctl.InstanceInfo, error) {
	c.getCalls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.info, c.getErr
}

func TestKubernetesCreateInstanceAcceptsCompleteTypedConfiguration(t *testing.T) {
	executor := NewKubernetesExecutor(nil, newTestLogger())
	_, err := executor.CreateInstance(context.Background(), validKubernetesCreateRequest())
	if errors.Is(err, errKubernetesLifecycleRequestIncomplete) {
		t.Fatalf("CreateInstance() error = %v, complete typed configuration was not consumed", err)
	}
}

func TestKubernetesAgentctlCreateReconcilesAmbiguousResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	control := &kubernetesAmbiguousCreateControl{
		createErr: context.Canceled,
		info: &agentctl.InstanceInfo{
			ID: "instance-1", Port: 41001, WorkspacePath: dockerWorkspacePath,
		},
	}
	request := &agentctl.CreateInstanceRequest{ID: "instance-1", WorkspacePath: dockerWorkspacePath}

	response, err := createOrReconcileKubernetesAgentctlInstance(ctx, control, request)

	require.NoError(t, err)
	require.Equal(t, &agentctl.CreateInstanceResponse{ID: "instance-1", Port: 41001}, response)
	require.Equal(t, 1, control.getCalls)
}

func TestKubernetesAgentctlCreateRejectsMismatchedReconciliation(t *testing.T) {
	control := &kubernetesAmbiguousCreateControl{
		createErr: context.DeadlineExceeded,
		info: &agentctl.InstanceInfo{
			ID: "instance-1", Port: 41001, WorkspacePath: "/foreign",
		},
	}
	request := &agentctl.CreateInstanceRequest{ID: "instance-1", WorkspacePath: dockerWorkspacePath}

	_, err := createOrReconcileKubernetesAgentctlInstance(context.Background(), control, request)

	require.ErrorContains(t, err, "workspace")
}

func TestKubernetesInstanceLockSerializesSameInstanceAndCleansUp(t *testing.T) {
	executor := NewKubernetesExecutor(nil, newTestLogger())
	firstUnlock := executor.lockInstance("instance-1")
	secondAcquired := make(chan struct{})
	secondReleased := make(chan struct{})
	go func() {
		secondUnlock := executor.lockInstance("instance-1")
		close(secondAcquired)
		secondUnlock()
		close(secondReleased)
	}()

	select {
	case <-secondAcquired:
		t.Fatal("same-instance operation was not serialized")
	default:
	}
	firstUnlock()
	select {
	case <-secondReleased:
	case <-time.After(time.Second):
		t.Fatal("same-instance waiter did not acquire the released lock")
	}

	executor.mu.Lock()
	defer executor.mu.Unlock()
	require.Empty(t, executor.locks)
}

func TestKubernetesCreateInstanceProvisionsBootstrapsAndForwardsAgentctl(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	execs := &recordingKubernetesExec{}
	forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	}}
	executor := NewKubernetesExecutor(nil, newTestLogger())
	executor.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		return &kubernetesRuntimeClient{
			resources: resources,
			streams:   kubeexecutor.NewStreamOperations(execs, forwards),
		}, nil
	}
	executor.resolveBinary = func(kubeexecutor.Platform) ([]byte, error) {
		return []byte("agentctl-binary"), nil
	}
	req := validKubernetesCreateRequest()
	req.Metadata["workspace.mode"] = "managed_pvc"
	req.Metadata["workspace.size"] = "1Gi"
	req.Metadata["workspace.access_modes"] = `["ReadWriteOnce"]`

	instance, err := executor.CreateInstance(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, "pod-uid", instance.Metadata[MetadataKeyKubernetesPodUID])
	require.Equal(t, "pvc-uid", instance.Metadata[MetadataKeyKubernetesPVCUID])
	require.Equal(t, true, instance.Metadata[MetadataKeyKubernetesPVCCreated])
	require.Equal(t, "41001", instance.Metadata[MetadataKeyKubernetesAgentctlRemotePort])
	require.Equal(t, "/workspace", instance.WorkspacePath)
	require.Equal(t, true, instance.Metadata[MetadataKeyIsRemote])
	require.Equal(t, "handshake-token", instance.AuthToken)
	require.NotEmpty(t, instance.BootstrapNonce)
	require.NotNil(t, instance.Client)
	require.Len(t, resources.createdPVCs, 1)
	require.Len(t, resources.createdPods, 1)
	require.Len(t, execs.requests, 4, "binary, runtime config, auth config, then start signal")
	require.Equal(t, []uint16{uint16(kubeexecutor.DefaultAgentctlPort), 41001}, forwards.remotePorts())
	for _, request := range forwards.requests {
		require.Equal(t, "127.0.0.1", request.LocalAddress)
	}
}

func TestKubernetesCreateInstanceReconnectsExactPodWithFreshForward(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initialExecs := &recordingKubernetesExec{}
	initial := newFakeKubernetesExecutor(t, resources, initialExecs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)

	reconnectExecs := &recordingKubernetesExec{}
	reconnectForwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{41001: instancePort}}
	restartedBackend := NewKubernetesExecutor(nil, newTestLogger())
	restartedBackend.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		return &kubernetesRuntimeClient{
			resources: resources,
			streams:   kubeexecutor.NewStreamOperations(reconnectExecs, reconnectForwards),
		}, nil
	}
	restartedBackend.resolveBinary = func(kubeexecutor.Platform) ([]byte, error) { return []byte("unused"), nil }
	reconnectRequest := validKubernetesCreateRequest()
	reconnectRequest.InstanceID = "new-execution-id"
	reconnectRequest.PreviousExecutionID = created.InstanceID
	reconnectRequest.AuthToken = created.AuthToken
	reconnectRequest.BootstrapNonce = created.BootstrapNonce
	for key, value := range created.Metadata {
		reconnectRequest.Metadata[key] = value
	}

	reconnected, err := restartedBackend.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, "new-execution-id", reconnected.InstanceID)
	require.Equal(t, created.AuthToken, reconnected.AuthToken)
	require.Equal(t, created.Metadata[MetadataKeyKubernetesPodUID], reconnected.Metadata[MetadataKeyKubernetesPodUID])
	require.Equal(t, created.InstanceID, reconnected.Metadata[MetadataKeyKubernetesAgentctlInstanceID])
	require.Empty(t, reconnectExecs.requests)
	require.Len(t, resources.createdPods, 1)
	require.Len(t, resources.createdPVCs, 0)
	require.Equal(t, []uint16{41001}, reconnectForwards.remotePorts())
}

func TestKubernetesCreateInstanceRehandshakesAfterMainContainerRestart(t *testing.T) {
	initialControlPort := startKubernetesAgentctlServerWithToken(t, true, 41001, "initial-token")
	initialInstancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): initialControlPort,
		41001:                                    initialInstancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)
	require.Equal(t, "initial-token", created.AuthToken)

	staleInstancePort := startKubernetesUnhealthyServer(t)
	restartedControlPort := startKubernetesAgentctlServerWithToken(t, true, 41002, "rotated-token")
	restartedInstancePort := startKubernetesAgentctlServer(t, false, 0)
	forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
		41001:                                    staleInstancePort,
		uint16(kubeexecutor.DefaultAgentctlPort): restartedControlPort,
		41002:                                    restartedInstancePort,
	}}
	execs := &recordingKubernetesExec{}
	restarted := newFakeKubernetesExecutorWithForwarder(resources, execs, forwards)

	reconnected, err := restarted.CreateInstance(context.Background(), kubernetesReconnectRequest(created))

	require.NoError(t, err)
	require.Equal(t, "rotated-token", reconnected.AuthToken)
	require.Equal(t, "41002", reconnected.Metadata[MetadataKeyKubernetesAgentctlRemotePort])
	require.Equal(t, created.InstanceID, reconnected.Metadata[MetadataKeyKubernetesAgentctlInstanceID])
	require.Equal(t, []uint16{41001, uint16(kubeexecutor.DefaultAgentctlPort), 41002}, forwards.remotePorts())
	require.True(t, forwards.sessions[0].isClosed(), "the stale instance forward must close")
	require.True(t, forwards.sessions[1].isClosed(), "the temporary control forward must close")
	require.False(t, forwards.sessions[2].isClosed(), "the rotated instance forward must remain live")
	require.Empty(t, execs.requests, "a container restart reuses Pod-scoped bootstrap files")
}

func TestKubernetesCreateInstanceUsesCurrentConnectionConfigOnReconnect(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)

	forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{41001: instancePort}}
	restarted := newFakeKubernetesExecutorWithForwarder(resources, &recordingKubernetesExec{}, forwards)
	reconnectRequest := kubernetesReconnectRequest(created)
	reconnectRequest.Metadata[MetadataKeyKubernetesAuthMode] = "kubeconfig"
	reconnectRequest.Metadata[MetadataKeyKubernetesKubeconfigPath] = "/etc/kandev/current-kubeconfig"
	reconnectRequest.Metadata[MetadataKeyKubernetesKubeContext] = "current-context"
	reconnectRequest.Metadata[MetadataKeyKubernetesRequestTimeoutSeconds] = "45"

	reconnected, err := restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, "/etc/kandev/current-kubeconfig", reconnected.Metadata[MetadataKeyKubernetesKubeconfigPath])
	require.Equal(t, "current-context", reconnected.Metadata[MetadataKeyKubernetesKubeContext])
	require.Equal(t, "45", reconnected.Metadata[MetadataKeyKubernetesRequestTimeoutSeconds])
	require.NotEqual(t, created.Metadata[MetadataKeyKubernetesExecutorConfigHash], reconnected.Metadata[MetadataKeyKubernetesExecutorConfigHash])
	require.NotEmpty(t, resources.getPodRequests)
	require.NotEmpty(t, forwards.requests)
}

func TestKubernetesCreateInstanceUsesRecordedNamespaceAfterConnectionNamespaceChanges(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{41001: instancePort})
	reconnectRequest := kubernetesReconnectRequest(created)
	reconnectRequest.Metadata[MetadataKeyKubernetesConfigNamespace] = "other-agents"

	reconnected, err := restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, "other-agents", reconnected.Metadata[MetadataKeyKubernetesConfigNamespace])
	require.Equal(t, "kandev-agents", reconnected.Metadata[MetadataKeyKubernetesNamespace])
	require.Contains(t, resources.getPodRequests, "kandev-agents/"+created.Metadata[MetadataKeyKubernetesPodName].(string))
}

func TestKubernetesCreateInstanceRejectsIncompleteRecordedResourceIdentity(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)

	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{41001: instancePort})
	reconnectRequest := kubernetesReconnectRequest(created)
	delete(reconnectRequest.Metadata, MetadataKeyKubernetesResourceInstanceID)

	_, err = restarted.CreateInstance(context.Background(), reconnectRequest)

	require.ErrorContains(t, err, "resource identity")
}

func TestKubernetesCreateInstanceRejectsMismatchedRecordedAgentctlInstance(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)

	reconnectRequest := kubernetesReconnectRequest(created)
	reconnectRequest.Metadata[MetadataKeyKubernetesAgentctlInstanceID] = "redirected-agentctl-instance"
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	resources.mu.Lock()
	requestsBefore := len(resources.getPodRequests)
	resources.mu.Unlock()

	_, err = restarted.CreateInstance(context.Background(), reconnectRequest)

	require.ErrorContains(t, err, "agentctl instance")
	resources.mu.Lock()
	require.Len(t, resources.getPodRequests, requestsBefore, "identity mismatch must fail before Pod access")
	resources.mu.Unlock()
}

func TestKubernetesRecordedResourceIdentityRejectsRequestMismatch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExecutorCreateRequest)
	}{
		{name: "task", mutate: func(req *ExecutorCreateRequest) { req.TaskID = "other-task" }},
		{name: "session", mutate: func(req *ExecutorCreateRequest) { req.SessionID = "other-session" }},
		{name: "executor", mutate: func(req *ExecutorCreateRequest) { req.Metadata["executor_id"] = "other-executor" }},
		{name: "profile", mutate: func(req *ExecutorCreateRequest) {
			req.Metadata[MetadataKeyExecutorProfileID] = "other-profile"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := validKubernetesCreateRequest()
			req.Metadata[MetadataKeyKubernetesResourceExecutorID] = "executor-1"
			req.Metadata[MetadataKeyKubernetesResourceProfileID] = "profile-1"
			req.Metadata[MetadataKeyKubernetesResourceInstanceID] = "resource-instance-1"
			req.Metadata[MetadataKeyKubernetesResourceTaskID] = "task-1"
			req.Metadata[MetadataKeyKubernetesResourceSessionID] = "session-1"
			req.Metadata[MetadataKeyKubernetesResourceEnvironmentID] = "environment-1"
			test.mutate(req)

			_, err := kubernetesRecordedResourceIdentity(req, true)

			require.ErrorContains(t, err, "does not match")
		})
	}
}

func TestKubernetesCreateInstanceRejectsLostEmptyDirPod(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod = nil
	resources.mu.Unlock()

	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	req := kubernetesReconnectRequest(created)
	_, err = restarted.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "emptyDir workspace is unrecoverable")
	require.Len(t, resources.createdPods, 1)
}

func TestKubernetesCreateInstanceRebuildsLostPodAgainstVerifiedManagedPVC(t *testing.T) {
	initialControlPort := startKubernetesAgentctlServer(t, true, 41001)
	initialInstancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): initialControlPort,
		41001:                                    initialInstancePort,
	})
	initialRequest := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(initialRequest)
	created, err := initial.CreateInstance(context.Background(), initialRequest)
	require.NoError(t, err)
	created.Metadata[MetadataKeyKubernetesContainerRestartCount] = "7"
	resources.mu.Lock()
	resources.pod = nil
	resources.nextPodUID = "replacement-pod-uid"
	resources.mu.Unlock()

	restartControlPort := startKubernetesAgentctlServer(t, true, 41002)
	restartInstancePort := startKubernetesAgentctlServer(t, false, 0)
	reconnectExecs := &recordingKubernetesExec{}
	restarted := newFakeKubernetesExecutor(t, resources, reconnectExecs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): restartControlPort,
		41002:                                    restartInstancePort,
	})
	reconnectRequest := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(reconnectRequest)
	reconnectRequest.InstanceID = "new-execution-id"
	reconnectRequest.PreviousExecutionID = created.InstanceID
	reconnectRequest.AuthToken = created.AuthToken
	reconnectRequest.BootstrapNonce = created.BootstrapNonce
	for key, value := range created.Metadata {
		reconnectRequest.Metadata[key] = value
	}

	reconnected, err := restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, "replacement-pod-uid", reconnected.Metadata[MetadataKeyKubernetesPodUID])
	require.Equal(t, "0", reconnected.Metadata[MetadataKeyKubernetesContainerRestartCount])
	require.Equal(t, "pvc-uid", reconnected.Metadata[MetadataKeyKubernetesPVCUID])
	require.Len(t, resources.createdPVCs, 1, "resume must reuse the recorded PVC")
	require.Len(t, resources.createdPods, 2)
	require.Len(t, reconnectExecs.requests, 4, "replacement Pod needs full bootstrap materialization")
}

func TestKubernetesStopInstancePreservesOrdinaryStopAndForceCleansManagedResources(t *testing.T) {
	t.Run("ordinary stop", func(t *testing.T) {
		controlPort := startKubernetesAgentctlServer(t, true, 41001)
		instancePort := startKubernetesAgentctlServer(t, false, 0)
		resources := &fakeKubernetesResources{}
		forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
			uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
			41001:                                    instancePort,
		}}
		executor := newFakeKubernetesExecutorWithForwarder(resources, &recordingKubernetesExec{}, forwards)
		instance, err := executor.CreateInstance(context.Background(), validKubernetesCreateRequest())
		require.NoError(t, err)

		require.NoError(t, executor.StopInstance(context.Background(), instance, false))

		require.Empty(t, resources.deletedPods)
		require.Empty(t, resources.deletedPVCs)
		require.True(t, forwards.lastSession().isClosed(), "ordinary stop must close the process-local forward")
	})

	t.Run("force cleanup", func(t *testing.T) {
		controlPort := startKubernetesAgentctlServer(t, true, 41001)
		instancePort := startKubernetesAgentctlServer(t, false, 0)
		resources := &fakeKubernetesResources{}
		executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
			uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
			41001:                                    instancePort,
		})
		req := validKubernetesCreateRequest()
		setManagedKubernetesWorkspace(req)
		instance, err := executor.CreateInstance(context.Background(), req)
		require.NoError(t, err)

		require.NoError(t, executor.StopInstance(context.Background(), instance, true))

		require.Equal(t, []string{instance.Metadata[MetadataKeyKubernetesPodName].(string) + ":pod-uid"}, resources.deletedPods)
		require.Equal(t, []string{instance.Metadata[MetadataKeyKubernetesPVCName].(string) + ":pvc-uid"}, resources.deletedPVCs)
		require.Equal(t, []string{"pod-rv-1"}, resources.deletedPodResourceVersions)
		require.Equal(t, []string{"pvc-rv-1"}, resources.deletedPVCResourceVersions)
		require.Equal(t, []string{"pod", "pvc"}, resources.deletionOrder)
	})
}

func TestDeleteKubernetesResourcesWaitsForPodAndPVCRemoval(t *testing.T) {
	resources := &fakeKubernetesResources{
		pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "kandev-agents", Name: "pod-1", UID: "pod-uid", ResourceVersion: "pod-rv-1",
		}},
		pvc: &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Namespace: "kandev-agents", Name: "pvc-1", UID: "pvc-uid", ResourceVersion: "pvc-rv-1",
		}},
		retainPodAfterDelete: true, retainPVCAfterDelete: true,
		podPostDeleteGetsUntilAbsent: 1, pvcPostDeleteGetsUntilAbsent: 1,
	}
	identity := kubeexecutor.ResourceIdentity{
		ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "instance-1",
		TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1",
	}
	labels, err := kubeexecutor.OwnershipLabels(identity)
	require.NoError(t, err)
	resources.pod.Labels = labels
	resources.pvc.Labels = labels
	recorded := kubernetesRecordedState{
		namespace: "kandev-agents", podName: "pod-1", podUID: "pod-uid",
		pvcName: "pvc-1", pvcUID: "pvc-uid", pvcCreated: true,
		inventoryState: KubernetesInventoryStateReady,
	}

	require.NoError(t, deleteKubernetesResources(context.Background(), resources, recorded, identity))

	resources.mu.Lock()
	defer resources.mu.Unlock()
	require.Nil(t, resources.pod, "successful cleanup must observe Pod absence")
	require.Nil(t, resources.pvc, "successful cleanup must observe PVC absence")
	require.Equal(t, []string{"pod", "pvc"}, resources.deletionOrder)
}

func TestDeleteKubernetesPodFailsWhenTerminationDoesNotComplete(t *testing.T) {
	resources := &fakeKubernetesResources{
		pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "kandev-agents", Name: "pod-1", UID: "pod-uid", ResourceVersion: "pod-rv-1",
		}},
		retainPodAfterDelete: true, podPostDeleteGetsUntilAbsent: -1,
	}
	identity := kubeexecutor.ResourceIdentity{
		ExecutorID: "executor-1", ProfileID: "profile-1", InstanceID: "instance-1",
		TaskID: "task-1", SessionID: "session-1", EnvironmentID: "environment-1",
	}
	labels, err := kubeexecutor.OwnershipLabels(identity)
	require.NoError(t, err)
	resources.pod.Labels = labels
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = deleteKubernetesPodIfExact(ctx, resources, resources.pod.DeepCopy(), identity)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	resources.mu.Lock()
	defer resources.mu.Unlock()
	require.NotNil(t, resources.pod, "stalled deletion must remain tracked")
}

func TestKubernetesStopInstanceRunsTerminalCleanupScriptAfterExactVerification(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	execs := &recordingKubernetesExec{}
	executor := newFakeKubernetesExecutor(t, resources, execs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	instance, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	instance.StopReason = StopReasonTaskDeleted
	instance.Metadata[MetadataKeyCleanupScript] = "echo cleanup {{workspace.path}}"
	verifiedBeforeCleanup := false
	resources.beforeDeletePod = func(*corev1.Pod) {
		execs.mu.Lock()
		defer execs.mu.Unlock()
		for _, recorded := range execs.requests {
			command := strings.Join(recorded.request.Command, " ")
			if strings.Contains(command, "echo cleanup") && strings.Contains(command, "/workspace") {
				verifiedBeforeCleanup = true
			}
		}
	}

	require.NoError(t, executor.StopInstance(context.Background(), instance, true))

	require.True(t, verifiedBeforeCleanup, "cleanup script must execute after verification and before Pod deletion")
	require.Equal(t, []string{"pod", "pvc"}, resources.deletionOrder)
}

func TestKubernetesStopInstanceCleanupScriptFailureDoesNotBlockExactDeletion(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	execs := &recordingKubernetesExec{}
	executor := newFakeKubernetesExecutor(t, resources, execs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	instance, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	instance.StopReason = StopReasonTaskDeleted
	instance.Metadata[MetadataKeyCleanupScript] = "echo cleanup"
	execs.err = errors.New("cleanup command failed")

	require.NoError(t, executor.StopInstance(context.Background(), instance, true))

	execs.mu.Lock()
	cleanupAttempted := false
	for _, recorded := range execs.requests {
		cleanupAttempted = cleanupAttempted || strings.Contains(strings.Join(recorded.request.Command, " "), "echo cleanup")
	}
	execs.mu.Unlock()
	require.True(t, cleanupAttempted)
	require.Equal(t, []string{"pod", "pvc"}, resources.deletionOrder)
}

func TestKubernetesStopInstanceValidatesPVCBeforeDeletingPod(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	instance, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pvc.UID = "same-name-foreign-uid"
	resources.mu.Unlock()

	err = executor.StopInstance(context.Background(), instance, true)

	require.ErrorContains(t, err, "PVC UID")
	require.Empty(t, resources.deletedPods, "cleanup must validate every identity before deleting anything")
	require.Empty(t, resources.deletedPVCs)
}

func TestKubernetesStopInstanceFailsClosedOnConcurrentPodMutation(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	instance, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)
	resources.mu.Lock()
	resources.beforeDeletePod = func(pod *corev1.Pod) { pod.ResourceVersion = "pod-rv-concurrent" }
	resources.mu.Unlock()

	err = executor.StopInstance(context.Background(), instance, true)

	require.ErrorContains(t, err, "precondition failed")
	require.Empty(t, resources.deletedPods)
	require.Empty(t, resources.deletedPVCs)
}

func TestKubernetesStopInstanceNeverDeletesExistingClaim(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{pvc: &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-workspace", Namespace: "kandev-agents", UID: "shared-pvc-uid"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	req.Metadata[MetadataKeyKubernetesWorkspaceMode] = "existing_claim"
	req.Metadata[MetadataKeyKubernetesWorkspaceClaimName] = "shared-workspace"
	instance, err := executor.CreateInstance(context.Background(), req)
	require.NoError(t, err)

	require.NoError(t, executor.StopInstance(context.Background(), instance, true))

	require.Len(t, resources.deletedPods, 1)
	require.Empty(t, resources.deletedPVCs)
	resources.mu.Lock()
	defer resources.mu.Unlock()
	require.NotNil(t, resources.pvc, "existing claims are never lifecycle-owned")
}

func TestKubernetesRemoteStatusUsesExactRecordedPodAndContainerState(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	instance, err := executor.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod.Status.Phase = corev1.PodPending
	resources.pod.Status.Reason = "Scheduling"
	resources.pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "kandev-agent", RestartCount: 2,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull delayed"}},
	}}
	resources.getPodRequests = nil
	resources.mu.Unlock()

	provider, ok := interface{}(executor).(RemoteStatusProvider)
	require.True(t, ok, "Kubernetes executor must expose remote Pod status")
	status, err := provider.GetRemoteStatus(context.Background(), instance)

	require.NoError(t, err)
	require.Equal(t, "starting", status.State)
	require.Equal(t, "ImagePullBackOff", status.Details["reason"])
	require.Equal(t, int32(2), status.Details["restart_count"])
	require.Equal(t,
		[]string{instance.Metadata[MetadataKeyKubernetesNamespace].(string) + "/" + instance.Metadata[MetadataKeyKubernetesPodName].(string)},
		resources.getPodRequests,
	)

	resources.mu.Lock()
	resources.pod.UID = "same-name-foreign-uid"
	resources.mu.Unlock()
	_, err = provider.GetRemoteStatus(context.Background(), instance)
	require.ErrorContains(t, err, "UID")
}
