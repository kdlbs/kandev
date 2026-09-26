package lifecycle

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kandev/kandev/internal/agent/agents"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/stretchr/testify/require"
)

func TestSSHInitialModeIsInstalledAtExportedConfigPath(t *testing.T) {
	remoteHome := t.TempDir()
	server := newFakeSSHServer(t, nil)
	server.enableSFTP()
	executor := &SSHExecutor{logger: newTestLogger()}
	request := &ExecutorCreateRequest{
		InstanceID:  "execution-1",
		AgentConfig: agents.NewClaudeACP(),
		InitialMode: &AgentInitialModeRequest{Mode: "bypassPermissions"},
		Env:         map[string]string{},
		Metadata: map[string]interface{}{
			MetadataKeyRemoteAuthHome: remoteHome,
		},
	}

	require.NoError(t, executor.uploadCredentials(context.Background(), server.dial(t), request, SSHRemotePlatform{}))

	expectedConfigDir := filepath.Join(remoteHome, ".kandev", "agent-sessions", request.InstanceID, ".claude")
	require.Equal(t, expectedConfigDir, request.Env["CLAUDE_CONFIG_DIR"])
	require.True(t, request.InitialMode.Delivered)
	require.Equal(t, expectedConfigDir, request.InitialMode.ConfigDir)
	settings, err := os.ReadFile(filepath.Join(expectedConfigDir, "settings.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"permissions":{"defaultMode":"bypassPermissions"}}`, string(settings))
}

func TestKubernetesInitialModeIsInstalledAtExportedConfigPath(t *testing.T) {
	request := validKubernetesCreateRequest()
	request.Metadata[metadataKubernetesTaskOwned] = true
	request.AgentConfig = agents.NewClaudeACP()
	request.InitialMode = &AgentInitialModeRequest{Mode: "bypassPermissions"}
	request.Env = map[string]string{}

	execs := &recordingKubernetesExec{}
	streams := kubeexecutor.NewStreamOperations(execs, nil)
	runtimeClient := &kubernetesRuntimeClient{streams: streams}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kandev-agents", Name: "task-pod"}}
	executor := NewKubernetesExecutor(nil, newTestLogger())
	uploader := kubernetesPodFileUploader{streams: streams, pod: pod, container: "kandev-agent"}

	require.NoError(t, executor.materializeKubernetesCredentials(
		context.Background(), uploader, runtimeClient, request, pod, "kandev-agent",
	))

	expectedConfigDir := path.Join(kubernetesSessionHome(request), ".claude")
	require.Equal(t, expectedConfigDir, kubernetesSessionEnvironment(request)["CLAUDE_CONFIG_DIR"])
	require.True(t, request.InitialMode.Delivered)
	require.Equal(t, expectedConfigDir, request.InitialMode.ConfigDir)
	settingsPath := path.Join(expectedConfigDir, "settings.json")
	found := false
	for _, recorded := range execs.requests {
		command := strings.Join(recorded.request.Command, " ")
		if strings.Contains(command, settingsPath) {
			require.NotNil(t, recorded.request.Stdin)
			require.NotEmpty(t, recorded.stdin)
			require.JSONEq(t, `{"permissions":{"defaultMode":"bypassPermissions"}}`, string(recorded.stdin))
			found = true
		}
	}
	require.True(t, found, "Kubernetes settings file was not transferred to %s", settingsPath)
}

func TestInitialModePathMismatchDoesNotConfirmDelivery(t *testing.T) {
	request := &ExecutorCreateRequest{
		AgentConfig: agents.NewClaudeACP(),
		InitialMode: &AgentInitialModeRequest{Mode: "bypassPermissions"},
	}

	err := markInitialModeDelivered(request, "/session/.claude", "/root/.claude")
	require.ErrorContains(t, err, "does not match exported agent config directory")
	require.False(t, request.InitialMode.Delivered)
	require.Empty(t, request.InitialMode.ConfigDir)
	require.NotEmpty(t, request.InitialMode.Reason)
}
