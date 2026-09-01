package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestKubernetesPodReadyRejectsImagePullFailureWithSanitizedDiagnostics(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodPending,
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "kandev-agent",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason:  "ImagePullBackOff",
				Message: "pull https://registry.example/private/image?token=kandev_pat_supersecret",
			}},
		}},
	}}

	ready, err := kubernetesPodReady(pod, "kandev-agent")

	require.False(t, ready)
	require.ErrorContains(t, err, "ImagePullBackOff")
	require.NotContains(t, err.Error(), "/private/image")
	require.NotContains(t, err.Error(), "supersecret")
}

func TestKubernetesWaitForPodRunningAllowsTransientUnschedulablePod(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodPending,
		Conditions: []corev1.PodCondition{{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable",
			Message: "see https://cluster.example/schedule?token=kandev_pat_supersecret",
		}},
	}}

	pod.Name = "agent"
	pod.Namespace = "agents"
	pod.ResourceVersion = "1"
	clientset := kubefake.NewSimpleClientset(pod)
	podWatch := watch.NewRaceFreeFake()
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, podWatch, nil
	})
	resources := &liveKubernetesResources{client: &kubeexecutor.Client{Clientset: clientset}}
	running := pod.DeepCopy()
	running.ResourceVersion = "2"
	running.Status.Conditions = nil
	running.Status.Phase = corev1.PodRunning
	running.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "kandev-agent", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}}
	go func() {
		podWatch.Modify(pod.DeepCopy())
		podWatch.Modify(running)
	}()

	got, err := resources.WaitForPodRunning(context.Background(), "agents", "agent", "kandev-agent")

	require.NoError(t, err)
	require.Equal(t, corev1.PodRunning, got.Status.Phase)
}

func TestKubernetesWaitForPodRunningSurfacesSchedulingDiagnosticAtDeadline(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "agent", Namespace: "agents", ResourceVersion: "1",
	}, Status: corev1.PodStatus{
		Phase: corev1.PodPending,
		Conditions: []corev1.PodCondition{{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: "Unschedulable",
			Message: "see https://cluster.example/schedule?token=kandev_pat_supersecret",
		}},
	}}
	clientset := kubefake.NewSimpleClientset(pod)
	podWatch := watch.NewRaceFreeFake()
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, podWatch, nil
	})
	resources := &liveKubernetesResources{client: &kubeexecutor.Client{Clientset: clientset}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := resources.WaitForPodRunning(ctx, "agents", "agent", "kandev-agent")

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "Unschedulable")
	require.NotContains(t, err.Error(), "/schedule")
	require.NotContains(t, err.Error(), "supersecret")
}

func TestKubernetesWaitForPodRunningRelistsAfterWatchCloses(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "agent", Namespace: "agents", ResourceVersion: "1",
	}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
	clientset := kubefake.NewSimpleClientset(pod)
	closedWatch := watch.NewRaceFreeFake()
	closedWatch.Stop()
	runningWatch := watch.NewRaceFreeFake()
	watchCalls := 0
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		watchCalls++
		if watchCalls == 1 {
			return true, closedWatch, nil
		}
		return true, runningWatch, nil
	})
	running := pod.DeepCopy()
	running.ResourceVersion = "2"
	running.Status.Phase = corev1.PodRunning
	running.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "kandev-agent", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}}
	go runningWatch.Modify(running)
	resources := &liveKubernetesResources{client: &kubeexecutor.Client{Clientset: clientset}}

	got, err := resources.WaitForPodRunning(context.Background(), "agents", "agent", "kandev-agent")

	require.NoError(t, err)
	require.Equal(t, corev1.PodRunning, got.Status.Phase)
	require.Equal(t, 2, watchCalls)
}

func TestKubernetesWaitForPodRunningBacksOffBeforeRelistingClosedWatch(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "agent", Namespace: "agents", ResourceVersion: "1",
	}, Status: corev1.PodStatus{Phase: corev1.PodPending}}
	clientset := kubefake.NewSimpleClientset(pod)
	watchCalls := 0
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		watchCalls++
		closedWatch := watch.NewRaceFreeFake()
		closedWatch.Stop()
		return true, closedWatch, nil
	})
	backoffEntered := make(chan struct{})
	resources := &liveKubernetesResources{
		client: &kubeexecutor.Client{Clientset: clientset},
		watchRelistBackoff: func(ctx context.Context) error {
			close(backoffEntered)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := resources.WaitForPodRunning(ctx, "agents", "agent", "kandev-agent")
		done <- err
	}()
	<-backoffEntered
	cancel()

	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, 1, watchCalls, "closed watches must not trigger a tight relist loop")
}

func TestKubernetesPodReadyRejectsInitContainerImagePullFailure(t *testing.T) {
	pod := &corev1.Pod{Status: corev1.PodStatus{
		Phase: corev1.PodPending,
		InitContainerStatuses: []corev1.ContainerStatus{{
			Name: "setup",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason:  "ErrImagePull",
				Message: "pull https://registry.example/private/init?token=kandev_pat_supersecret",
			}},
		}},
	}}

	ready, err := kubernetesPodReady(pod, "kandev-agent")

	require.False(t, ready)
	require.ErrorContains(t, err, "init container setup image pull failed")
	require.ErrorContains(t, err, "ErrImagePull")
	require.NotContains(t, err.Error(), "/private/init")
	require.NotContains(t, err.Error(), "supersecret")
}

func TestKubernetesWaitForPodRunningFailsPromptlyWhenPodIsDeleted(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "agent", Namespace: "agents", ResourceVersion: "1",
	}, Status: corev1.PodStatus{
		Phase: corev1.PodPending,
	}}
	clientset := kubefake.NewSimpleClientset(pod)
	podWatch := watch.NewRaceFreeFake()
	clientset.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, podWatch, nil
	})
	resources := &liveKubernetesResources{client: &kubeexecutor.Client{Clientset: clientset}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go podWatch.Delete(pod.DeepCopy())

	_, err := resources.WaitForPodRunning(ctx, "agents", "agent", "kandev-agent")

	require.ErrorContains(t, err, "deleted")
	require.NotErrorIs(t, err, context.DeadlineExceeded)
}

func TestKubernetesCreateBoundsPodWaitByConfiguredRequestTimeoutAndKeepsDiagnostic(t *testing.T) {
	deadlineSeen := make(chan time.Time, 1)
	resources := &fakeKubernetesResources{}
	resources.waitForPodRunning = func(ctx context.Context, _, _, _ string) (*corev1.Pod, error) {
		if deadline, ok := ctx.Deadline(); ok {
			deadlineSeen <- deadline
		}
		select {
		case <-ctx.Done():
			return nil, kubernetesPodWaitError(ctx, ctx.Err(), errors.New(
				"pod scheduling failed: Unschedulable: no nodes match kubernetes.io/arch",
			))
		case <-time.After(1500 * time.Millisecond):
			return nil, errors.New("Pod wait context was unbounded")
		}
	}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	req := validKubernetesCreateRequest()
	req.Metadata[MetadataKeyKubernetesRequestTimeoutSeconds] = "1"
	started := time.Now()

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "Unschedulable")
	require.ErrorContains(t, err, "kubernetes.io/arch")
	require.Less(t, time.Since(started), 1400*time.Millisecond)
	select {
	case deadline := <-deadlineSeen:
		require.WithinDuration(t, started.Add(time.Second), deadline, 250*time.Millisecond)
	default:
		t.Fatal("configured Pod-wait deadline was not applied")
	}
}

func TestKubernetesRemoteStatusSanitizesConditionAndClientErrors(t *testing.T) {
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
	resources.pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "kandev-agent",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
			Reason:  "ErrImagePull",
			Message: "pull https://registry.example/private?token=kandev_pat_supersecret",
		}},
	}}
	resources.mu.Unlock()

	status, err := executor.GetRemoteStatus(context.Background(), instance)

	require.NoError(t, err)
	require.Equal(t, "ErrImagePull", status.Details["reason"])
	message := status.Details["message"].(string)
	require.NotContains(t, message, "/private")
	require.NotContains(t, message, "supersecret")

	resources.mu.Lock()
	resources.getPodErr = errors.New("GET https://cluster.example/api/v1/pods?token=kandev_pat_supersecret")
	resources.mu.Unlock()
	_, err = executor.GetRemoteStatus(context.Background(), instance)
	require.Error(t, err)
	require.False(t, strings.Contains(err.Error(), "/api/v1/pods"), err.Error())
	require.NotContains(t, err.Error(), "supersecret")
}
