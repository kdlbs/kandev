package lifecycle

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func TestAgentctlProviderMappingsPreserveProviderCapabilities(t *testing.T) {
	providersByCase := map[string][]string{
		"github-only":          {"github"},
		"gitlab-only":          {"gitlab"},
		"mixed":                {"github", "gitlab"},
		"empty":                {},
		"unsupported-filtered": {},
	}

	for name, providers := range providersByCase {
		providers := providers
		t.Run(name, func(t *testing.T) {
			req := &ExecutorCreateRequest{McpProviders: providers}
			standalone := buildStandaloneCreateInstanceRequest(req, nil, "", false, false, false, false, nil)
			container := buildContainerCreateInstanceRequest(ContainerConfig{McpProviders: providers}, "", false, false, false, false, nil)
			docker := buildReconnectCreateInstanceRequest(req, "previous-execution")
			sprites := agentctlInstanceRequest(req, spritesWorkspacePath)
			ssh := buildSSHCreateInstanceRequest(req, "/workspace", "/remote/agentctl")

			mapped := map[string][]string{
				"standalone":    standalone.McpProviders,
				"container":     container.McpProviders,
				"docker":        docker.McpProviders,
				"sprites":       sprites.McpProviders,
				executorTypeSSH: ssh.McpProviders,
			}
			for backend, got := range mapped {
				if !reflect.DeepEqual(got, providers) {
					t.Errorf("%s provider mapping = %#v, want %#v", backend, got, providers)
				}
			}
		})
	}
}

func TestAgentctlMCPToolNamePresentationCapabilityPropagatesThroughExecutors(t *testing.T) {
	agent := &fakeRuntimeAgent{
		MockAgent:          agents.NewMockAgent(),
		id:                 "auggie",
		namespacesMCPTools: true,
	}
	req := &ExecutorCreateRequest{AgentConfig: agent}
	sprites := agentctlInstanceRequest(req, spritesWorkspacePath)
	ssh := buildSSHCreateInstanceRequest(req, "/workspace", "/remote/agentctl")

	requests := map[string]*agentctl.CreateInstanceRequest{
		"standalone": buildStandaloneCreateInstanceRequest(req, nil, "", false, false, false, false, nil),
		"container":  buildContainerCreateInstanceRequest(ContainerConfig{AgentConfig: agent}, "", false, false, false, false, nil),
		"docker":     buildReconnectCreateInstanceRequest(req, "previous-execution"),
		"sprites":    &sprites,
		"ssh":        &ssh,
	}
	for executor, request := range requests {
		if !request.NamespacesMCPToolsByServer {
			t.Errorf("%s request lost NamespacesMCPToolsByServer", executor)
		}
	}
}

func TestManagerLaunchCarriesMCPProvidersToExecutor(t *testing.T) {
	mgr, backend := newEnvironmentExecutionTestManager(t, nil)
	want := []string{"github", "gitlab"}
	_, err := mgr.Launch(context.Background(), &LaunchRequest{
		TaskID:         "task-provider-launch",
		SessionID:      "session-provider-launch",
		AgentProfileID: "profile-provider-launch",
		ExecutorType:   string(models.ExecutorTypeLocal),
		IsEphemeral:    true,
		McpProviders:   want,
	})
	if err != nil {
		t.Fatalf("Manager.Launch: %v", err)
	}
	if backend.lastRequest == nil {
		t.Fatal("executor CreateInstance was not called")
	}
	if !reflect.DeepEqual(backend.lastRequest.McpProviders, want) {
		t.Fatalf("executor McpProviders = %#v, want %#v", backend.lastRequest.McpProviders, want)
	}
}

func TestCopilotNativeOptInPropagatesThroughEveryAgentctlCreateRequest(t *testing.T) {
	agent := &fakeRuntimeAgent{MockAgent: agents.NewMockAgent(), id: agents.CopilotACPAgentID}
	req := &ExecutorCreateRequest{
		AgentConfig: agent, DeliveryStreamID: "stream-native", CopilotNativeSessionEvents: true,
	}
	containerConfig := ContainerConfig{
		AgentConfig: agent, DeliveryStreamID: "stream-native", CopilotNativeSessionEvents: true,
	}
	sprites := agentctlInstanceRequest(req, spritesWorkspacePath)
	ssh := buildSSHCreateInstanceRequest(req, "/workspace", "/remote/agentctl")
	requests := map[string]*agentctl.CreateInstanceRequest{
		"standalone": buildStandaloneCreateInstanceRequest(req, nil, agent.ID(), false, false, false, false, nil),
		"container":  buildContainerCreateInstanceRequest(containerConfig, agent.ID(), false, false, false, false, nil),
		"docker":     buildReconnectCreateInstanceRequest(req, "previous-execution"),
		"sprites":    &sprites,
		"ssh":        &ssh,
	}
	for name, request := range requests {
		if !request.CopilotNativeSessionEvents {
			t.Errorf("%s request lost CopilotNativeSessionEvents", name)
		}
	}
	dockerConfig, err := buildDockerContainerConfig(req, "local_docker")
	if err != nil {
		t.Fatalf("buildDockerContainerConfig: %v", err)
	}
	if !dockerConfig.CopilotNativeSessionEvents {
		t.Fatal("Docker container config lost CopilotNativeSessionEvents")
	}

	other := &fakeRuntimeAgent{MockAgent: agents.NewMockAgent(), id: "claude-acp"}
	if buildStandaloneCreateInstanceRequest(&ExecutorCreateRequest{AgentConfig: other, DeliveryStreamID: "stream-native"},
		nil, other.ID(), false, false, false, false, nil).CopilotNativeSessionEvents {
		t.Fatal("non-Copilot launch must not opt into vendor native events")
	}
	noStream := &ExecutorCreateRequest{AgentConfig: agent}
	if buildStandaloneCreateInstanceRequest(noStream, nil, agent.ID(), false, false, false, false, nil).CopilotNativeSessionEvents {
		t.Fatal("Copilot launch without a durable stream must not opt in")
	}
}
