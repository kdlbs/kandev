package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/runtimeprobe"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

const workerAPIURL = "http://127.0.0.1:41999"

func workerAPIURLProvider() models.ExecutorProvider {
	provider := forwardedProvider()
	provider.RequiredCapabilities = append(provider.RequiredCapabilities,
		models.ExecutorProviderCapabilityRequirement{Name: "worker_api_url", Version: 1})
	return provider
}

func newWorkerURLFixture(t *testing.T, provider models.ExecutorProvider, workerURL string) *forwardedFixture {
	t.Helper()
	fixture := newForwardedFixture(t, provider, nil)
	fixture.operations.provisionResponse.Resource.WorkerApiUrl = workerURL
	return fixture
}

func TestForwardedLaunchProbesTheWorkerURLWhenTheProviderReturnsOne(t *testing.T) {
	fixture := newWorkerURLFixture(t, workerAPIURLProvider(), workerAPIURL)
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()

	require.Equal(t, []string{workerAPIURL + runtimeprobe.PingPath}, fixture.probeURLs,
		"the probe goes to the worker-side forward, not the host's own URL")
	bootstrap := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(fixture.operations.provisionRequest.GetBootstrapJson()), &bootstrap))
	require.Equal(t, forwardedHostAPIURL, bootstrap["host_api_url"], "the bootstrap still carries the host URL")
}

func TestForwardedLaunchWithoutAWorkerURLProbesTheHostURL(t *testing.T) {
	fixture := newWorkerURLFixture(t, workerAPIURLProvider(), "")
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()
	require.Equal(t, []string{forwardedHostAPIURL + runtimeprobe.PingPath}, fixture.probeURLs)
}

func TestForwardedLaunchRefusesAnInvalidWorkerURLAndRollsBack(t *testing.T) {
	for name, value := range map[string]string{
		"not absolute":       "127.0.0.1:41999",
		"https":              "https://127.0.0.1:41999",
		"non loopback":       "http://10.0.0.5:41999",
		"public name":        "http://executor.example:41999",
		"userinfo":           "http://" + "user" + ":" + "pw" + "@" + "127.0.0.1:41999",
		"path":               "http://127.0.0.1:41999/api/v1",
		"query":              "http://127.0.0.1:41999?x=1",
		"fragment":           "http://127.0.0.1:41999#x",
		"no port":            "http://127.0.0.1",
		"loopback lookalike": "http://127.0.0.1.example:41999",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newWorkerURLFixture(t, workerAPIURLProvider(), value)
			instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
			require.Error(t, err)
			require.Nil(t, instance)
			require.Zero(t, fixture.attemptCount(), "nothing is probed")
			require.NotNil(t, fixture.operations.destroyRequest, "provisioning rolls back")
		})
	}
}

func TestForwardedLaunchAcceptsEachLoopbackSpelling(t *testing.T) {
	for _, value := range []string{"http://127.0.0.1:41999", "http://localhost:41999", "http://[::1]:41999", "http://127.0.0.1:41999/"} {
		t.Run(value, func(t *testing.T) {
			fixture := newWorkerURLFixture(t, workerAPIURLProvider(), value)
			instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
			require.NoError(t, err)
			defer instance.Client.Close()
			require.Len(t, fixture.probeURLs, 1)
		})
	}
}

func TestWorkerURLWithoutTheNegotiatedCapabilityIsRefused(t *testing.T) {
	fixture := newWorkerURLFixture(t, forwardedProvider(), workerAPIURL)
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.Error(t, err)
	require.Nil(t, instance)
	require.Zero(t, fixture.attemptCount(), "a provider that did not opt in never redirects the probe")
	require.NotNil(t, fixture.operations.destroyRequest)
}

func TestWorkerURLOnAHostPublicLaunchIsRefused(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	provider.RuntimeAPIURL = "host_public"
	provider.RequiredCapabilities = []models.ExecutorProviderCapabilityRequirement{
		{Name: "api_url_forward", Version: 1}, {Name: "worker_api_url", Version: 1}}
	fixture := newWorkerURLFixture(t, provider, workerAPIURL)
	fixture.request.Env = map[string]string{envKeyKandevAPIURL: "https://kandev.example/api/v1"}
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.Error(t, err)
	require.Nil(t, instance)
	require.Zero(t, fixture.attemptCount())
}

func TestWorkerURLFailedProbeStillRollsBackWithApiUnreachable(t *testing.T) {
	fixture := newForwardedFixture(t, workerAPIURLProvider(), func(_ int, _ agentctltypes.HostAPIProbeRequest, w http.ResponseWriter) bool {
		_ = json.NewEncoder(w).Encode(agentctltypes.HostAPIProbeResponse{StatusCode: http.StatusBadGateway})
		return true
	})
	fixture.operations.provisionResponse.Resource.WorkerApiUrl = workerAPIURL
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.Error(t, err)
	require.Nil(t, instance)
	require.Equal(t, "api_unreachable", fixture.operations.destroyRequest.GetCleanupReason())
	require.Equal(t, []string{workerAPIURL + runtimeprobe.PingPath, workerAPIURL + runtimeprobe.PingPath}, fixture.probeURLs)
}

func TestResolveWorkerAPIURLNamesItsRefusals(t *testing.T) {
	with := func(raw string) *pluginsdk.ExecutorResourceDescriptor {
		return &pluginsdk.ExecutorResourceDescriptor{WorkerApiUrl: raw}
	}
	declared := []string{"api_url_forward@1", "worker_api_url@1"}

	got, err := resolvePluginExecutorWorkerAPIURL(declared, true, with(""))
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = resolvePluginExecutorWorkerAPIURL(declared, true, with("http://127.0.0.1:41999/"))
	require.NoError(t, err)
	require.Equal(t, workerAPIURL, got)

	_, err = resolvePluginExecutorWorkerAPIURL([]string{"api_url_forward@1"}, true, with(workerAPIURL))
	require.ErrorIs(t, err, models.ErrExecutorConflict)

	_, err = resolvePluginExecutorWorkerAPIURL(declared, false, with(workerAPIURL))
	require.ErrorIs(t, err, ErrWorkerAPIURLInvalid)

	_, err = resolvePluginExecutorWorkerAPIURL(declared, true, with("http://10.0.0.5:1"))
	require.ErrorIs(t, err, ErrWorkerAPIURLInvalid)
}
