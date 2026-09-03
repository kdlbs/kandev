package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
)

func TestKubernetesCreateInstanceRollsBackAdmissionMutatedPodByCreatedUID(t *testing.T) {
	resources := &fakeKubernetesResources{
		mutateCreatedPod: func(pod *corev1.Pod) {
			pod.Labels["kandev.ai/instance-id"] = "mutated-by-admission"
		},
	}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)

	_, err := executor.CreateInstance(context.Background(), validKubernetesCreateRequest())

	require.ErrorContains(t, err, "ownership label")
	require.Equal(t, []string{"kandev-6212a06cac8b50f7:pod-uid"}, resources.deletedPods,
		"the exact UID returned by Create authorizes rollback even when admission changed labels")
}

func TestKubernetesCreateInstanceRollsBackAdmissionMutatedPVCByCreatedUID(t *testing.T) {
	resources := &fakeKubernetesResources{
		mutateCreatedPVC: func(pvc *corev1.PersistentVolumeClaim) {
			pvc.Labels["kandev.ai/instance-id"] = "mutated-by-admission"
		},
	}
	executor := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, nil)
	req := validKubernetesCreateRequest()
	setManagedKubernetesWorkspace(req)

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "ownership label")
	require.Equal(t, []string{"kandev-6212a06cac8b50f7-workspace:pvc-uid"}, resources.deletedPVCs)
	require.Empty(t, resources.createdPods)
}

func TestKubernetesReconnectRejectsExtraKandevOwnershipLabel(t *testing.T) {
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
	resources.pod.Labels["kandev.ai/forged"] = "true"
	resources.mu.Unlock()
	restarted := newFakeKubernetesExecutor(t, resources, &recordingKubernetesExec{}, map[uint16]uint16{41001: instancePort})

	_, err = restarted.CreateInstance(context.Background(), kubernetesReconnectRequest(created))

	require.ErrorContains(t, err, "ownership label")
}

func TestKubernetesCleanupRejectsExtraKandevOwnershipLabels(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeKubernetesResources)
	}{
		{name: "Pod", mutate: func(resources *fakeKubernetesResources) {
			resources.pod.Labels["kandev.ai/forged"] = "true"
		}},
		{name: "managed PVC", mutate: func(resources *fakeKubernetesResources) {
			resources.pvc.Labels["kandev.ai/forged"] = "true"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			test.mutate(resources)
			resources.mu.Unlock()

			err = executor.StopInstance(context.Background(), instance, true)

			require.ErrorContains(t, err, "ownership label")
			require.Empty(t, resources.deletedPods)
			require.Empty(t, resources.deletedPVCs)
		})
	}
}

func TestVerifyCreatedKubernetesPodRejectsAdmissionMutatedAgentctlFields(t *testing.T) {
	req := validKubernetesCreateRequest()
	profile, err := kubernetesProfileConfigFromMetadata(req.Metadata)
	require.NoError(t, err)
	identity, err := kubernetesIdentity(req)
	require.NoError(t, err)
	template, err := kubeexecutor.ParsePodTemplate(profile.PodTemplateYAML)
	require.NoError(t, err)
	desired, _, err := kubeexecutor.ComposePod(template, profile, kubeexecutor.PodOptions{
		Name: "pod-1", Namespace: "kandev-agents", Identity: identity,
		Command: []string{"sh", "-c"}, Args: []string{kubernetesBootstrapCommand()},
		WorkingDir: kubernetesWorkspacePath, AgentctlPort: kubeexecutor.DefaultAgentctlPort,
	})
	require.NoError(t, err)

	t.Run("port", func(t *testing.T) {
		created := desired.DeepCopy()
		created.UID = "pod-uid"
		main := findKubernetesContainer(created, profile.MainContainer)
		require.NotNil(t, main)
		main.Ports[len(main.Ports)-1].ContainerPort = 9999

		err := verifyCreatedPod(created, desired, identity, profile.MainContainer)

		require.ErrorContains(t, err, "agentctl port")
	})

	t.Run("reserved environment", func(t *testing.T) {
		created := desired.DeepCopy()
		created.UID = "pod-uid"
		main := findKubernetesContainer(created, profile.MainContainer)
		require.NotNil(t, main)
		main.Env = append(main.Env, corev1.EnvVar{Name: "AGENTCTL_PORT", Value: "9999"})

		err := verifyCreatedPod(created, desired, identity, profile.MainContainer)

		require.ErrorContains(t, err, "reserved environment")
	})
}

func TestKubernetesCreateInstanceMaterializesSkillsCredentialsAndPortableConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".mock-agent"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte("auth-data"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".mock-agent", "settings.json"), []byte("settings-data"), 0o600))

	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	execs := &recordingKubernetesExec{}
	executor := NewKubernetesExecutor(nil, newTestLogger())
	executor.clientFactory = func(kubeexecutor.ExecutorConfig) (*kubernetesRuntimeClient, error) {
		return &kubernetesRuntimeClient{
			resources: &fakeKubernetesResources{},
			streams: kubeexecutor.NewStreamOperations(execs, &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
				uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
				41001:                                    instancePort,
			}}),
		}, nil
	}
	executor.resolveBinary = func(kubeexecutor.Platform) ([]byte, error) { return []byte("agentctl"), nil }
	req := validKubernetesCreateRequest()
	req.AgentConfig = agents.NewMockAgentWithID("codex-acp", "Mock Codex", "Mock Codex")
	req.Metadata[MetadataKeyRemoteAuthHome] = "/home/untrusted-task-value"
	req.Metadata["remote_credentials"] = `["agent:codex-acp:files:0"]`
	req.Metadata[MetadataKeyAgentConfigBundles] = `["codex-acp.settings"]`
	req.Metadata[MetadataKeySkillManifestJSON] = `{
		"Skills":[{"Slug":"review","Content":"# Review","Files":[{"Path":"refs/checklist.md","Content":"check"}]}],
		"Instructions":[{"Filename":"AGENTS.md","Content":"office rules","IsEntry":true}],
		"AgentTypeID":"codex-acp","WorkspaceSlug":"office","AgentID":"profile-1","ProjectSkillDir":".agents/skills"
	}`

	_, err := executor.CreateInstance(context.Background(), req)

	require.NoError(t, err)
	requireKubernetesUploadedFile(t, execs, "/workspace/.agents/skills/kandev-review/SKILL.md", "# Review")
	requireKubernetesUploadedFile(t, execs, "/workspace/.agents/skills/kandev-review/refs/checklist.md", "check")
	requireKubernetesUploadedFile(t, execs, "/opt/kandev/runtime/office/instructions/profile-1/AGENTS.md", "office rules")
	requireKubernetesUploadedFile(t, execs, "/run/kandev/home/.codex/auth.json", "auth-data")
	requireKubernetesUploadedFile(t, execs, "/run/kandev/home/.mock-agent/settings.json", "settings-data")
	requireKubernetesPathNotUsed(t, execs, "/home/untrusted-task-value")
}
