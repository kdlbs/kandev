package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestKubernetesCreateInstanceReconnectsExistingPodWithoutParsingCurrentProfile(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	created, err := initial.CreateInstance(context.Background(), validKubernetesCreateRequest())
	require.NoError(t, err)
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		41001: instancePort,
	})
	reconnectRequest := kubernetesReconnectRequest(created)
	reconnectRequest.Metadata[MetadataKeyKubernetesProfilePlatform] = "not-a-platform"
	reconnectRequest.Metadata[MetadataKeyKubernetesPodTemplateYAML] = "not valid PodTemplate YAML"

	reconnected, err := restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, created.Metadata[MetadataKeyKubernetesPodUID], reconnected.Metadata[MetadataKeyKubernetesPodUID])
	require.Len(t, resources.createdPods, 1)
}

func TestKubernetesProfileSnapshotExcludesResolvedRuntimeMaterial(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	executor := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	req.Env = map[string]string{"OPENAI_API_KEY": "runtime-secret-marker"}
	req.Metadata[MetadataKeySetupScript] = "echo setup-script-marker"
	req.Metadata[MetadataKeyRepoSetupScript] = "echo repo-script-marker"
	req.Metadata[MetadataKeyAgentConfigBundles] = "portable-config-marker"

	instance, err := executor.CreateInstance(context.Background(), req)

	require.NoError(t, err)
	snapshot := instance.Metadata[MetadataKeyKubernetesProfileSnapshot].(string)
	for _, forbidden := range []string{
		"runtime-secret-marker", "setup-script-marker", "repo-script-marker", "portable-config-marker",
	} {
		require.False(t, strings.Contains(snapshot, forbidden), "snapshot leaked %q", forbidden)
	}
}

func TestKubernetesCreateInstanceRebuildsLostPodFromRecordedSnapshotAfterProfileChange(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	initialRequest := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(initialRequest)
	created, err := initial.CreateInstance(context.Background(), initialRequest)
	require.NoError(t, err)
	resources.mu.Lock()
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
	reconnectRequest.Metadata[MetadataKeyKubernetesPodTemplateYAML] = "not valid PodTemplate YAML"

	reconnected, err := restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, "replacement-pod-uid", reconnected.Metadata[MetadataKeyKubernetesPodUID])
	require.Len(t, resources.createdPods, 2)
	require.Equal(t, "amd64", resources.createdPods[1].Spec.NodeSelector[corev1.LabelArchStable],
		"replacement must use the recorded launch platform, not the edited profile")
	require.Equal(t, "example.test/agent:latest", resources.createdPods[1].Spec.Containers[0].Image,
		"replacement must use the recorded Pod template")
}

func TestKubernetesCreateInstanceRebuildsLostPodWhenCurrentProfileIsUnavailable(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	initialRequest := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(initialRequest)
	created, err := initial.CreateInstance(context.Background(), initialRequest)
	require.NoError(t, err)
	resources.mu.Lock()
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
	for _, key := range kubernetesProfileMetadataKeys {
		delete(reconnectRequest.Metadata, key)
	}

	reconnected, err := restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	require.Equal(t, "replacement-pod-uid", reconnected.Metadata[MetadataKeyKubernetesPodUID])
	require.Len(t, resources.createdPods, 2)
}

func TestKubernetesReplacementBootstrapUsesRecordedEnvironmentIdentity(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	resources := &fakeKubernetesResources{}
	initial := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	initialRequest := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(initialRequest)
	created, err := initial.CreateInstance(context.Background(), initialRequest)
	require.NoError(t, err)
	resources.mu.Lock()
	resources.pod = nil
	resources.nextPodUID = "replacement-pod-uid"
	resources.mu.Unlock()

	restartControlPort := startKubernetesAgentctlServer(t, true, 41002)
	restartInstancePort := startKubernetesAgentctlServer(t, false, 0)
	replacementExecs := &recordingKubernetesExec{}
	restarted := newFakeKubernetesExecutor(t, resources, replacementExecs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): restartControlPort,
		41002:                                    restartInstancePort,
	})
	reconnectRequest := kubernetesReconnectRequest(created)
	reconnectRequest.TaskEnvironmentID = "drifted-environment"

	_, err = restarted.CreateInstance(context.Background(), reconnectRequest)

	require.NoError(t, err)
	replacementExecs.mu.Lock()
	defer replacementExecs.mu.Unlock()
	var bootstrapData string
	for _, exec := range replacementExecs.requests {
		if strings.Contains(strings.Join(exec.request.Command, " "), kubernetesRuntimeEnvPath) {
			bootstrapData = string(exec.stdin)
			break
		}
	}
	require.Contains(t, bootstrapData, "KANDEV_TASK_ENVIRONMENT_ID='environment-1'")
	require.NotContains(t, bootstrapData, "drifted-environment")
}

func TestKubernetesReplacementRollbackUsesBoundedExecutorTimeout(t *testing.T) {
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

	resources.mu.Lock()
	resources.pod = nil
	resources.nextPodUID = "replacement-pod-uid"
	resources.deletePodErr = errors.New("rollback delete failed")
	resources.mu.Unlock()
	restarted := newFakeKubernetesExecutor(
		t, resources, &recordingKubernetesExec{err: errors.New("replacement bootstrap failed")}, nil,
	)
	reconnectRequest := kubernetesReconnectRequest(created)
	reconnectRequest.Metadata[MetadataKeyKubernetesRequestTimeoutSeconds] = "1"

	_, err = restarted.CreateInstance(context.Background(), reconnectRequest)

	require.ErrorContains(t, err, "replacement bootstrap failed")
	resources.mu.Lock()
	deadline := resources.deletePodContextDeadline
	resources.mu.Unlock()
	require.False(t, deadline.IsZero(), "replacement rollback must not inherit an unbounded detached context")
	require.WithinDuration(t, time.Now().Add(time.Second), deadline, 250*time.Millisecond)
}
