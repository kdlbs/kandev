package lifecycle

import (
	"context"
	"reflect"
	"testing"

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
			sprites := spriteCreateInstanceRequest(req)
			ssh := buildSSHCreateInstanceRequest(req, "/workspace")

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

// Regression: the container (docker executor fresh-launch) create-instance
// builder dropped TaskID while SessionID survived, leaving the in-container
// MCP server unbound — set_task_title_kandev / step_complete_kandev then fail
// with "requires a bound task" for every task running in a docker container.
// Keep all five executor builders forwarding the same identity fields.
func TestExecutorCreateInstanceBuildersPropagateTaskAndSession(t *testing.T) {
	const wantTask = "task-identity-propagation"
	const wantSession = "session-identity-propagation"

	req := &ExecutorCreateRequest{TaskID: wantTask, SessionID: wantSession}
	containerCfg := ContainerConfig{TaskID: wantTask, SessionID: wantSession}

	standalone := buildStandaloneCreateInstanceRequest(req, nil, "", false, false, false, false, nil)
	container := buildContainerCreateInstanceRequest(containerCfg, "", false, false, false, false, nil)
	reconnect := buildReconnectCreateInstanceRequest(req, "previous-execution")
	sprites := spriteCreateInstanceRequest(req)
	ssh := buildSSHCreateInstanceRequest(req, "/workspace")

	mapped := map[string]*agentctlCreateInstanceIdentity{
		"standalone": {standalone.TaskID, standalone.SessionID},
		"container":  {container.TaskID, container.SessionID},
		"reconnect":  {reconnect.TaskID, reconnect.SessionID},
		"sprites":    {sprites.TaskID, sprites.SessionID},
		"ssh":        {ssh.TaskID, ssh.SessionID},
	}
	for backend, got := range mapped {
		if got.taskID != wantTask {
			t.Errorf("%s TaskID = %q, want %q", backend, got.taskID, wantTask)
		}
		if got.sessionID != wantSession {
			t.Errorf("%s SessionID = %q, want %q", backend, got.sessionID, wantSession)
		}
	}
}

type agentctlCreateInstanceIdentity struct {
	taskID    string
	sessionID string
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
