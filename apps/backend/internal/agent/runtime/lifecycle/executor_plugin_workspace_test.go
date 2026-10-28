package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/workspacedescriptor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

const testBackendOnlyWorkspace = "/kandev-backend-host-only/never-created/workspace"

type workspaceLaunchHarness struct {
	runtime     *PluginRemoteExecutor
	operations  *capabilityConfirmerFake
	request     *ExecutorCreateRequest
	instance    agentctl.CreateInstanceRequest
	checkpoints []pluginExecutorInventory
}

func newWorkspaceLaunchHarness(t *testing.T, provider models.ExecutorProvider, resource *pluginsdk.ExecutorResourceDescriptor) *workspaceLaunchHarness {
	t.Helper()
	harness := &workspaceLaunchHarness{operations: &capabilityConfirmerFake{}}
	harness.operations.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: resource}
	harness.operations.connectionResponse = &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
		BaseUrl: "https://executor.example", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Generation: "generation-1",
	}}
	harness.operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}
	harness.runtime = NewPluginRemoteExecutor(harness.operations, newTestLogger())
	harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&harness.instance)
		_ = json.NewEncoder(w).Encode(agentctl.CreateInstanceResponse{ID: harness.instance.ID, Port: testPluginExecutorInstancePort})
	}))
	t.Cleanup(control.Close)
	controlURL, _ := url.Parse(control.URL)
	controlPort, _ := strconv.Atoi(controlURL.Port())
	harness.runtime.newAgentctlControlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, _ string) (*agentctl.ControlClient, string, error) {
		if _, err := resolver(ctx); err != nil {
			return nil, "", err
		}
		return agentctl.NewControlClient(controlURL.Hostname(), controlPort, log), "agentctl-token", nil
	}
	harness.runtime.ready = func(context.Context, *agentctl.Client) error { return nil }
	harness.request = pluginExecutorLaunchRequest(provider)
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok {
			harness.checkpoints = append(harness.checkpoints, envelope)
		}
		return nil
	}
	harness.request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	return harness
}

func workspaceResource(path, identity string) *pluginsdk.ExecutorResourceDescriptor {
	return &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "opaque-resource", StateJson: `{"resource_id":"fixture-1"}`, Platform: "linux-amd64", StateVersion: 1,
		Capabilities:  &pluginsdk.ExecutorProviderCapabilities{Terminal: true, Files: true, Git: true},
		WorkspacePath: path, WorkspaceIdentity: identity,
	}
}

func TestDeclaredWorkspaceDescriptorDrivesInstanceCreationAndIsRecorded(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, capabilityLaunchProvider(), workspaceResource(testBackendOnlyWorkspace, "worker-7"))

	instance, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if err != nil {
		t.Fatalf("CreateInstance(): %v", err)
	}
	if harness.instance.WorkspacePath != testBackendOnlyWorkspace || instance.WorkspacePath != testBackendOnlyWorkspace {
		t.Fatalf("instance workspace = %q, executor instance = %q; both must be the recorded path", harness.instance.WorkspacePath, instance.WorkspacePath)
	}
	ready := harness.checkpoints[len(harness.checkpoints)-1]
	want := &workspacedescriptor.Descriptor{Path: testBackendOnlyWorkspace, Identity: "worker-7"}
	if ready.WorkspaceDescriptor == nil || *ready.WorkspaceDescriptor != *want {
		t.Fatalf("ready envelope descriptor = %#v, want %#v", ready.WorkspaceDescriptor, want)
	}
	for _, envelope := range harness.checkpoints {
		if envelope.Phase == pluginExecutorPhaseProvisioned && envelope.WorkspaceDescriptor == nil {
			t.Fatal("the descriptor must be persisted with the provisioned checkpoint, before the first use of the path")
		}
	}
	if _, statErr := os.Stat(testBackendOnlyWorkspace); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the backend host must never create the recorded path: %v", statErr)
	}
}

func TestDeclaredWorkspaceDescriptorRejectionsCleanTheResource(t *testing.T) {
	cases := map[string]*pluginsdk.ExecutorResourceDescriptor{
		"missing path":     workspaceResource("", "worker-7"),
		"missing identity": workspaceResource(testBackendOnlyWorkspace, ""),
		"relative path":    workspaceResource("workspace", "worker-7"),
		"dot segment":      workspaceResource("/srv/../workspace", "worker-7"),
		"bad identity":     workspaceResource(testBackendOnlyWorkspace, "has space"),
		"windows form":     workspaceResource(`C:\work`, "worker-7"),
	}
	for name, resource := range cases {
		t.Run(name, func(t *testing.T) {
			harness := newWorkspaceLaunchHarness(t, capabilityLaunchProvider(), resource)
			if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err == nil {
				t.Fatal("CreateInstance() unexpectedly succeeded")
			}
			if harness.operations.destroyRequest == nil {
				t.Fatal("a rejected descriptor must run the existing cleanup for the created resource")
			}
			if harness.instance.ID != "" {
				t.Fatal("no agentctl instance may be created from a rejected descriptor")
			}
		})
	}
}

func TestUndeclaredWorkspaceDescriptorFieldsAreAConflict(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource(testBackendOnlyWorkspace, "worker-7"))
	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("CreateInstance() error = %v, want conflict", err)
	}
	if harness.operations.destroyRequest == nil {
		t.Fatal("the resource created by a conflicting response must be cleaned up")
	}
}

func TestUndeclaredProviderKeepsTheFixedWorkspacePath(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	instance, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if err != nil {
		t.Fatalf("CreateInstance(): %v", err)
	}
	if instance.WorkspacePath != pluginExecutorWorkspacePath || harness.instance.WorkspacePath != pluginExecutorWorkspacePath {
		t.Fatalf("workspace = %q / %q, want %q", instance.WorkspacePath, harness.instance.WorkspacePath, pluginExecutorWorkspacePath)
	}
	for _, envelope := range harness.checkpoints {
		if envelope.WorkspaceDescriptor != nil {
			t.Fatalf("a non-declaring provider must record no descriptor: %#v", envelope.WorkspaceDescriptor)
		}
	}
}

func TestResolveWorkspaceDescriptorImmutability(t *testing.T) {
	negotiated := []string{"workspace_descriptor@1"}
	recorded := &workspacedescriptor.Descriptor{Path: "/srv/work", Identity: "worker-7"}

	same, err := resolvePluginExecutorWorkspace(negotiated, recorded, workspaceResource("/srv/work", "worker-7"))
	if err != nil || same == nil || *same != *recorded {
		t.Fatalf("an identical descriptor must be accepted: %v, %#v", err, same)
	}
	for name, resource := range map[string]*pluginsdk.ExecutorResourceDescriptor{
		"different path":     workspaceResource("/srv/other", "worker-7"),
		"different identity": workspaceResource("/srv/work", "worker-8"),
		"same path respelt":  workspaceResource("/srv/work/", "worker-7"),
		"fields dropped":     workspaceResource("", ""),
	} {
		if _, err := resolvePluginExecutorWorkspace(negotiated, recorded, resource); err == nil {
			t.Errorf("%s: a changed or absent descriptor for a recorded generation must be refused", name)
		}
	}
	if _, err := resolvePluginExecutorWorkspace(negotiated, recorded, workspaceResource("/srv/other", "worker-7")); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("a changed descriptor must be a conflict, got %v", err)
	}
}

func TestRecoveredInstanceMustReportTheRecordedWorkspace(t *testing.T) {
	state := &pluginExecutorRecoveryState{inventory: pluginExecutorInventory{
		InstancePort:        testPluginExecutorInstancePort,
		WorkspaceDescriptor: &workspacedescriptor.Descriptor{Path: "/srv/work", Identity: "worker-7"},
	}}
	record := &models.ExecutorRunning{AgentExecutionID: "execution-1", TaskID: "task-1", SessionID: "session-1", TransientAuthToken: "token"}
	for path, ok := range map[string]bool{"/srv/work": true, "/workspace": false, "/srv/other": false} {
		runtime := NewPluginRemoteExecutor(&pluginExecutorOperationsFake{}, newTestLogger())
		runtime.recoveredAgentctlInfoReader = func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.InstanceInfo, error) {
			return &agentctl.InstanceInfo{ID: "execution-1", TaskID: "task-1", SessionID: "session-1", Port: testPluginExecutorInstancePort, WorkspacePath: path}, nil
		}
		_, err := runtime.readRecoveredAgentctlInstance(context.Background(), record, state)
		if (err == nil) != ok {
			t.Errorf("reported workspace %q: error = %v, want ok=%v", path, err, ok)
		}
	}
}

func TestCheckpointCallbackKeepsTheNegotiatedSetAndDescriptor(t *testing.T) {
	request := pluginExecutorLaunchRequest(capabilityLaunchProvider())
	context := &pluginsdk.ExecutorProviderRequestContext{OperationId: "operation-1", InputDigest: "digest", ExecutionId: request.InstanceID, EnvironmentGeneration: 1}
	inventory := pluginExecutorInventoryForLaunch(request, context)
	if len(inventory.NegotiatedCapabilities) == 0 {
		t.Fatal("a provider-driven checkpoint must carry the negotiated set, or the merge would erase it")
	}
}
