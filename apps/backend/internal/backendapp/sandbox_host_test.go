package backendapp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agent/sandboxruntime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/stretchr/testify/require"
)

func useSandboxHost(t *testing.T, goos string) {
	t.Helper()
	previous := sandboxHostOS
	sandboxHostOS = goos
	t.Cleanup(func() { sandboxHostOS = previous })
}

func identityServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/identity", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func agentConfig(t *testing.T, server *httptest.Server) *config.Config {
	t.Helper()
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	host, portText, err := net.SplitHostPort(address.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return &config.Config{Agent: config.AgentConfig{StandaloneHost: host, StandalonePort: port}}
}

func transportFor(t *testing.T, server *httptest.Server) *runtime.AgentCtlClient {
	t.Helper()
	address, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(address.Port())
	require.NoError(t, err)
	return client.NewClient("127.0.0.1", port, testLogger(t))
}

func TestDarwinAdmissionRequiresTheAdvertisedLaunchCapability(t *testing.T) {
	useSandboxHost(t, "darwin")
	cases := map[string]struct {
		status int
		body   string
		err    bool
	}{
		"advertised":         {http.StatusOK, `{"capabilities":["sandbox-launch.v1"]}`, false},
		"absent":             {http.StatusOK, `{"capabilities":["other"]}`, true},
		"read failure":       {http.StatusInternalServerError, ``, true},
		"malformed":          {http.StatusOK, `{`, true},
		"legacy no route":    {http.StatusNotFound, ``, true},
		"empty capabilities": {http.StatusOK, `{}`, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := identityServer(t, tc.status, tc.body)
			err := admitSandboxHost(t.Context(), transportFor(t, server))
			if tc.err {
				require.ErrorIs(t, err, policy.ErrHostUnsupported)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestLinuxAdmissionReadsNoCapability(t *testing.T) {
	useSandboxHost(t, "linux")
	require.NoError(t, admitSandboxHost(t.Context(), nil))
	require.NoError(t, checkManagedSandboxHost(t.Context(), &config.Config{}))
}

func TestManagedHostCheckReadsIdentityOnEveryCall(t *testing.T) {
	useSandboxHost(t, "darwin")
	calls := 0
	body := `{"capabilities":["sandbox-launch.v1"]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	cfg := agentConfig(t, server)
	require.NoError(t, checkManagedSandboxHost(t.Context(), cfg))
	body = `{"capabilities":[]}`
	require.ErrorIs(t, checkManagedSandboxHost(t.Context(), cfg), policy.ErrHostUnsupported, "a later probe loss is not hidden by a cache")
	require.Equal(t, 2, calls)
	server.Close()
	require.ErrorIs(t, checkManagedSandboxHost(t.Context(), cfg), policy.ErrHostUnsupported, "an unreachable agentctl is unsupported")
}

func TestDarwinUtilityPreparationIsRefused(t *testing.T) {
	factory := sandboxRuntimeFactory{runtime: &sandboxruntime.Manager{}}
	transport := client.NewClient("127.0.0.1", 1, testLogger(t))
	useSandboxHost(t, "darwin")
	_, err := factory.prepareUtility(t.Context(), nil, &utility.InferenceConfigDTO{}, transport)
	require.ErrorIs(t, err, runtime.ErrSandboxLaunchUnsupported)
	useSandboxHost(t, "linux")
	_, err = factory.prepareUtility(t.Context(), nil, &utility.InferenceConfigDTO{}, transport)
	require.ErrorIs(t, err, policy.ErrIneligible, "Linux keeps its existing selection refusal")
}
