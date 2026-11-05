package plugins

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestPluginExecutorPackagedRecovery(t *testing.T) {
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()
	dataDir := t.TempDir()
	binary := filepath.Join(t.TempDir(), "plugin-fixture")
	packageDir, err := os.Getwd()
	require.NoError(t, err)
	fixturePackage := filepath.Clean(filepath.Join(packageDir, "../../cmd/plugin-fixture"))
	build := exec.CommandContext(buildCtx, "go", "build", "-trimpath", "-o", binary, ".")
	build.Dir = fixturePackage
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	cancelBuild()
	require.NoError(t, err, string(output))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	profile := &pluginsdk.ExecutorProfileSnapshot{ProfileId: "profile-1", Config: map[string]string{"region": "eu-west-1"}}
	requestContext := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "kandev-plugin-e2e", InstallationId: "installation-1", ProviderKey: "remote-sandbox",
		ContractVersion: 1, EnvironmentId: "environment-1", OperationId: "operation-1", InputDigest: "input-digest-1",
	}

	checkpointHost := &acceptingCheckpointHost{}
	startFixture := func() (*hcplugin.Client, *pluginsdk.RemotePlugin) {
		client := hcplugin.NewClient(&hcplugin.ClientConfig{
			HandshakeConfig:  pluginsdk.Handshake,
			Plugins:          map[string]hcplugin.Plugin{pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{Host: checkpointHost}},
			AllowedProtocols: []hcplugin.Protocol{hcplugin.ProtocolGRPC},
			AutoMTLS:         true,
			Cmd:              fixtureCommand(binary, dataDir),
			Logger:           hclog.NewNullLogger(),
		})
		_, err := client.Start()
		require.NoError(t, err)
		rpcClient, err := client.Client()
		require.NoError(t, err)
		raw, err := rpcClient.Dispense(pluginsdk.PluginMapKey)
		require.NoError(t, err)
		return client, raw.(*pluginsdk.RemotePlugin)
	}

	client, remote := startFixture()
	defer client.Kill()
	provisioned, err := remote.ProvisionExecutorEnvironment(ctx, &pluginsdk.ProvisionExecutorEnvironmentRequest{
		Context: requestContext, Profile: profile, BootstrapJson: `{"nonce":"one-time"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "fixture:environment-1", provisioned.GetResource().GetResourceHandle())
	connectionRequest := &pluginsdk.ResolveExecutorConnectionRequest{
		Context: requestContext, Resource: provisioned.GetResource(), Purpose: "agentctl", RuntimePort: 8765,
	}
	firstLease, err := remote.ResolveExecutorConnection(ctx, connectionRequest)
	require.NoError(t, err)
	require.Equal(t, "first-request", fixtureExecutorLeaseRequest(t, ctx, dataDir, firstLease.GetLease(), "first-request"))
	rotatedLease, err := remote.ResolveExecutorConnection(ctx, connectionRequest)
	require.NoError(t, err)
	require.NotEqual(t, firstLease.GetLease().GetGeneration(), rotatedLease.GetLease().GetGeneration())
	require.NotEqual(t, firstLease.GetLease().GetHttpHeaders()["X-Fixture-Token"], rotatedLease.GetLease().GetHttpHeaders()["X-Fixture-Token"])
	require.Equal(t, "rotated-request", fixtureExecutorLeaseRequest(t, ctx, dataDir, rotatedLease.GetLease(), "rotated-request"))
	staleStatus, _ := fixtureExecutorLeaseStatus(t, ctx, dataDir, firstLease.GetLease())
	require.Equal(t, http.StatusUnauthorized, staleStatus)

	client.Kill()
	restartedClient, restartedRemote := startFixture()
	defer restartedClient.Kill()
	recovered, err := restartedRemote.RecoverExecutorOperation(ctx, &pluginsdk.RecoverExecutorOperationRequest{Context: requestContext, Profile: profile})
	require.NoError(t, err)
	require.Equal(t, "found", recovered.GetOutcome())
	require.Equal(t, provisioned.GetResource().GetResourceHandle(), recovered.GetResource().GetResourceHandle())

	reattached, err := restartedRemote.AttachExecutorEnvironment(ctx, &pluginsdk.AttachExecutorEnvironmentRequest{
		Context: requestContext, Resource: recovered.GetResource(), ExpectedRuntimeIdentity: "agentctl:1",
	})
	require.NoError(t, err)
	require.Equal(t, provisioned.GetResource().GetResourceHandle(), reattached.GetResource().GetResourceHandle())
	connectionRequest.Resource = reattached.GetResource()
	lease, err := restartedRemote.ResolveExecutorConnection(ctx, connectionRequest)
	require.NoError(t, err)
	require.NotEmpty(t, lease.GetLease().GetHttpHeaders()["X-Fixture-Token"])
	require.NotEqual(t, rotatedLease.GetLease().GetHttpHeaders()["X-Fixture-Token"], lease.GetLease().GetHttpHeaders()["X-Fixture-Token"])
	require.Equal(t, "reattached-request", fixtureExecutorLeaseRequest(t, ctx, dataDir, lease.GetLease(), "reattached-request"))

	statePath := filepath.Join(dataDir, "executor-resources.json")
	state, err := os.ReadFile(statePath)
	require.NoError(t, err)
	require.Contains(t, string(state), `"allocations":1`)
	require.NotContains(t, string(state), "one-time")
	require.NotContains(t, string(state), "fixture-secret")

	destroyed, err := restartedRemote.DestroyExecutorEnvironment(ctx, &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: requestContext, Resource: reattached.GetResource(), CleanupReason: "launch_failed", CleanupClaim: "claim-1", DrainOutcome: "not_attempted",
	})
	require.NoError(t, err)
	require.True(t, destroyed.GetConfirmedAbsent())
	require.Equal(t, []string{"cleanup_progress"}, checkpointHost.phases(), "the destroy checkpoints before it mutates")
	require.Equal(t, []string{"claim-1"}, checkpointHost.claims(), "the checkpoint echoes the destroy's cleanup claim")
}

// acceptingCheckpointHost stands in for the host side of a destroy under a
// persisted cleanup claim: it accepts and records the provider's checkpoints.
type acceptingCheckpointHost struct {
	pluginsdk.Host
	mu      sync.Mutex
	checked []string
	echoed  []string
}

func (h *acceptingCheckpointHost) ExecutorProvider() pluginsdk.ExecutorProviderHostAPI { return h }

func (h *acceptingCheckpointHost) CheckpointExecutorResource(_ context.Context, req *pluginsdk.CheckpointExecutorResourceRequest) (*pluginsdk.CheckpointExecutorResourceResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checked = append(h.checked, req.GetPhase())
	h.echoed = append(h.echoed, req.GetClaim())
	return &pluginsdk.CheckpointExecutorResourceResponse{Accepted: true}, nil
}

func (h *acceptingCheckpointHost) ReportExecutorProgress(context.Context, *pluginsdk.ReportExecutorProgressRequest) (*pluginsdk.ReportExecutorProgressResponse, error) {
	return &pluginsdk.ReportExecutorProgressResponse{Accepted: true}, nil
}

func (h *acceptingCheckpointHost) ReadExecutorRuntimeArtifact(context.Context, *pluginsdk.ReadExecutorRuntimeArtifactRequest, func(*pluginsdk.ExecutorRuntimeArtifactChunk) error) error {
	return nil
}

func (h *acceptingCheckpointHost) claims() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.echoed...)
}

func (h *acceptingCheckpointHost) phases() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.checked...)
}

func fixtureExecutorLeaseRequest(t *testing.T, ctx context.Context, dataDir string, lease *pluginsdk.ExecutorConnectionLease, value string) string {
	t.Helper()
	statusCode, body := fixtureExecutorLeaseResponse(t, ctx, dataDir, lease, "/echo?value="+url.QueryEscape(value))
	require.Equal(t, http.StatusOK, statusCode)
	return body
}

func fixtureExecutorLeaseStatus(t *testing.T, ctx context.Context, dataDir string, lease *pluginsdk.ExecutorConnectionLease) (int, string) {
	t.Helper()
	return fixtureExecutorLeaseResponse(t, ctx, dataDir, lease, "/health")
}

func fixtureExecutorLeaseResponse(t *testing.T, ctx context.Context, dataDir string, lease *pluginsdk.ExecutorConnectionLease, requestPath string) (int, string) {
	t.Helper()
	roots := x509.NewCertPool()
	certificate, err := os.ReadFile(filepath.Join(dataDir, "executor-test-ca.pem"))
	require.NoError(t, err)
	require.True(t, roots.AppendCertsFromPEM(certificate))
	endpoint, err := url.Parse(lease.GetBaseUrl() + requestPath)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	require.NoError(t, err)
	for name, value := range lease.GetHttpHeaders() {
		request.Header.Set(name, value)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(body)
}

func fixtureCommand(binary, dataDir string) *exec.Cmd {
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "KANDEV_PLUGIN_DATA_DIR="+dataDir)
	return command
}
