package lifecycle

import (
	"context"
	"errors"
	"net"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestKubernetesCreateFreshLeavesDurableInventoryProvisionalUntilManagerPersistsSecrets(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	executor := newFakeKubernetesExecutor(
		t, &fakeKubernetesResources{}, &recordingKubernetesExec{},
		map[uint16]uint16{
			uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
			41001:                                    instancePort,
		},
	)
	req := validKubernetesCreateRequest()
	var states []string
	req.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		states = append(states, getMetadataString(metadata, MetadataKeyKubernetesInventoryState))
		return nil
	}
	req.ReleaseRuntimeInventory = func(context.Context) error { return nil }

	instance, err := executor.CreateInstance(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, KubernetesInventoryStateReady, instance.Metadata[MetadataKeyKubernetesInventoryState])
	require.NotEmpty(t, states)
	require.NotContains(t, states, KubernetesInventoryStateReady,
		"manager must persist required runtime secret refs before marking durable inventory ready")
}

func TestKubernetesCreateFreshReconcilesCommittedPodAfterAmbiguousCreateError(t *testing.T) {
	createErr := &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET}
	resources := &fakeKubernetesResources{createPodErr: createErr}
	executor := newFakeKubernetesExecutor(
		t, resources, &recordingKubernetesExec{err: errors.New("bootstrap failed")}, nil,
	)
	req := validKubernetesCreateRequest()
	var states []string
	req.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		states = append(states, getMetadataString(metadata, MetadataKeyKubernetesInventoryState))
		return nil
	}
	releases := 0
	req.ReleaseRuntimeInventory = func(context.Context) error { releases++; return nil }

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "bootstrap")
	require.Len(t, resources.createdPods, 1)
	require.Equal(t, "kandev-agents/"+resources.createdPods[0].Name, resources.getPodRequests[0])
	require.Contains(t, states, KubernetesInventoryStatePodCreated)
	require.Contains(t, states, KubernetesInventoryStatePodAdmitted)
	require.Contains(t, resources.deletedPods, resources.createdPods[0].Name+":pod-uid")
	require.Equal(t, 1, releases)
}

func TestKubernetesCreateFreshReconcilesCommittedPVCAfterAmbiguousCreateError(t *testing.T) {
	createErr := &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET}
	resources := &fakeKubernetesResources{createPVCErr: createErr}
	executor := newFakeKubernetesExecutor(
		t, resources, &recordingKubernetesExec{err: errors.New("bootstrap failed")}, nil,
	)
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	var states []string
	req.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		states = append(states, getMetadataString(metadata, MetadataKeyKubernetesInventoryState))
		return nil
	}
	releases := 0
	req.ReleaseRuntimeInventory = func(context.Context) error { releases++; return nil }

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "bootstrap")
	require.Len(t, resources.createdPVCs, 1)
	require.Equal(t, "kandev-agents/"+resources.createdPVCs[0].Name, resources.getPVCRequests[0])
	require.Contains(t, states, KubernetesInventoryStatePVCCreated)
	require.Contains(t, states, KubernetesInventoryStatePVCAdmitted)
	require.Contains(t, resources.deletedPVCs, resources.createdPVCs[0].Name+":pvc-uid")
	require.Equal(t, 1, releases)
}

func TestKubernetesCreateStampsFreshNonceOnEachManagedResource(t *testing.T) {
	resources := &fakeKubernetesResources{}
	execs := &recordingKubernetesExec{err: errors.New("bootstrap failed")}
	executor := newFakeKubernetesExecutor(t, resources, execs, nil)
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "bootstrap")
	require.Len(t, resources.createdPVCs, 1)
	require.Len(t, resources.createdPods, 1)
	pvcNonce := resources.createdPVCs[0].Annotations[kubeexecutor.CreateNonceAnnotation]
	podNonce := resources.createdPods[0].Annotations[kubeexecutor.CreateNonceAnnotation]
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), pvcNonce)
	require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{64}$`), podNonce)
	require.NotEqual(t, pvcNonce, podNonce)
}

func TestKubernetesAmbiguousCreateWithoutExactNonceFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		resources *fakeKubernetesResources
		configure func(*ExecutorCreateRequest)
	}{
		{
			name: "Pod missing nonce",
			resources: &fakeKubernetesResources{
				createPodErr: &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET},
				mutateCreatedPod: func(pod *corev1.Pod) {
					delete(pod.Annotations, kubeexecutor.CreateNonceAnnotation)
				},
			},
			configure: func(*ExecutorCreateRequest) {},
		},
		{
			name: "Pod different nonce",
			resources: &fakeKubernetesResources{
				createPodErr: &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET},
				mutateCreatedPod: func(pod *corev1.Pod) {
					if pod.Annotations == nil {
						pod.Annotations = make(map[string]string)
					}
					pod.Annotations[kubeexecutor.CreateNonceAnnotation] = "foreign"
				},
			},
			configure: func(*ExecutorCreateRequest) {},
		},
		{
			name: "PVC missing nonce",
			resources: &fakeKubernetesResources{
				createPVCErr: &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET},
				mutateCreatedPVC: func(pvc *corev1.PersistentVolumeClaim) {
					delete(pvc.Annotations, kubeexecutor.CreateNonceAnnotation)
				},
			},
			configure: setManagedKubernetesWorkspace,
		},
		{
			name: "PVC different nonce",
			resources: &fakeKubernetesResources{
				createPVCErr: &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET},
				mutateCreatedPVC: func(pvc *corev1.PersistentVolumeClaim) {
					if pvc.Annotations == nil {
						pvc.Annotations = make(map[string]string)
					}
					pvc.Annotations[kubeexecutor.CreateNonceAnnotation] = "foreign"
				},
			},
			configure: setManagedKubernetesWorkspace,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execs := &recordingKubernetesExec{}
			executor := newFakeKubernetesExecutor(t, test.resources, execs, nil)
			req := validKubernetesCreateRequest()
			test.configure(req)
			checkpoints := 0
			req.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error {
				checkpoints++
				return nil
			}

			_, err := executor.CreateInstance(context.Background(), req)

			require.ErrorContains(t, err, "owned annotation")
			require.Zero(t, checkpoints)
			require.Empty(t, execs.requests)
			require.Empty(t, test.resources.deletedPods)
			require.Empty(t, test.resources.deletedPVCs)
		})
	}
}

func TestKubernetesAmbiguousPodCreateNeverDeletesMismatchedObject(t *testing.T) {
	resources := &fakeKubernetesResources{
		createPodErr: &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET},
		mutateCreatedPod: func(pod *corev1.Pod) {
			pod.Labels["kandev.ai/copied-owner"] = "other-session"
		},
	}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	req := validKubernetesCreateRequest()
	var checkpoints int
	req.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error {
		checkpoints++
		return nil
	}

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "Pod ownership labels")
	require.Zero(t, checkpoints)
	require.Empty(t, resources.deletedPods)
	require.NotNil(t, resources.pod, "foreign same-name Pod must remain untouched")
}

func TestKubernetesAmbiguousPVCCreateNeverDeletesMismatchedObject(t *testing.T) {
	resources := &fakeKubernetesResources{
		createPVCErr: &net.OpError{Op: "write", Net: "tcp", Err: syscall.ECONNRESET},
		mutateCreatedPVC: func(pvc *corev1.PersistentVolumeClaim) {
			pvc.Labels["kandev.ai/copied-owner"] = "other-session"
		},
	}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	var checkpoints int
	req.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error {
		checkpoints++
		return nil
	}

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "PVC ownership labels")
	require.Zero(t, checkpoints)
	require.Empty(t, resources.deletedPVCs)
	require.NotNil(t, resources.pvc, "foreign same-name PVC must remain untouched")
}

func TestKubernetesCreateAlreadyExistsNeverAdoptsVisiblePodOrPVC(t *testing.T) {
	tests := []struct {
		name      string
		resources *fakeKubernetesResources
		configure func(*ExecutorCreateRequest)
	}{
		{
			name: "Pod",
			resources: &fakeKubernetesResources{createPodErr: apierrors.NewAlreadyExists(
				schema.GroupResource{Resource: "pods"}, "visible-pod",
			)},
			configure: func(*ExecutorCreateRequest) {},
		},
		{
			name: "PVC",
			resources: &fakeKubernetesResources{createPVCErr: apierrors.NewAlreadyExists(
				schema.GroupResource{Resource: "persistentvolumeclaims"}, "visible-pvc",
			)},
			configure: setManagedKubernetesWorkspace,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := newFakeKubernetesExecutor(t, test.resources, &recordingKubernetesExec{}, nil)
			req := validKubernetesCreateRequest()
			test.configure(req)
			checkpoints := 0
			req.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error {
				checkpoints++
				return nil
			}

			_, err := executor.CreateInstance(context.Background(), req)

			require.ErrorContains(t, err, "already exists")
			require.Zero(t, checkpoints, "known create rejection must not authorize visible-object adoption")
			require.Empty(t, test.resources.getPodRequests)
			require.Empty(t, test.resources.getPVCRequests)
			require.Empty(t, test.resources.deletedPods)
			require.Empty(t, test.resources.deletedPVCs)
		})
	}
}

func TestKubernetesRollbackUsesCurrentResourceVersionForMatchingCreateUID(t *testing.T) {
	t.Run("Pod", func(t *testing.T) {
		created := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "agents", Name: "pod", UID: types.UID("pod-uid"), ResourceVersion: "rv-create",
		}}
		resources := &fakeKubernetesResources{pod: created.DeepCopy()}
		resources.pod.ResourceVersion = "rv-current"
		resources.pod.Labels = map[string]string{"kandev.ai/admission-mutated": "true"}

		err := rollbackCreatedKubernetesPod(context.Background(), resources, created)

		require.NoError(t, err)
		require.Equal(t, []string{"rv-current"}, resources.deletedPodResourceVersions)
	})

	t.Run("PVC", func(t *testing.T) {
		created := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Namespace: "agents", Name: "pvc", UID: types.UID("pvc-uid"), ResourceVersion: "rv-create",
		}}
		resources := &fakeKubernetesResources{pvc: created.DeepCopy()}
		resources.pvc.ResourceVersion = "rv-current"
		resources.pvc.Labels = map[string]string{"kandev.ai/admission-mutated": "true"}

		err := rollbackCreatedKubernetesPVC(context.Background(), resources, created)

		require.NoError(t, err)
		require.Equal(t, []string{"rv-current"}, resources.deletedPVCResourceVersions)
	})
}

func TestKubernetesRollbackLeavesSameNameReplacementUntouched(t *testing.T) {
	created := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "agents", Name: "pod", UID: types.UID("created-uid"), ResourceVersion: "rv-create",
	}}
	resources := &fakeKubernetesResources{pod: created.DeepCopy()}
	resources.pod.UID = types.UID("replacement-uid")
	resources.pod.ResourceVersion = "rv-replacement"

	require.NoError(t, rollbackCreatedKubernetesPod(context.Background(), resources, created))
	require.Empty(t, resources.deletedPods)
	require.Equal(t, types.UID("replacement-uid"), resources.pod.UID)
}

func TestKubernetesCreateNotFoundReconciliationPreservesOriginalCreateError(t *testing.T) {
	tests := []struct {
		name      string
		resources *fakeKubernetesResources
		configure func(*ExecutorCreateRequest)
		getCalls  func(*fakeKubernetesResources) int
	}{
		{
			name: "Pod",
			resources: &fakeKubernetesResources{createPodBeforeCommitErr: &net.OpError{
				Op: "write", Net: "tcp", Err: errors.New("Pod create transport failed"),
			}},
			configure: func(*ExecutorCreateRequest) {},
			getCalls:  func(resources *fakeKubernetesResources) int { return len(resources.getPodRequests) },
		},
		{
			name: "PVC",
			resources: &fakeKubernetesResources{createPVCBeforeCommitErr: &net.OpError{
				Op: "write", Net: "tcp", Err: errors.New("PVC create transport failed"),
			}},
			configure: setManagedKubernetesWorkspace,
			getCalls:  func(resources *fakeKubernetesResources) int { return len(resources.getPVCRequests) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := newFakeKubernetesExecutor(t, test.resources, &recordingKubernetesExec{}, nil)
			req := validKubernetesCreateRequest()
			test.configure(req)

			_, err := executor.CreateInstance(context.Background(), req)

			require.ErrorContains(t, err, test.name+" create transport failed")
			require.Equal(t, 1, test.getCalls(test.resources))
			require.Empty(t, test.resources.deletedPods)
			require.Empty(t, test.resources.deletedPVCs)
		})
	}
}

func TestKubernetesAmbiguousCreateReconciliationOutlivesCanceledCreateContext(t *testing.T) {
	t.Run("Pod", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		resources := &fakeKubernetesResources{
			createPodErr:              context.Canceled,
			rejectCanceledGetContexts: true,
			mutateCreatedPod:          func(*corev1.Pod) { cancel() },
		}
		req := validKubernetesCreateRequest()
		profile, err := kubernetesProfileConfigFromMetadata(req.Metadata)
		require.NoError(t, err)
		identity, err := kubernetesIdentity(req)
		require.NoError(t, err)
		desired, err := composeKubernetesLifecyclePod(
			profile, identity, "pod", "kandev-agents", "",
		)
		require.NoError(t, err)

		created, running, err := createAndWaitKubernetesPod(
			ctx, resources, desired, identity, profile.MainContainer, "Pod", nil,
		)

		require.NoError(t, err)
		require.NotNil(t, created)
		require.NotNil(t, running)
	})

	t.Run("PVC", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		resources := &fakeKubernetesResources{
			createPVCErr:              context.Canceled,
			rejectCanceledGetContexts: true,
			mutateCreatedPVC:          func(*corev1.PersistentVolumeClaim) { cancel() },
		}
		workspace := kubeexecutor.WorkspaceConfig{
			Mode: kubeexecutor.WorkspaceModeManagedPVC, Size: "1Gi",
			AccessModes: []string{"ReadWriteOnce"},
		}
		identity, err := kubernetesIdentity(validKubernetesCreateRequest())
		require.NoError(t, err)

		provision, err := createManagedKubernetesWorkspacePVC(
			ctx, resources, "kandev-agents", workspace, identity, "workspace", nil,
		)

		require.NoError(t, err)
		require.NotNil(t, provision.claim)
	})
}

func TestKubernetesCreateFreshRetainsCheckpointWhenRollbackFails(t *testing.T) {
	deleteErr := errors.New("delete denied")
	resources := &fakeKubernetesResources{
		deletePodErr: deleteErr,
		mutateCreatedPod: func(pod *corev1.Pod) {
			pod.Labels["kandev.ai/injected"] = "unsafe"
		},
	}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	var checkpoints []map[string]interface{}
	releases := 0
	req.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		checkpoints = append(checkpoints, cloneKubernetesMetadata(metadata))
		return nil
	}
	req.ReleaseRuntimeInventory = func(context.Context) error {
		releases++
		return nil
	}

	_, err := executor.CreateInstance(context.Background(), req)

	require.Error(t, err)
	require.ErrorIs(t, err, deleteErr, "rollback error must stay causal")
	require.NotEmpty(t, checkpoints)
	last := checkpoints[len(checkpoints)-1]
	require.Equal(t, KubernetesInventoryStatePodCreated, last[MetadataKeyKubernetesInventoryState])
	require.Equal(t, "pod-uid", last[MetadataKeyKubernetesPodUID])
	require.Equal(t, "pvc-uid", last[MetadataKeyKubernetesPVCUID])
	require.Zero(t, releases, "failed rollback must retain exact durable inventory")
	require.NotNil(t, resources.pod)
	require.Nil(t, resources.pvc, "successful PVC rollback may complete independently")

	resources.deletePodErr = nil
	cleanupMetadata := cloneKubernetesMetadata(req.Metadata)
	for key, value := range last {
		cleanupMetadata[key] = value
	}
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	require.NoError(t, restarted.StopInstance(context.Background(), &ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		Metadata: cleanupMetadata, StopReason: StopReasonTaskDeleted,
	}, true))
	require.Nil(t, resources.pod, "recorded Create UID must authorize retry after admission mutation")
}

func TestKubernetesCreateFreshReleasesCheckpointAfterSuccessfulRollback(t *testing.T) {
	resources := &fakeKubernetesResources{mutateCreatedPod: func(pod *corev1.Pod) {
		pod.Labels["kandev.ai/injected"] = "unsafe"
	}}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)
	checkpoints := 0
	releases := 0
	req.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error {
		checkpoints++
		return nil
	}
	req.ReleaseRuntimeInventory = func(context.Context) error {
		releases++
		return nil
	}

	_, err := executor.CreateInstance(context.Background(), req)

	require.Error(t, err)
	require.Positive(t, checkpoints)
	require.Equal(t, 1, releases)
	require.Nil(t, resources.pod)
	require.Nil(t, resources.pvc)
}

func TestCheckpointKubernetesRuntimeInventoryPersistsBeforeExecutionRegistration(t *testing.T) {
	writer := &checkpointExecutorRunningWriter{}
	mgr := &Manager{runningWriter: writer, logger: newTestRegistryLogger()}
	req := &ExecutorCreateRequest{
		InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "agent-profile-1",
		Metadata: map[string]interface{}{
			"executor_id":                        "executor-1",
			MetadataKeyExecutorProfileID:         "executor-profile-1",
			MetadataKeyCleanupScript:             "cleanup",
			MetadataKeyKubernetesAuthMode:        "in_cluster",
			MetadataKeyKubernetesConfigNamespace: "kandev-agents",
			"raw_secret_value":                   "must-not-persist",
		},
	}
	runtimeMetadata := map[string]interface{}{
		MetadataKeyKubernetesInventoryState:        KubernetesInventoryStatePodCreated,
		MetadataKeyKubernetesNamespace:             "kandev-agents",
		MetadataKeyKubernetesPodName:               "pod-1",
		MetadataKeyKubernetesPodUID:                "pod-uid",
		MetadataKeyKubernetesResourceInstanceID:    "resource-1",
		MetadataKeyKubernetesResourceExecutorID:    "executor-1",
		MetadataKeyKubernetesResourceProfileID:     "executor-profile-1",
		MetadataKeyKubernetesResourceTaskID:        "task-1",
		MetadataKeyKubernetesResourceSessionID:     "session-1",
		MetadataKeyKubernetesResourceEnvironmentID: "environment-1",
	}

	err := mgr.checkpointKubernetesRuntimeInventory(context.Background(), req, runtimeMetadata)

	require.NoError(t, err)
	require.NotNil(t, writer.running)
	require.Equal(t, "execution-1", writer.running.AgentExecutionID)
	require.Equal(t, "executor-1", writer.running.ExecutorID)
	require.Equal(t, "agent-profile-1", writer.running.ExecutionProfileID)
	require.Equal(t, agentruntime.RuntimeKubernetes, writer.running.Runtime)
	require.Equal(t, models.ExecutorRunningStatusStarting, writer.running.Status)
	require.Equal(t, "pod-uid", writer.running.Metadata[MetadataKeyKubernetesPodUID])
	require.Equal(t, "cleanup", writer.running.Metadata[MetadataKeyCleanupScript])
	require.NotContains(t, writer.running.Metadata, "raw_secret_value")

	require.NoError(t, mgr.releaseKubernetesRuntimeInventory(context.Background(), req))
	require.Equal(t, "session-1", writer.deletedSessionID)
	require.Equal(t, "execution-1", writer.deletedExecutionID)
}

func TestKubernetesInventoryPersistenceOutlivesCanceledLaunchContext(t *testing.T) {
	writer := &checkpointExecutorRunningWriter{rejectCanceledCalls: true}
	mgr := &Manager{runningWriter: writer, logger: newTestRegistryLogger()}
	req := &ExecutorCreateRequest{
		InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "agent-profile-1",
		Metadata:       map[string]interface{}{"executor_id": "executor-1"},
	}
	runtimeMetadata := map[string]interface{}{
		MetadataKeyKubernetesInventoryState: KubernetesInventoryStatePodCreated,
		MetadataKeyKubernetesPodName:        "pod-1",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, mgr.checkpointKubernetesRuntimeInventory(ctx, req, runtimeMetadata))
	require.NotNil(t, writer.running)
	require.NoError(t, mgr.releaseKubernetesRuntimeInventory(ctx, req))
	require.Nil(t, writer.running)
}

func TestRollbackLaunchExecutionReleasesInventoryOnlyAfterRuntimeCleanup(t *testing.T) {
	tests := []struct {
		name         string
		stopErr      error
		wantReleases int
	}{
		{name: "cleanup succeeds", wantReleases: 1},
		{name: "cleanup fails", stopErr: errors.New("cleanup denied")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mgr := &Manager{logger: newTestRegistryLogger()}
			backend := &mockStopTracker{name: agentruntime.RuntimeKubernetes, stopErr: test.stopErr}
			releases := 0
			instance := &ExecutorInstance{ReleaseRuntimeInventory: func(context.Context) error {
				releases++
				return nil
			}}
			execution := &AgentExecution{ID: "execution-1", SessionID: "session-1"}

			mgr.rollbackLaunchExecution(context.Background(), backend, instance, execution, "test rollback")

			require.Equal(t, test.wantReleases, releases)
		})
	}
}

func TestRollbackRegisteredLaunchPreservesInventoryUntilRuntimeCleanupSucceeds(t *testing.T) {
	writer := &rollbackCheckpointWriter{checkpointExecutorRunningWriter: checkpointExecutorRunningWriter{
		running: &models.ExecutorRunning{
			SessionID: "session-1", AgentExecutionID: "execution-1",
			Runtime: agentruntime.RuntimeKubernetes,
		},
	}}
	mgr := &Manager{
		runningWriter: writer, logger: newTestRegistryLogger(), stopCh: make(chan struct{}),
		executionStore: NewExecutionStore(),
	}
	execution := &AgentExecution{ID: "execution-1", SessionID: "session-1"}
	require.NoError(t, mgr.executionStore.Add(execution))
	backend := &mockStopTracker{
		name: agentruntime.RuntimeKubernetes, stopErr: errors.New("exact cleanup failed"),
	}

	mgr.rollbackRegisteredLaunch(backend, &ExecutorInstance{}, execution, "test rollback")

	require.NotNil(t, writer.running, "cleanup failure must retain authoritative row")
	_, exists := mgr.executionStore.Get("execution-1")
	require.True(t, exists, "cleanup failure must retain retry ownership")
	mgr.closeStopCh()
	mgr.wg.Wait()
}

func TestFreshKubernetesSiblingRollbackOwnsItsNewRuntime(t *testing.T) {
	mgr := &Manager{logger: newTestRegistryLogger()}
	backend := &mockStopTracker{name: agentruntime.RuntimeKubernetes}
	releases := 0
	instance := &ExecutorInstance{ReleaseRuntimeInventory: func(context.Context) error {
		releases++
		return nil
	}}
	execution := &AgentExecution{
		ID: "execution-new", SessionID: "session-new", RuntimeName: agentruntime.RuntimeKubernetes,
	}
	applyResumeIntent(execution, &ExecutorCreateRequest{PreviousExecutionID: ""})

	err := mgr.stopRegisteredLaunchRuntime(backend, instance, execution)

	require.NoError(t, err)
	require.False(t, execution.isResumedSession)
	require.True(t, backend.stopForce, "fresh sibling Pod/PVC must be deleted on launch rollback")
	require.Equal(t, 1, releases, "fresh provisional inventory must be released after exact cleanup")
}

type checkpointExecutorRunningWriter struct {
	running             *models.ExecutorRunning
	deletedSessionID    string
	deletedExecutionID  string
	rejectCanceledCalls bool
}

type rollbackCheckpointWriter struct {
	checkpointExecutorRunningWriter
}

func (w *rollbackCheckpointWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	w.running = nil
	return nil
}

func (w *checkpointExecutorRunningWriter) UpsertExecutorRunning(
	ctx context.Context,
	running *models.ExecutorRunning,
) error {
	if w.rejectCanceledCalls && ctx.Err() != nil {
		return ctx.Err()
	}
	w.running = running
	return nil
}

func (w *checkpointExecutorRunningWriter) GetExecutorRunningBySessionID(
	ctx context.Context,
	_ string,
) (*models.ExecutorRunning, error) {
	if w.rejectCanceledCalls && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if w.running == nil {
		return nil, models.ErrExecutorRunningNotFound
	}
	return w.running, nil
}

func (*checkpointExecutorRunningWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	return errors.New("non-CAS delete must not be used")
}

func (*checkpointExecutorRunningWriter) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

func (*checkpointExecutorRunningWriter) RepairExecutorRunningDeadIfCurrent(
	context.Context,
	string,
	string,
	time.Time,
) error {
	return nil
}

func (w *checkpointExecutorRunningWriter) DeleteExecutorRunningIfCurrent(
	ctx context.Context,
	sessionID string,
	executionID string,
	_ time.Time,
) error {
	if w.rejectCanceledCalls && ctx.Err() != nil {
		return ctx.Err()
	}
	w.deletedSessionID = sessionID
	w.deletedExecutionID = executionID
	w.running = nil
	return nil
}
