package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	managedmcp "github.com/kandev/kandev/internal/mcp/managed"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/stretchr/testify/require"
)

func TestCloudCallbackAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	harness := newBootStateTestHarness(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stdout"})
	require.NoError(t, err)

	for _, test := range []struct {
		name    string
		enabled bool
		status  int
	}{
		{name: "disabled route", enabled: false, status: http.StatusNotFound},
		{name: "enabled route requires grant", enabled: true, status: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			gateway := gateways.NewGateway(log)
			registerCursorCloudManagedMCP(routeParams{
				router: router, gateway: gateway, taskRepo: harness.taskRepo, log: log,
				features: config.FeaturesConfig{CursorCloud: test.enabled},
			}, mcpscope.NewResolver(harness.taskRepo, nil, func() bool { return false }, log))
			request := httptest.NewRequest(http.MethodPost, managedmcp.ManagedCallbackPath+"grant-1", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, test.status, response.Code, response.Body.String())
		})
	}
}
