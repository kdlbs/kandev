package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type pluginExecutorOperationsFake struct {
	provisionRequest   *pluginsdk.ProvisionExecutorEnvironmentRequest
	provisionResponse  *pluginsdk.ProvisionExecutorEnvironmentResponse
	provisionErr       error
	recoverRequest     *pluginsdk.RecoverExecutorOperationRequest
	recoverResponse    *pluginsdk.RecoverExecutorOperationResponse
	recoverErr         error
	attachRequest      *pluginsdk.AttachExecutorEnvironmentRequest
	attachResponse     *pluginsdk.AttachExecutorEnvironmentResponse
	attachErr          error
	inspectRequests    []*pluginsdk.InspectExecutorEnvironmentRequest
	inspectResponses   []*pluginsdk.InspectExecutorEnvironmentResponse
	inspectIndex       int
	inspectErr         error
	connectionResponse *pluginsdk.ResolveExecutorConnectionResponse
	connectionErr      error
	destroyRequest     *pluginsdk.DestroyExecutorEnvironmentRequest
	destroyResponse    *pluginsdk.DestroyExecutorEnvironmentResponse
	destroyErr         error
}

func (f *pluginExecutorOperationsFake) ProvisionExecutorEnvironment(_ context.Context, req *pluginsdk.ProvisionExecutorEnvironmentRequest) (*pluginsdk.ProvisionExecutorEnvironmentResponse, error) {
	f.provisionRequest = req
	return f.provisionResponse, f.provisionErr
}

func (f *pluginExecutorOperationsFake) RecoverExecutorOperation(_ context.Context, req *pluginsdk.RecoverExecutorOperationRequest) (*pluginsdk.RecoverExecutorOperationResponse, error) {
	f.recoverRequest = req
	return f.recoverResponse, f.recoverErr
}

func (f *pluginExecutorOperationsFake) AttachExecutorEnvironment(_ context.Context, req *pluginsdk.AttachExecutorEnvironmentRequest) (*pluginsdk.AttachExecutorEnvironmentResponse, error) {
	f.attachRequest = req
	return f.attachResponse, f.attachErr
}

func (f *pluginExecutorOperationsFake) InspectExecutorEnvironment(_ context.Context, req *pluginsdk.InspectExecutorEnvironmentRequest) (*pluginsdk.InspectExecutorEnvironmentResponse, error) {
	f.inspectRequests = append(f.inspectRequests, req)
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	if f.inspectIndex < len(f.inspectResponses) {
		response := f.inspectResponses[f.inspectIndex]
		f.inspectIndex++
		return response, nil
	}
	return nil, errors.New("unexpected inspect")
}

func (f *pluginExecutorOperationsFake) ResolveExecutorConnection(_ context.Context, _ *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error) {
	return f.connectionResponse, f.connectionErr
}

func (f *pluginExecutorOperationsFake) DestroyExecutorEnvironment(_ context.Context, req *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error) {
	f.destroyRequest = req
	return f.destroyResponse, f.destroyErr
}

func TestPluginExecutorLaunch(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	operations := &pluginExecutorOperationsFake{
		provisionResponse: &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: &pluginsdk.ExecutorResourceDescriptor{
			ResourceHandle: "opaque-resource", StateJson: `{"resource_id":"fixture-1"}`, Platform: "linux-amd64", StateVersion: 1,
			Capabilities: &pluginsdk.ExecutorProviderCapabilities{Terminal: true, Files: true, Git: true},
		}},
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: "2026-09-26T14:00:00Z", Generation: "generation-1",
		}},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.newAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, _ string) (*agentctl.Client, string, error) {
		if _, err := resolver(ctx); err != nil {
			return nil, "", err
		}
		return agentctl.NewClient("unused", 0, log, agentctl.WithExecutionID(executionID)), "agentctl-token", nil
	}
	runtime.ready = func(context.Context, *agentctl.Client) error { return nil }

	var checkpoints []map[string]interface{}
	request := pluginExecutorLaunchRequest(provider)
	request.CheckpointRuntimeInventory = func(_ context.Context, checkpoint map[string]interface{}) error {
		checkpoints = append(checkpoints, checkpoint)
		return nil
	}
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	instance, err := runtime.CreateInstance(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateInstance(): %v", err)
	}
	if instance.RuntimeName != agentruntime.RuntimePluginRemote || instance.AuthToken != "agentctl-token" || instance.Client == nil {
		t.Fatalf("instance = %#v", instance)
	}
	if got := operations.provisionRequest.GetContext().GetOperationId(); got != request.InstanceID {
		t.Fatalf("operation id = %q, want stable execution id %q", got, request.InstanceID)
	}
	if got := operations.provisionRequest.GetProfile().GetSecretValues()["credential"]; got != "launch-secret" {
		t.Fatalf("transient secret value = %q", got)
	}
	if len(checkpoints) != 3 {
		t.Fatalf("checkpoint count = %d, want allocating, provisioned, and ready", len(checkpoints))
	}
	encoded, err := json.Marshal(checkpoints)
	if err != nil {
		t.Fatalf("marshal checkpoints: %v", err)
	}
	if string(encoded) == "" || containsString(string(encoded), "launch-secret") {
		t.Fatalf("checkpoint leaked a profile secret: %s", encoded)
	}
}

func TestPluginExecutorPartialLaunch(t *testing.T) {
	operations := &pluginExecutorOperationsFake{
		provisionResponse: &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "allocated"}},
		connectionErr:     errors.New("connection lease unavailable"),
		destroyResponse:   &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	runtime.newAgentctlClient = func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.Client, string, error) {
		return nil, "", errors.New("unexpected agentctl client construction")
	}
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	var released bool
	request.ReleaseRuntimeInventory = func(context.Context) error { released = true; return nil }
	var phases []string
	request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok {
			phases = append(phases, envelope.Phase)
		}
		return nil
	}
	if _, err := runtime.CreateInstance(context.Background(), request); err == nil {
		t.Fatal("CreateInstance() unexpectedly succeeded")
	}
	if operations.destroyRequest == nil || operations.destroyRequest.GetCleanupReason() != "launch_failed" {
		t.Fatalf("cleanup request = %#v", operations.destroyRequest)
	}
	if !released {
		t.Fatal("confirmed absence did not release provisional inventory")
	}
	if !containsString(strings.Join(phases, ","), "absent") {
		t.Fatalf("checkpoint phases = %v, want final absent", phases)
	}
}

func TestPluginExecutorBootstrapSecrets(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	operations := &pluginExecutorOperationsFake{
		provisionErr:    errors.New("allocation rejected"),
		recoverResponse: &pluginsdk.RecoverExecutorOperationResponse{Outcome: "unknown"},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	request := pluginExecutorLaunchRequest(provider)
	var checkpoints []map[string]interface{}
	request.CheckpointRuntimeInventory = func(_ context.Context, checkpoint map[string]interface{}) error {
		checkpoints = append(checkpoints, checkpoint)
		return nil
	}
	if _, err := runtime.CreateInstance(context.Background(), request); err == nil {
		t.Fatal("CreateInstance() unexpectedly succeeded")
	}
	if operations.recoverResponse == nil {
		t.Fatal("ambiguous provision response was not recovered by original operation id")
	}
	for _, checkpoint := range checkpoints {
		encoded, _ := json.Marshal(checkpoint)
		if containsString(string(encoded), "launch-secret") {
			t.Fatalf("inventory checkpoint leaked a profile secret: %s", encoded)
		}
	}
}

func TestPluginExecutorHostCallbacksAreExecutionAndOwnershipFenced(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	launch := pluginExecutorLaunchRequest(provider)
	launch.PluginExecutor.Profile.OwnershipGeneration = 4
	launch.OnProgress = func(step PrepareStep, index, total int) {
		if step.Name != "bootstrap" || step.Status != PrepareStepCompleted || index != 2 || total != 2 {
			t.Errorf("progress = %+v %d/%d", step, index, total)
		}
	}
	reader := &fakeExecutorProfileReader{
		session: &models.TaskSession{ID: launch.SessionID, TaskID: launch.TaskID, TaskEnvironmentID: launch.TaskEnvironmentID},
		env:     &models.TaskEnvironment{ID: launch.TaskEnvironmentID, TaskID: launch.TaskID, OwnershipGeneration: 4},
	}
	manager := &Manager{logger: newTestLogger(), executorProfileReader: reader}
	if err := manager.registerPluginExecutorCallbacks(launch); err != nil {
		t.Fatalf("register callbacks: %v", err)
	}
	defer manager.unregisterPluginExecutorCallbacks(launch.InstanceID)

	callbackContext := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: provider.PluginID, InstallationId: provider.InstallationID, ProviderKey: provider.Key,
		ContractVersion: int32(provider.ContractVersion), TaskId: launch.TaskID, SessionId: launch.SessionID,
		EnvironmentId: launch.TaskEnvironmentID, ExecutionId: launch.InstanceID, OperationId: launch.InstanceID,
		EnvironmentGeneration: 4, DispatchGeneration: 11, InputDigest: "digest",
	}
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-1", StateJson: `{"id":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	var checkpoint map[string]interface{}
	launch.CheckpointRuntimeInventory = func(_ context.Context, value map[string]interface{}) error { checkpoint = value; return nil }
	if err := manager.CheckpointExecutorResource(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
		Context: callbackContext, Resource: resource, Phase: "provisioned",
	}); err != nil {
		t.Fatalf("CheckpointExecutorResource: %v", err)
	}
	envelope, ok := checkpoint[MetadataKeyPluginExecutor].(pluginExecutorInventory)
	if !ok || envelope.Resource.GetResourceHandle() != "resource-1" || envelope.EnvironmentGeneration != 4 {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	if err := manager.ReportExecutorProgress(context.Background(), &pluginsdk.ReportExecutorProgressRequest{
		Context: callbackContext, Stage: "bootstrap", Completed: 2, Total: 2,
	}); err != nil {
		t.Fatalf("ReportExecutorProgress: %v", err)
	}
	if err := manager.ReportExecutorProgress(context.Background(), &pluginsdk.ReportExecutorProgressRequest{
		Context: callbackContext, Stage: "raw-log-line", Completed: 1, Total: 2,
	}); err == nil {
		t.Fatal("unsupported provider progress stage was accepted")
	}

	artifactPath := filepath.Join(t.TempDir(), "agentctl-linux-amd64")
	artifact := []byte("fixture-agentctl-artifact")
	if err := os.WriteFile(artifactPath, artifact, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	t.Setenv("KANDEV_AGENTCTL_LINUX_AMD64_BINARY", artifactPath)
	var chunks []*pluginsdk.ExecutorRuntimeArtifactChunk
	if err := manager.ReadExecutorRuntimeArtifact(context.Background(), &pluginsdk.ReadExecutorRuntimeArtifactRequest{
		Context: callbackContext, ArtifactId: "agentctl", Platform: "linux-amd64",
	}, func(chunk *pluginsdk.ExecutorRuntimeArtifactChunk) error { chunks = append(chunks, chunk); return nil }); err != nil {
		t.Fatalf("ReadExecutorRuntimeArtifact: %v", err)
	}
	if len(chunks) != 1 || string(chunks[0].GetData()) != string(artifact) || !chunks[0].GetFinal() {
		t.Fatalf("artifact chunks = %#v", chunks)
	}
	digest := sha256.Sum256(artifact)
	if chunks[0].GetSha256() != fmt.Sprintf("%x", digest) {
		t.Fatalf("artifact digest = %q", chunks[0].GetSha256())
	}

	reader.env.OwnershipGeneration++
	if err := manager.ReportExecutorProgress(context.Background(), &pluginsdk.ReportExecutorProgressRequest{
		Context: callbackContext, Stage: "bootstrap", Completed: 1, Total: 2,
	}); err == nil {
		t.Fatal("stale environment ownership callback was accepted")
	}
}

func pluginExecutorLaunchRequest(provider models.ExecutorProvider) *ExecutorCreateRequest {
	return &ExecutorCreateRequest{
		InstanceID: "execution-plugin-1", TaskID: "task-plugin-1", SessionID: "session-plugin-1",
		TaskEnvironmentID: "environment-plugin-1", AgentProfileID: "agent-profile-1",
		Env: map[string]string{envKeyKandevAPIURL: "https://kandev.example/api/v1"},
		PluginExecutor: &PluginExecutorLaunch{Profile: models.ExecutorProviderLaunchProfile{
			Provider: provider, ProfileID: "executor-profile-1", Config: map[string]string{"region": "eu-west-1"},
			SecretValues: map[string]string{"credential": "launch-secret"}, SecretReferences: map[string]string{"credential": "vault-ref-1"},
		}},
	}
}

func testPluginExecutorLaunchProvider() models.ExecutorProvider {
	return models.ExecutorProvider{
		ExecutorID: "exec-plugin-1", Identity: "plugin:example:remote", PluginID: "example", InstallationID: "install-1",
		Key: "remote", ContractVersion: 1, SupportedStateVersions: []int{1}, Available: true,
		Capabilities: models.ExecutorProviderCapabilities{Terminal: true, Files: true, Git: true, Reattach: true, Retention: "persistent"},
	}
}
