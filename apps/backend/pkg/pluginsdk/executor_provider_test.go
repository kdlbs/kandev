package pluginsdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	testing "testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeExecutorProviderPlugin struct {
	UnimplementedPlugin
	lastContext *ExecutorProviderRequestContext
	lastSecret  string
}

type partialExecutorProviderPlugin struct{ UnimplementedPlugin }

func (*partialExecutorProviderPlugin) ValidateExecutorProfile(context.Context, *ValidateExecutorProfileRequest) (*ValidateExecutorProfileResponse, error) {
	return &ValidateExecutorProfileResponse{}, nil
}

func (p *fakeExecutorProviderPlugin) ValidateExecutorProfile(_ context.Context, req *ValidateExecutorProfileRequest) (*ValidateExecutorProfileResponse, error) {
	p.lastContext = req.GetContext()
	p.lastSecret = req.GetProfile().GetSecretValues()["credential"]
	return &ValidateExecutorProfileResponse{
		FieldErrors:  []*ExecutorProviderFieldError{{Field: "region", Code: "required", MessageId: "profile.region.required"}},
		Capabilities: &ExecutorProviderCapabilities{Files: true, Reattach: true, Retention: "bounded", MaximumLifetimeSeconds: 3600},
		Error:        &ExecutorProviderError{Code: "invalid_config", MessageId: "profile.invalid"},
	}, nil
}

func (*fakeExecutorProviderPlugin) ProvisionExecutorEnvironment(_ context.Context, req *ProvisionExecutorEnvironmentRequest) (*ProvisionExecutorEnvironmentResponse, error) {
	return &ProvisionExecutorEnvironmentResponse{Resource: &ExecutorResourceDescriptor{
		ResourceHandle: "opaque-resource", StateJson: `{"id":"instance-1"}`, Platform: "linux-amd64",
		Capabilities: &ExecutorProviderCapabilities{Files: true, Reattach: true}, ExpiresAt: "2026-09-26T18:00:00Z", Retention: "bounded",
	}, Error: nil}, nil
}

func (*fakeExecutorProviderPlugin) RecoverExecutorOperation(context.Context, *RecoverExecutorOperationRequest) (*RecoverExecutorOperationResponse, error) {
	return &RecoverExecutorOperationResponse{Outcome: "found", Resource: &ExecutorResourceDescriptor{ResourceHandle: "opaque-resource"}}, nil
}

func (*fakeExecutorProviderPlugin) AttachExecutorEnvironment(context.Context, *AttachExecutorEnvironmentRequest) (*AttachExecutorEnvironmentResponse, error) {
	return &AttachExecutorEnvironmentResponse{Resource: &ExecutorResourceDescriptor{ResourceHandle: "opaque-resource"}}, nil
}

func (*fakeExecutorProviderPlugin) InspectExecutorEnvironment(context.Context, *InspectExecutorEnvironmentRequest) (*InspectExecutorEnvironmentResponse, error) {
	return &InspectExecutorEnvironmentResponse{State: "running", ExpiresAt: "2026-09-26T18:00:00Z"}, nil
}

func (*fakeExecutorProviderPlugin) ResolveExecutorConnection(context.Context, *ResolveExecutorConnectionRequest) (*ResolveExecutorConnectionResponse, error) {
	return &ResolveExecutorConnectionResponse{Lease: &ExecutorConnectionLease{
		BaseUrl: "https://agent.example.test", HttpHeaders: map[string]string{"X-Provider-Token": "transient"},
		WebsocketHeaders: map[string]string{"X-Provider-Token": "transient"}, WebsocketSubprotocols: []string{"acp"},
		ExpiresAt: "2026-09-26T18:00:00Z", Generation: "connection-1",
	}}, nil
}

func (*fakeExecutorProviderPlugin) DestroyExecutorEnvironment(context.Context, *DestroyExecutorEnvironmentRequest) (*DestroyExecutorEnvironmentResponse, error) {
	return &DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}, nil
}

type fakeExecutorProviderHost struct {
	chunks []*ExecutorRuntimeArtifactChunk
}

func (h *fakeExecutorProviderHost) CheckpointExecutorResource(_ context.Context, req *CheckpointExecutorResourceRequest) (*CheckpointExecutorResourceResponse, error) {
	return &CheckpointExecutorResourceResponse{Accepted: req.GetResource().GetResourceHandle() == "opaque-resource"}, nil
}

func (*fakeExecutorProviderHost) ReportExecutorProgress(context.Context, *ReportExecutorProgressRequest) (*ReportExecutorProgressResponse, error) {
	return &ReportExecutorProgressResponse{Accepted: true}, nil
}

func (h *fakeExecutorProviderHost) ReadExecutorRuntimeArtifact(_ context.Context, _ *ReadExecutorRuntimeArtifactRequest, send func(*ExecutorRuntimeArtifactChunk) error) error {
	for _, chunk := range h.chunks {
		if err := send(chunk); err != nil {
			return err
		}
	}
	return nil
}

type fakeExecutorProviderHostImpl struct {
	*recordingHost
	provider ExecutorProviderHostAPI
}

func (h *fakeExecutorProviderHostImpl) ExecutorProvider() ExecutorProviderHostAPI { return h.provider }

func TestPluginExecutorWireContract(t *testing.T) {
	content := []byte("fixture-agentctl-runtime")
	digest := sha256.Sum256(content)
	hostAPI := &fakeExecutorProviderHost{chunks: []*ExecutorRuntimeArtifactChunk{{
		ArtifactId: "agentctl", Version: "1.0.0", TotalSizeBytes: uint64(len(content)), Sha256: hex.EncodeToString(digest[:]),
		ChunkIndex: 0, Data: content, Final: true,
	}}}
	author := &fakeExecutorProviderPlugin{}
	host := &fakeExecutorProviderHostImpl{recordingHost: &recordingHost{}, provider: hostAPI}
	gp := &GRPCPlugin{Impl: author, Host: host, HostDialTimeout: 5 * time.Second}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{PluginMapKey: gp})
	defer func() { _ = client.Close() }()
	defer server.Stop()

	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote := raw.(*RemotePlugin)
	ctx := &ExecutorProviderRequestContext{
		PluginId: "cloud", InstallationId: "install-1", ProviderKey: "microvm", ContractVersion: 1,
		WorkspaceId: "workspace-1", TaskId: "task-1", SessionId: "session-1", EnvironmentId: "env-1",
		ExecutionId: "execution-1", OperationId: "operation-1", Deadline: "2026-09-26T17:00:00Z", EnvironmentGeneration: 3,
		InputDigest: "digest-1",
	}
	profile := &ExecutorProfileSnapshot{ProfileId: "profile-1", Config: map[string]string{"region": "eu-west-1"}, SecretValues: map[string]string{"credential": "sensitive-value"}}

	validation, err := remote.ValidateExecutorProfile(context.Background(), &ValidateExecutorProfileRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)
	require.Equal(t, "operation-1", author.lastContext.GetOperationId())
	require.Equal(t, "sensitive-value", author.lastSecret)
	require.Equal(t, "profile.invalid", validation.GetError().GetMessageId())
	require.NotContains(t, validation.String(), "sensitive-value")

	provisioned, err := remote.ProvisionExecutorEnvironment(context.Background(), &ProvisionExecutorEnvironmentRequest{Context: ctx, Profile: profile, BootstrapJson: `{"nonce":"one-time"}`})
	require.NoError(t, err)
	require.Equal(t, "opaque-resource", provisioned.GetResource().GetResourceHandle())
	require.Equal(t, `{"id":"instance-1"}`, provisioned.GetResource().GetStateJson())

	recovered, err := remote.RecoverExecutorOperation(context.Background(), &RecoverExecutorOperationRequest{Context: ctx, Profile: profile})
	require.NoError(t, err)
	require.Equal(t, "found", recovered.GetOutcome())

	attached, err := remote.AttachExecutorEnvironment(context.Background(), &AttachExecutorEnvironmentRequest{Context: ctx, Resource: provisioned.GetResource(), ExpectedRuntimeIdentity: "agentctl:1"})
	require.NoError(t, err)
	require.Equal(t, "opaque-resource", attached.GetResource().GetResourceHandle())

	inspected, err := remote.InspectExecutorEnvironment(context.Background(), &InspectExecutorEnvironmentRequest{Context: ctx, Resource: provisioned.GetResource()})
	require.NoError(t, err)
	require.Equal(t, "running", inspected.GetState())

	connection, err := remote.ResolveExecutorConnection(context.Background(), &ResolveExecutorConnectionRequest{Context: ctx, Resource: provisioned.GetResource(), Purpose: "agentctl", RuntimePort: 8765})
	require.NoError(t, err)
	require.Equal(t, "connection-1", connection.GetLease().GetGeneration())
	require.Equal(t, "transient", connection.GetLease().GetHttpHeaders()["X-Provider-Token"])

	destroyed, err := remote.DestroyExecutorEnvironment(context.Background(), &DestroyExecutorEnvironmentRequest{Context: ctx, Resource: provisioned.GetResource(), CleanupReason: "reset", CleanupClaim: "claim-1"})
	require.NoError(t, err)
	require.True(t, destroyed.GetConfirmedAbsent())

	require.Eventually(t, func() bool { return author.Host() != nil }, 5*time.Second, 10*time.Millisecond)
	hostExtension, ok := author.Host().(ExecutorProviderHost)
	require.True(t, ok)
	callbacks := hostExtension.ExecutorProvider()
	checkpoint, err := callbacks.CheckpointExecutorResource(context.Background(), &CheckpointExecutorResourceRequest{Context: ctx, Resource: provisioned.GetResource(), Phase: "provisioned"})
	require.NoError(t, err)
	require.True(t, checkpoint.GetAccepted())
	progress, err := callbacks.ReportExecutorProgress(context.Background(), &ReportExecutorProgressRequest{Context: ctx, Stage: "bootstrap", Completed: 1, Total: 2})
	require.NoError(t, err)
	require.True(t, progress.GetAccepted())
	var downloaded []byte
	err = callbacks.ReadExecutorRuntimeArtifact(context.Background(), &ReadExecutorRuntimeArtifactRequest{Context: ctx, ArtifactId: "agentctl", Platform: "linux-amd64"}, func(chunk *ExecutorRuntimeArtifactChunk) error {
		downloaded = append(downloaded, chunk.GetData()...)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, content, downloaded)
}

func TestPluginExecutorOldPluginCompatibility(t *testing.T) {
	gp := &GRPCPlugin{Impl: &UnimplementedPlugin{}}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{PluginMapKey: gp})
	defer func() { _ = client.Close() }()
	defer server.Stop()
	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote := raw.(*RemotePlugin)
	_, err = remote.ValidateExecutorProfile(context.Background(), &ValidateExecutorProfileRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "complete executor provider")
}

func TestPluginExecutorIncompleteProviderRejected(t *testing.T) {
	gp := &GRPCPlugin{Impl: &partialExecutorProviderPlugin{}}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{PluginMapKey: gp})
	defer func() { _ = client.Close() }()
	defer server.Stop()
	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote := raw.(*RemotePlugin)
	_, err = remote.ValidateExecutorProfile(context.Background(), &ValidateExecutorProfileRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Contains(t, err.Error(), "complete executor provider")
}

func TestPluginExecutorArtifactRejectsOversizedChunk(t *testing.T) {
	content := make([]byte, MaxExecutorRuntimeArtifactChunkBytes+1)
	digest := sha256.Sum256(content)
	provider := &fakeExecutorProviderHost{
		chunks: []*ExecutorRuntimeArtifactChunk{{ArtifactId: "agentctl", Version: "1", TotalSizeBytes: uint64(len(content)), Sha256: hex.EncodeToString(digest[:]), Data: content, Final: true}},
	}
	validator := newExecutorArtifactStreamValidator("agentctl", func(*ExecutorRuntimeArtifactChunk) error { return nil })
	err := validator.accept(provider.chunks[0])
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

var _ pluginv1.PluginServer = (*grpcPluginServer)(nil)
