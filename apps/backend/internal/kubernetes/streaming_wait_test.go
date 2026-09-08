package kubernetes

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestWaitForStreamingProbePodAllowsSchedulerAssignedNodeName(t *testing.T) {
	admitted := streamingWaitProbePod(t)
	scheduled := admitted.DeepCopy()
	scheduled.Spec.NodeName = "kind-worker"
	markStreamingProbeRunning(scheduled)
	clientset := kubernetesfake.NewSimpleClientset(scheduled)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	running, err := waitForStreamingProbePod(
		ctx, clientset, admitted.Namespace, admitted.Name, "kandev-agent", admitted,
	)

	require.NoError(t, err)
	require.Equal(t, "kind-worker", running.Spec.NodeName)
}

func TestWaitForStreamingProbePodRejectsOwnedMutationAfterScheduling(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{
			name: "ownership label",
			mutate: func(pod *corev1.Pod) {
				pod.Labels["kandev.ai/session-id"] = "foreign"
			},
		},
		{
			name: "main bootstrap",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.Containers[0].Command = []string{"false"}
			},
		},
		{
			name: "platform selector",
			mutate: func(pod *corev1.Pod) {
				pod.Spec.NodeSelector[corev1.LabelOSStable] = "windows"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			admitted := streamingWaitProbePod(t)
			scheduled := admitted.DeepCopy()
			scheduled.Spec.NodeName = "kind-worker"
			test.mutate(scheduled)
			markStreamingProbeRunning(scheduled)
			clientset := kubernetesfake.NewSimpleClientset(scheduled)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err := waitForStreamingProbePod(
				ctx, clientset, admitted.Namespace, admitted.Name, "kandev-agent", admitted,
			)

			require.ErrorContains(t, err, "identity changed")
		})
	}
}

func streamingWaitProbePod(t *testing.T) *corev1.Pod {
	t.Helper()
	profile, err := agentkubernetes.ParseProfileConfig(validHandlerProfileConfig())
	require.NoError(t, err)
	pod, err := buildStreamingProbePod("kandev", "probe-pod", profile)
	require.NoError(t, err)
	pod.UID = types.UID("probe-uid")
	pod.CreationTimestamp = metav1.Now()
	pod.Status.Phase = corev1.PodPending
	return pod
}

func markStreamingProbeRunning(pod *corev1.Pod) {
	pod.Status.Phase = corev1.PodRunning
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "kandev-agent",
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()},
		},
	}}
}
