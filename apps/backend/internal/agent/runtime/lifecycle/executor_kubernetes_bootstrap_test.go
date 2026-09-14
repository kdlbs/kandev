package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/githubauth"
)

func TestKubernetesWriteFileDoesNotOpenStdinForEmptySentinel(t *testing.T) {
	execs := &recordingKubernetesExec{}
	streams := kubeexecutor.NewStreamOperations(execs, nil)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kandev-agents", Name: "agent-pod"}}

	err := kubernetesWriteFile(
		context.Background(), streams, pod, "kandev-agent", kubernetesStartPath, nil, 0o600,
	)

	require.NoError(t, err)
	require.Len(t, execs.requests, 1)
	require.Nil(t, execs.requests[0].request.Stdin,
		"empty start sentinel must not enable remotecommand stdin streaming")
	command := strings.Join(execs.requests[0].request.Command, " ")
	require.Contains(t, command, "temporary=$(mktemp")
	require.Contains(t, command, "mv -f \"$temporary\" \"$destination\"")
}

func TestKubernetesWriteFileCommandRejectsSymlinkedAncestor(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "redirect")))
	destination := filepath.ToSlash(filepath.Join(root, "redirect", "secret.json"))
	command := exec.Command("sh", "-c", kubernetesWriteFileCommand(destination, 0o600, true))
	command.Stdin = strings.NewReader("secret")

	err := command.Run()

	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(outside, "secret.json"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestKubernetesPodFileUploaderNormalizesWindowsSeparators(t *testing.T) {
	execs := &recordingKubernetesExec{}
	uploader := kubernetesPodFileUploader{
		streams: kubeexecutor.NewStreamOperations(execs, nil),
		pod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: "kandev-agents", Name: "agent-pod",
		}},
		container: "kandev-agent",
	}

	err := uploader.WriteFile(
		context.Background(), `\run\kandev\home\.codex\auth.json`, []byte("secret"), 0o600,
	)

	require.NoError(t, err)
	require.Len(t, execs.requests, 1)
	remoteCommand := strings.Join(execs.requests[0].request.Command, " ")
	require.Contains(t, remoteCommand, "/run/kandev/home/.codex/auth.json")
	require.NotContains(t, remoteCommand, `\run\kandev`)
}

func TestKubernetesBootstrapOwnsControlEnvironmentAndPreservesOfficeEnvironment(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	execs := &recordingKubernetesExec{}
	executor := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, execs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	req.AgentProfileID = "execution-profile"
	req.OfficeAgentProfileID = "office-profile"
	req.Env = map[string]string{
		"HOME":                        "/tmp/attacker-home",
		"AGENTCTL_AUTH_TOKEN":         "attacker-token",
		"AGENTCTL_BOOTSTRAP_NONCE":    "attacker-nonce",
		"AGENTCTL_LISTEN_HOST":        "0.0.0.0",
		"AGENTCTL_PORT":               "9999",
		"AGENTCTL_FUTURE_CONTROL":     "attacker-control",
		"KANDEV_INSTANCE_ID":          "attacker-instance",
		"KANDEV_EXECUTION_ID":         "attacker-execution",
		"KANDEV_TASK_ID":              "attacker-task",
		"KANDEV_SESSION_ID":           "attacker-session",
		"KANDEV_TASK_ENVIRONMENT_ID":  "attacker-environment",
		"KANDEV_AGENT_PROFILE_ID":     "attacker-agent-profile",
		"KANDEV_EXECUTION_PROFILE_ID": "attacker-execution-profile",
		"KANDEV_API_KEY":              "office-api-key",
		"KANDEV_CLI":                  "/opt/kandev/bin/kandev",
		"KANDEV_RUN_ID":               "run-office-1",
	}

	instance, err := executor.CreateInstance(context.Background(), req)

	require.NoError(t, err)
	runtimeData := string(kubernetesRecordedUpload(t, execs, kubernetesRuntimeEnvPath))
	require.Contains(t, runtimeData, "KANDEV_INSTANCE_ID='instance-1'")
	require.Contains(t, runtimeData, "KANDEV_TASK_ID='task-1'")
	require.Contains(t, runtimeData, "KANDEV_SESSION_ID='session-1'")
	require.Contains(t, runtimeData, "KANDEV_TASK_ENVIRONMENT_ID='environment-1'")
	require.Contains(t, runtimeData, "KANDEV_AGENT_PROFILE_ID='office-profile'")
	require.Contains(t, runtimeData, "KANDEV_EXECUTION_PROFILE_ID='execution-profile'")
	require.NotContains(t, runtimeData, "KANDEV_EXECUTION_ID")

	authData := string(kubernetesRecordedUpload(t, execs, kubernetesAuthEnvPath))
	require.Contains(t, authData, "HOME='/run/kandev/home'")
	require.Contains(t, authData, "AGENTCTL_BOOTSTRAP_NONCE='"+instance.BootstrapNonce+"'")
	require.Contains(t, authData, "KANDEV_API_KEY='office-api-key'")
	require.Contains(t, authData, "KANDEV_CLI='/opt/kandev/bin/kandev'")
	require.Contains(t, authData, "KANDEV_RUN_ID='run-office-1'")
	for _, hostile := range []string{
		"attacker-token", "attacker-nonce", "0.0.0.0", "9999", "attacker-control",
		"attacker-instance", "attacker-execution", "attacker-task", "attacker-session",
		"attacker-environment", "attacker-agent-profile", "attacker-execution-profile",
	} {
		require.NotContains(t, authData, hostile)
	}
}

func TestKubernetesBootstrapPublishesManagedCredentialHelperPath(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	execs := &recordingKubernetesExec{}
	executor := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, execs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})
	req := validKubernetesCreateRequest()
	req.Env = map[string]string{
		githubauth.CredentialBrokerURLEnv:  "https://broker.example/resolve",
		githubauth.CredentialLeaseEnv:      "opaque-lease",
		githubauth.CredentialHelperPathEnv: "/tmp/hostile-agentctl",
	}

	_, err := executor.CreateInstance(context.Background(), req)

	require.NoError(t, err)
	authData := string(kubernetesRecordedUpload(t, execs, kubernetesAuthEnvPath))
	require.Contains(t, authData,
		githubauth.CredentialHelperPathEnv+"='/opt/kandev/agentctl'",
	)
	require.NotContains(t, authData, "/tmp/hostile-agentctl")
	command := kubernetesBootstrapCommand()
	require.Less(t, strings.Index(command, ". "+kubernetesAuthEnvPath), strings.Index(command, "sh "+kubernetesPreparePath))
	require.Less(t, strings.Index(command, ". "+kubernetesAuthEnvPath), strings.Index(command, "exec "+kubernetesAgentctlPath))
}

func TestKubernetesPrepareScriptUsesManagedAgentctlPathWithoutStartingSecondProcess(t *testing.T) {
	req := validKubernetesCreateRequest()
	req.Metadata[MetadataKeySetupScript] = "{{kandev.agentctl.install}}\n{{kandev.agentctl.start}}\n"

	script, err := kubernetesPrepareScript(req)

	require.NoError(t, err)
	require.Contains(t, script, "chmod +x '/opt/kandev/agentctl'")
	require.NotContains(t, script, "/usr/local/bin/agentctl")
	require.NotContains(t, script, "nohup agentctl")
}

func TestKubernetesDefaultPrepareScriptSupportsRepositorylessAgentInstall(t *testing.T) {
	req := validKubernetesCreateRequest()
	req.AgentConfig = &stubAgent{
		MockAgent: agents.NewMockAgent(), id: "office-agent",
		installScript: "printf 'managed-agent-installed\\n'",
	}

	script, err := kubernetesPrepareScript(req)

	require.NoError(t, err)
	require.NotContains(t, script, "{{repository.clone_url}}")
	require.NotContains(t, script, "{{kandev.agents.install}}")
	require.Contains(t, script, "repository_url=''", "repo-less launch must resolve an explicit empty repository")
	require.Contains(t, script, `if [ -n "$repository_url" ]; then`, "clone must be guarded for repo-less launches")
	require.Contains(t, script, "managed-agent-installed")
}

func TestKubernetesBootstrapCreatesPrivateAuthHomeWithoutCredentialFiles(t *testing.T) {
	controlPort := startKubernetesAgentctlServer(t, true, 41001)
	instancePort := startKubernetesAgentctlServer(t, false, 0)
	execs := &recordingKubernetesExec{}
	executor := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, execs, map[uint16]uint16{
		uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
		41001:                                    instancePort,
	})

	_, err := executor.CreateInstance(context.Background(), validKubernetesCreateRequest())

	require.NoError(t, err)
	execs.mu.Lock()
	defer execs.mu.Unlock()
	homeIndex, authIndex := -1, -1
	for index, recorded := range execs.requests {
		command := strings.Join(recorded.request.Command, " ")
		if strings.Contains(command, kubernetesAuthHomePath) && strings.Contains(command, "chmod 0700") {
			homeIndex = index
		}
		if strings.Contains(command, kubernetesAuthEnvPath) {
			authIndex = index
		}
	}
	require.NotEqual(t, -1, homeIndex, "bootstrap must create the fixed auth HOME even without config bundles")
	require.NotEqual(t, -1, authIndex)
	require.Less(t, homeIndex, authIndex, "auth HOME must exist before auth.env is written")
}

func TestKubernetesSerializeEnvironmentPreservesExplicitEmptyValues(t *testing.T) {
	serialized, err := kubernetesSerializeEnvironment(map[string]string{
		"FOO": "",
		"BAR": "value",
	})

	require.NoError(t, err)
	require.Equal(t, "BAR='value'\nFOO=''\n", string(serialized))
}

func TestKubernetesSkillManifestRejectsDotInstructionIdentity(t *testing.T) {
	execs := &recordingKubernetesExec{}
	executor := newFakeKubernetesExecutor(t, &fakeKubernetesResources{}, execs, nil)
	req := validKubernetesCreateRequest()
	req.Metadata[MetadataKeySkillManifestJSON] = `{
		"Instructions":[{"Filename":"agentctl","Content":"overwrite"}],
		"WorkspaceSlug":"..","AgentID":"..","ProjectSkillDir":".agents/skills"
	}`

	_, err := executor.CreateInstance(context.Background(), req)

	require.ErrorContains(t, err, "unsafe path component")
	for _, request := range execs.requests {
		require.NotEqual(t, "overwrite", string(request.stdin),
			"manifest traversal must not overwrite /opt/kandev/agentctl")
	}
}

func kubernetesRecordedUpload(t *testing.T, execs *recordingKubernetesExec, destination string) []byte {
	t.Helper()
	execs.mu.Lock()
	defer execs.mu.Unlock()
	for _, recorded := range execs.requests {
		if strings.Contains(strings.Join(recorded.request.Command, " "), destination) {
			return append([]byte(nil), recorded.stdin...)
		}
	}
	t.Fatalf("upload for %s not found", destination)
	return nil
}
