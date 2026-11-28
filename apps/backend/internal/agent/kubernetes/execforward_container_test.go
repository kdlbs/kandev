package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func composeSidecarPod(t *testing.T, mainContainer string) *corev1.Pod {
	t.Helper()
	template, err := ParsePodTemplate("apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n" +
		"      - name: istio-proxy\n        image: example/proxy:v1\n        ports:\n" +
		"          - name: http-envoy-prom\n            containerPort: 15090\n" +
		"      - name: " + mainContainer + "\n        image: example/agent:v1\n" +
		"      - name: docker-engine\n        image: example/docker:v1\n")
	require.NoError(t, err)
	profile := validProfile("")
	profile.MainContainer = mainContainer
	pod, _, err := ComposePod(template, profile, podOptions())
	require.NoError(t, err)
	return pod
}

// TestAgentctlContainerSelectsMainContainerAmongSidecars composes a real
// session Pod whose first container is a mesh sidecar and checks that the
// exec transport targets the configured main container, by default and by a
// custom name.
func TestAgentctlContainerSelectsMainContainerAmongSidecars(t *testing.T) {
	for _, mainContainer := range []string{DefaultMainContainerName, "agent"} {
		pod := composeSidecarPod(t, mainContainer)
		require.Equal(t, "istio-proxy", pod.Spec.Containers[0].Name, "the sidecar comes first")

		container, err := AgentctlContainer(pod)

		require.NoError(t, err)
		require.Equal(t, mainContainer, container)
	}
}

func TestAgentctlContainerFailsWithoutExactlyOneMainContainer(t *testing.T) {
	_, err := AgentctlContainer(nil)
	require.Error(t, err)

	pod := composeSidecarPod(t, DefaultMainContainerName)
	pod.Spec.Containers[1].Ports = nil
	_, err = AgentctlContainer(pod)
	require.ErrorContains(t, err, "has no main container with the kandev-agentctl port")

	pod = composeSidecarPod(t, DefaultMainContainerName)
	pod.Spec.Containers[0].Ports = append(pod.Spec.Containers[0].Ports,
		corev1.ContainerPort{Name: AgentctlContainerPort, ContainerPort: 9000})
	_, err = AgentctlContainer(pod)
	require.ErrorContains(t, err, "several containers with the kandev-agentctl port: istio-proxy, kandev-agent")
}

func TestStreamOperationsForwardsOverExecOnlyForExecForwarder(t *testing.T) {
	executor := &fakeBridgeExecutor{}
	exec := NewStreamOperations(executor, &ExecPortForwarder{Executor: executor, BridgePath: "/opt/kandev/agentctl"})
	require.True(t, exec.ForwardsOverExec())
	require.False(t, NewStreamOperations(executor, nil).ForwardsOverExec())

	_, err := exec.Forward(context.Background(), PortForwardRequest{
		Namespace: "kandev-agents", Pod: "session-pod", RemotePort: 8765,
	})
	require.ErrorContains(t, err, "main (agentctl) container")
}
