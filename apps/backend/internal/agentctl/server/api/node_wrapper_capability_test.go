package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/logger"
)

func identityCapabilities(t *testing.T, handler http.Handler, authorization string) ([]string, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/identity", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusOK, response.Code)
	var identity struct {
		Capabilities []string `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &identity))
	return identity.Capabilities, response.Body.String()
}

func nodeIdentityHandlers(t *testing.T, node launchwrapper.Node) (control, instanceHandler http.Handler) {
	t.Helper()
	log := logger.Default()
	cfg := &config.Config{AuthToken: "test-control", ServerIdentity: "id", NodeLaunchWrapper: node}
	mgr := instance.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	control = NewControlServer(cfg, mgr, log).Router()

	icfg := &config.InstanceConfig{WorkDir: t.TempDir(), AuthToken: "test-control", NodeLaunchWrapper: node}
	instanceHandler = NewServer(icfg, process.NewManager(icfg, log), nil, nil, log).Router()
	return control, instanceHandler
}

// @covers AC-AGENTS-NODEWRAP-004.1 AC-AGENTS-NODEWRAP-004.2
func TestNodeWrapperCapabilityOnlyWhenActive(t *testing.T) {
	spec := launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: []string{"/opt/secret-wrapper-path", "--secret-flag"}}
	control, instanceHandler := nodeIdentityHandlers(t, launchwrapper.Node{Spec: spec, Required: true})
	for name, got := range map[string]func() ([]string, string){
		"control":  func() ([]string, string) { return identityCapabilities(t, control, "") },
		"instance": func() ([]string, string) { return identityCapabilities(t, instanceHandler, "Bearer test-control") },
	} {
		capabilities, body := got()
		require.Contains(t, capabilities, launchwrapper.NodeCapability, name)
		require.NotContains(t, body, "secret-wrapper-path", name)
		require.NotContains(t, body, "secret-flag", name)
	}

	control, instanceHandler = nodeIdentityHandlers(t, launchwrapper.Node{})
	capabilities, _ := identityCapabilities(t, control, "")
	require.Equal(t, SurvivalCapabilities, capabilities, "inactive output is identical to today's")
	capabilities, _ = identityCapabilities(t, instanceHandler, "Bearer test-control")
	require.Equal(t, SurvivalCapabilities, capabilities)
}
