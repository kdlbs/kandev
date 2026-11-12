package instance

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// captureInstanceConfigs records the config each created instance's handler
// was built with.
func captureInstanceConfigs(mgr *Manager) *[]*config.InstanceConfig {
	var configs []*config.InstanceConfig
	mgr.SetServerFactory(func(cfg *config.InstanceConfig, _ *process.Manager, _ *logger.Logger) http.Handler {
		configs = append(configs, cfg)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})
	return &configs
}

func TestCreateInstance_CredentialIsRandomAndScoped(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 2)
	configs := captureInstanceConfigs(mgr)
	ctx := context.Background()

	_, err := mgr.CreateInstance(ctx, &CreateRequest{WorkspacePath: t.TempDir(), BuiltinMCPAuthEnforced: true})
	require.NoError(t, err)
	_, err = mgr.CreateInstance(ctx, &CreateRequest{WorkspacePath: t.TempDir()})
	require.NoError(t, err)
	require.Len(t, *configs, 2)
	first, second := (*configs)[0], (*configs)[1]

	raw, err := base64.RawURLEncoding.DecodeString(first.BuiltinMCPCredential)
	require.NoError(t, err)
	require.Len(t, raw, 32)
	require.NotEqual(t, first.BuiltinMCPCredential, second.BuiltinMCPCredential)
	require.NotEmpty(t, second.BuiltinMCPCredential, "credential is issued even without enforcement")
	require.True(t, first.BuiltinMCPAuthEnforced)
	require.False(t, second.BuiltinMCPAuthEnforced)

	for _, info := range mgr.ListInstances() {
		for _, value := range info.Env {
			require.NotContains(t, value, first.BuiltinMCPCredential)
		}
	}
}

func TestCreateInstance_PortReuseIssuesNewCredential(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 1)
	configs := captureInstanceConfigs(mgr)
	ctx := context.Background()

	first, err := mgr.CreateInstance(ctx, &CreateRequest{WorkspacePath: t.TempDir(), BuiltinMCPAuthEnforced: true})
	require.NoError(t, err)
	require.NoError(t, mgr.StopInstance(ctx, first.ID))
	second, err := mgr.CreateInstance(ctx, &CreateRequest{WorkspacePath: t.TempDir(), BuiltinMCPAuthEnforced: true})
	require.NoError(t, err)

	require.Equal(t, first.Port, second.Port, "the single-port range forces reuse")
	require.Len(t, *configs, 2)
	require.NotEqual(t, (*configs)[0].BuiltinMCPCredential, (*configs)[1].BuiltinMCPCredential)
}

func TestCreateInstance_BuiltinEntriesCarryInstanceCredential(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 1)
	configs := captureInstanceConfigs(mgr)
	_, err := mgr.CreateInstance(context.Background(), &CreateRequest{WorkspacePath: t.TempDir()})
	require.NoError(t, err)
	require.Len(t, *configs, 1)
	cfg := (*configs)[0]
	want := config.BuiltinMCPHeaders(cfg.BuiltinMCPCredential)
	require.NotEmpty(t, want)
	found := 0
	for _, server := range cfg.McpServers {
		if server.Name == "kandev" {
			require.Equal(t, want, server.Headers)
			found++
		}
	}
	require.Equal(t, 2, found)
}
