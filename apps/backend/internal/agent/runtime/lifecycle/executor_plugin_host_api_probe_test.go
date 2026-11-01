package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/runtimeprobe"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

const forwardedHostAPIURL = "http://127.0.0.1:38917"

// probeReply decides how the fake agentctl answers one probe attempt. A nil
// return means the fake agentctl performs the request against the host
// registry and reports its status.
type probeReply func(attempt int, request agentctltypes.HostAPIProbeRequest, w http.ResponseWriter) (handled bool)

type forwardedFixture struct {
	runtime    *PluginRemoteExecutor
	operations *capabilityConfirmerFake
	registry   *runtimeprobe.Registry
	request    *ExecutorCreateRequest
	mu         sync.Mutex
	attempts   int
	probeURLs  []string
	order      []string
}

func forwardedProvider() models.ExecutorProvider {
	provider := testPluginExecutorLaunchProvider()
	provider.RuntimeAPIURL = "provider_forwarded"
	provider.RequiredCapabilities = []models.ExecutorProviderCapabilityRequirement{{Name: "api_url_forward", Version: 1}}
	return provider
}

func newForwardedFixture(t *testing.T, provider models.ExecutorProvider, reply probeReply) *forwardedFixture {
	t.Helper()
	fixture := &forwardedFixture{registry: runtimeprobe.New()}
	fixture.operations = &capabilityConfirmerFake{}
	fixture.operations.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "opaque-resource", StateJson: `{"resource_id":"fixture-1"}`, Platform: "linux-amd64", StateVersion: 1,
		Capabilities: &pluginsdk.ExecutorProviderCapabilities{Terminal: true, Files: true, Git: true},
	}}
	fixture.operations.onProvision = func() { fixture.note("provision") }
	fixture.operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}
	fixture.operations.connectionResponse = &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
		BaseUrl: "https://executor.example", ExpiresAt: "2099-01-01T00:00:00Z", Generation: "generation-1",
	}}
	fixture.runtime = NewPluginRemoteExecutor(fixture.operations, newTestLogger())
	fixture.runtime.SetHostAPIProbe(forwardedHostAPIURL, fixture.registry)
	fixture.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	fixture.runtime.hostAPIProbeInterval = time.Millisecond

	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runtime/probe-host-api":
			fixture.serveProbe(w, r, reply)
		case "/api/v1/instances":
			fixture.note("create_instance")
			var instance agentctl.CreateInstanceRequest
			_ = json.NewDecoder(r.Body).Decode(&instance)
			_ = json.NewEncoder(w).Encode(agentctl.CreateInstanceResponse{ID: instance.ID, Port: testPluginExecutorInstancePort})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(control.Close)
	controlURL, _ := url.Parse(control.URL)
	controlPort, _ := strconv.Atoi(controlURL.Port())
	fixture.runtime.newAgentctlControlClient = func(_ context.Context, _ agentctl.ConnectionLeaseResolver, log *logger.Logger, _ string) (*agentctl.ControlClient, string, error) {
		return agentctl.NewControlClient(controlURL.Hostname(), controlPort, log), "agentctl-token", nil
	}
	fixture.runtime.ready = func(context.Context, *agentctl.Client) error { return nil }
	fixture.request = pluginExecutorLaunchRequest(provider)
	fixture.request.Env = map[string]string{}
	fixture.request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	fixture.request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	return fixture
}

func (f *forwardedFixture) note(entry string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.order = append(f.order, entry)
}

func (f *forwardedFixture) serveProbe(w http.ResponseWriter, r *http.Request, reply probeReply) {
	var request agentctltypes.HostAPIProbeRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	f.mu.Lock()
	f.attempts++
	attempt := f.attempts
	f.probeURLs = append(f.probeURLs, request.URL)
	f.order = append(f.order, "probe")
	f.mu.Unlock()
	if reply != nil && reply(attempt, request, w) {
		return
	}
	_ = json.NewEncoder(w).Encode(agentctltypes.HostAPIProbeResponse{StatusCode: f.hitHost(request.Token)})
}

// hitHost performs the worker's request against the host registry.
func (f *forwardedFixture) hitHost(token string) int {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(runtimeprobe.PingPath, f.registry.Handler())
	ping := httptest.NewRequest(http.MethodGet, runtimeprobe.PingPath, http.NoBody)
	ping.Header.Set(runtimeprobe.HeaderName, token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, ping)
	return recorder.Code
}

func (f *forwardedFixture) attemptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts
}

func TestForwardedLaunchSkipsThePublicURLCheckProbesAndProceeds(t *testing.T) {
	fixture := newForwardedFixture(t, forwardedProvider(), nil)
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()

	bootstrap := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(fixture.operations.provisionRequest.GetBootstrapJson()), &bootstrap))
	require.Equal(t, forwardedHostAPIURL, bootstrap["host_api_url"])
	require.NotContains(t, bootstrap, "api_url", "a forwarded launch must not hand the worker a public URL it cannot use")
	require.Equal(t, 1, fixture.attemptCount())
	require.Equal(t, []string{forwardedHostAPIURL + runtimeprobe.PingPath}, fixture.probeURLs)
	require.Equal(t, []string{"provision", "probe", "create_instance"}, fixture.order, "the probe runs after provisioning and before the instance is created")
	require.Zero(t, fixture.registry.Active(), "the probe token must be deleted when the probe ends")
}

func TestForwardedLaunchRetriesOnceAndSucceeds(t *testing.T) {
	fixture := newForwardedFixture(t, forwardedProvider(), func(attempt int, _ agentctltypes.HostAPIProbeRequest, w http.ResponseWriter) bool {
		if attempt == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return true
		}
		return false
	})
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()
	require.Equal(t, 2, fixture.attemptCount())
	require.Zero(t, fixture.registry.Active())
}

func TestForwardedLaunchCountsAMatchedRequestWhoseReplyWasLost(t *testing.T) {
	var fixture *forwardedFixture
	fixture = newForwardedFixture(t, forwardedProvider(), func(_ int, request agentctltypes.HostAPIProbeRequest, w http.ResponseWriter) bool {
		require.Equal(t, http.StatusNoContent, fixture.hitHost(request.Token), "the request itself reached the host")
		w.WriteHeader(http.StatusBadGateway)
		return true
	})
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()
	require.Equal(t, 1, fixture.attemptCount(), "a matched request is success even when the 204 reply is lost, so no retry runs against a spent token")
}

func TestForwardedLaunchRollsBackWithApiUnreachableWhenBothAttemptsFail(t *testing.T) {
	fixture := newForwardedFixture(t, forwardedProvider(), func(_ int, _ agentctltypes.HostAPIProbeRequest, w http.ResponseWriter) bool {
		_ = json.NewEncoder(w).Encode(agentctltypes.HostAPIProbeResponse{StatusCode: http.StatusUnauthorized})
		return true
	})
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.Error(t, err)
	require.Nil(t, instance)
	require.Equal(t, 2, fixture.attemptCount(), "two attempts, no more")
	require.NotNil(t, fixture.operations.destroyRequest, "provisioning must roll back")
	require.Equal(t, "api_unreachable", fixture.operations.destroyRequest.GetCleanupReason())
	require.NotContains(t, fixture.order, "create_instance")
	require.Zero(t, fixture.registry.Active(), "the token is deleted on a failed probe too")
}

func TestForwardedLaunchWithoutAKnownHostAPIURLFailsBeforeAnyProviderCall(t *testing.T) {
	fixture := newForwardedFixture(t, forwardedProvider(), nil)
	fixture.runtime.SetHostAPIProbe("", fixture.registry)
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.Error(t, err)
	require.Nil(t, instance)
	require.Nil(t, fixture.operations.provisionRequest, "no provider call before the host API URL is known")
	require.Zero(t, fixture.attemptCount())
}

func TestHostPublicLaunchIsUnchanged(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	provider.RuntimeAPIURL = "host_public"
	fixture := newForwardedFixture(t, provider, nil)
	fixture.request.Env = map[string]string{envKeyKandevAPIURL: "https://kandev.example/api/v1"}
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()
	bootstrap := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(fixture.operations.provisionRequest.GetBootstrapJson()), &bootstrap))
	require.Equal(t, "https://kandev.example/api/v1", bootstrap["api_url"])
	require.NotContains(t, bootstrap, "host_api_url")
	require.Zero(t, fixture.attemptCount(), "a host_public launch never probes")

	loopback := newForwardedFixture(t, provider, nil)
	loopback.request.Env = map[string]string{envKeyKandevAPIURL: "https://127.0.0.1/api/v1"}
	_, err = loopback.runtime.CreateInstance(context.Background(), loopback.request)
	require.Error(t, err, "https validation still rejects loopback")
	require.Nil(t, loopback.operations.provisionRequest)
}
