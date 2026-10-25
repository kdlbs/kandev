package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestKubernetesWorkspaceGrantUsesLaunchSnapshot(t *testing.T) {
	t.Run("retains recorded grant", func(t *testing.T) { assertKubernetesWorkspaceGrantSnapshot(t, true) })
	t.Run("profile cannot add grant", func(t *testing.T) { assertKubernetesWorkspaceGrantSnapshot(t, false) })
}

func assertKubernetesWorkspaceGrantSnapshot(t *testing.T, initialGrant bool) {
	t.Helper()
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	initialRequest := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(initialRequest)
	raw := initialRequest.Metadata[MetadataKeyKubernetesPodTemplateYAML].(string)
	template, err := kubeexecutor.ParsePodTemplate(raw)
	require.NoError(t, err)
	grant := corev1.Container{Name: "docker-engine", Image: "example/daemon:v1", VolumeMounts: []corev1.VolumeMount{{Name: kubeexecutor.WorkspaceVolumeName, MountPath: kubeexecutor.WorkspaceMountPath, ReadOnly: true}}}
	if initialGrant {
		template.Template.Spec.Containers = append(template.Template.Spec.Containers, grant)
	}
	encoded, err := json.Marshal(template)
	require.NoError(t, err)
	initialRequest.Metadata[MetadataKeyKubernetesPodTemplateYAML] = string(encoded)
	created, err := initial.CreateInstance(context.Background(), initialRequest)
	require.NoError(t, err)
	resources.mu.Lock()
	originalVolumes := resources.pod.DeepCopy().Spec.Volumes
	originalClaimUID := resources.pvc.UID
	resources.pod = nil
	resources.nextPodUID = "replacement-pod-uid"
	resources.mu.Unlock()

	restartControlPort := startKubernetesAgentctlServer(t, true, 41002)
	restartInstancePort := startKubernetesAgentctlServer(t, false, 0)
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): restartControlPort,
		41002:                                    restartInstancePort,
	})
	reconnectRequest := kubernetesReconnectRequest(created)
	setManagedKubernetesWorkspace(reconnectRequest)
	reconnectRequest.Metadata[MetadataKeyKubernetesProfilePlatform] = "not-a-platform"
	if initialGrant {
		template.Template.Spec.Containers = template.Template.Spec.Containers[:1]
	} else {
		template.Template.Spec.Containers = append(template.Template.Spec.Containers, grant)
	}
	edited, err := json.Marshal(template)
	require.NoError(t, err)
	reconnectRequest.Metadata[MetadataKeyKubernetesPodTemplateYAML] = string(edited)

	reconnected, err := restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, "replacement-pod-uid", reconnected.Metadata[MetadataKeyKubernetesPodUID])
	require.Len(t, resources.createdPods, 2)
	require.Equal(t, "amd64", resources.createdPods[1].Spec.NodeSelector[corev1.LabelArchStable],
		"replacement must use the recorded launch platform, not the edited profile")
	require.Equal(t, "example.test/agent:latest", resources.createdPods[1].Spec.Containers[0].Image,
		"replacement must use the recorded Pod template")
	if initialGrant {
		require.Equal(t, grant, resources.createdPods[1].Spec.Containers[1])
	} else {
		require.Len(t, resources.createdPods[1].Spec.Containers, 1)
	}
	require.Equal(t, originalVolumes, resources.createdPods[1].Spec.Volumes)
	require.Equal(t, originalClaimUID, resources.pvc.UID)
}
