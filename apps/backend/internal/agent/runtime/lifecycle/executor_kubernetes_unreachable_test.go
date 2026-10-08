package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.2, .6, .13
func TestExecutorFailureWorkerUnavailableOverridesStaleContainerReadiness(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		t.Run(map[bool]string{false: "running", true: "taint-deletion"}[deleting], func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "owned-pod"}, Status: corev1.PodStatus{Phase: corev1.PodRunning,
				Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "NodeNotReady", Message: "Node is not ready"}},
				ContainerStatuses: []corev1.ContainerStatus{{Name: "kandev-agent", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}},
			}}
			if deleting {
				now := metav1.Now()
				pod.DeletionTimestamp = &now
				pod.Status.Conditions = append(pod.Status.Conditions, corev1.PodCondition{Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue, Reason: "DeletionByTaintManager"})
			}
			obs := kubernetesExecutorObservation(pod, "kandev-agent", time.Now().UTC())
			require.Equal(t, "unknown", obs.Outcome)
			require.Equal(t, "WorkerUnavailable", obs.Reason)
			require.False(t, obs.ContainerReady)
			require.False(t, obs.ConfirmedLoss(), "unreachable worker does not prove process death")
			require.NotContains(t, obs.Message, "OOM")
			raw, err := json.Marshal(obs)
			require.NoError(t, err)
			require.Contains(t, string(raw), "Node is not ready")
			require.ErrorContains(t, kubernetesPodControlPreflight(pod, "kandev-agent"), "WorkerUnavailable")
		})
	}
}

func TestExecutorFailurePodNotReadyCannotProjectHealthy(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"}}, ContainerStatuses: []corev1.ContainerStatus{{Name: "kandev-agent", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}}
	obs := kubernetesExecutorObservation(pod, "kandev-agent", time.Now().UTC())
	require.Equal(t, "unknown", obs.Outcome)
	require.Equal(t, "PodNotReady", obs.Reason, "readiness failure alone cannot identify a worker outage")
}

func TestExecutorFailureUnavailableDisconnectPersistsWithoutRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		obs := &models.ExecutorObservation{Outcome: "unknown", Runtime: "k8s", ResourceKey: "pod", ObservedAt: time.Now().UTC(), Reason: "WorkerUnavailable"}
		provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}, status: &RemoteStatus{Observation: obs}}
		mgr := newRemoteStatusManager(t, provider)
		mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
		calls := 0
		mgr.SetExecutorObservationHandler(func(_ context.Context, _ models.ExecutorObservationTarget, got *models.ExecutorObservation) error {
			calls++
			require.Equal(t, "WorkerUnavailable", got.Reason)
			return nil
		})
		execution := &AgentExecution{ID: "uncertain", SessionID: "session", TaskID: "task", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeKubernetes, promptDoneCh: make(chan PromptCompletionSignal, 1)}
		execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
		require.NoError(t, mgr.executionStore.Add(execution))
		mgr.storeRemoteStatus("session", &RemoteStatus{RuntimeName: agentruntime.RuntimeKubernetes, State: "running", LastCheckedAt: time.Now().UTC()})
		mgr.classifyExecutorDisconnect(execution, 0, 0)
		cached, ok := mgr.GetRemoteStatusBySession("session")
		require.True(t, ok)
		require.Equal(t, "unknown", cached.State)
		require.Equal(t, "WorkerUnavailable", cached.Observation.Reason)
		require.Equal(t, 1, calls, "stable worker evidence must become a durable uncertain warning")
		current, ok := mgr.executionStore.GetBySessionID("session")
		require.True(t, ok)
		require.Same(t, execution, current)
		require.Empty(t, execution.promptDoneCh)
		err := mgr.existingExecutorLaunchError(t.Context(), execution)
		require.ErrorContains(t, err, "WorkerUnavailable")
		require.NotErrorIs(t, err, ErrAgentAlreadyRunning)
	})
}

func TestExecutorFailureUnavailableCleanupKeepsPrimaryConnectivityCause(t *testing.T) {
	provider := &statusProviderExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes, stopInstanceErr: errors.New("port-forward proxy HTTP 502")}, status: &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: "unknown", Runtime: "k8s", ResourceKey: "pod", ObservedAt: time.Now().UTC(), Reason: "WorkerUnavailable"}}}
	mgr := newRemoteStatusManager(t, provider)
	mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
	execution := &AgentExecution{ID: "uncertain", SessionID: "session", TaskID: "task", TaskEnvironmentID: "env", RuntimeName: agentruntime.RuntimeKubernetes}
	execution.setMetadataValue(MetadataKeyKubernetesPodUID, "pod")
	require.NoError(t, mgr.executionStore.Add(execution))
	err := mgr.cleanupStaleExecutionWithCause(t.Context(), execution)
	var unavailable *ExecutorUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.Equal(t, "WorkerUnavailable", unavailable.Observation.Reason)
	require.Equal(t, "unknown", unavailable.Observation.Outcome)
	require.Len(t, unavailable.Observation.Secondary, 1)
	require.Equal(t, "cleanup_failed", unavailable.Observation.Secondary[0].Reason)
}

func TestExecutorFailureReadinessDoesNotHideCrashLoopTermination(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"}}, ContainerStatuses: []corev1.ContainerStatus{{Name: "kandev-agent", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 2}}}}}}
	obs := kubernetesExecutorObservation(pod, "kandev-agent", time.Now().UTC())
	require.Equal(t, "terminated", obs.Outcome)
	require.Equal(t, "CrashLoopBackOff", obs.Reason)
}

func TestExecutorFailureResponsiveAPIReportsUnavailableWorkerAndBoundInventory(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	execs := &recordingKubernetesExec{}
	backend := newFakeKubernetesExecutor(t, resources, execs, map[uint16]uint16{8765: controlPort, 41001: instancePort})
	req := validKubernetesCreateRequest()
	req.Metadata["workspace.mode"] = "managed_pvc"
	req.Metadata["workspace.size"] = "1Gi"
	req.Metadata["workspace.access_modes"] = `["ReadWriteOnce"]`
	instance, err := backend.CreateInstance(t.Context(), req)
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "NodeNotReady"}}
	resources.mu.Unlock()
	execs.mu.Lock()
	execs.requests = nil
	execs.mu.Unlock()
	status, err := backend.GetRemoteStatus(t.Context(), instance)
	require.NoError(t, err)
	require.Equal(t, "unknown", status.State)
	require.Equal(t, "WorkerUnavailable", status.Observation.Reason)
	require.Equal(t, string(resources.pod.UID), status.Observation.ResourceKey)
	require.Equal(t, "retained", status.Observation.Workspace, "retention reports matching Bound inventory only")
	require.False(t, status.Observation.ContainerReady)
	require.False(t, status.Observation.ConfirmedLoss())
	_, err = backend.CreateInstance(t.Context(), kubernetesReconnectRequest(instance))
	require.ErrorContains(t, err, "WorkerUnavailable")
	execs.mu.Lock()
	defer execs.mu.Unlock()
	require.Empty(t, execs.requests, "inspection and blocked retry must not exec into the workload")
}
