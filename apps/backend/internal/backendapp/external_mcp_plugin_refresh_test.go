package backendapp

import (
	"testing"

	"github.com/gin-gonic/gin"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestRegisterExternalMCPSubscribesToPluginCatalogFanout(t *testing.T) {
	log := newTestLogger()
	fanout := newPluginToolCatalogFanout(nil, log)
	cleanups := make([]func() error, 0, 1)
	params := routeParams{
		router:  gin.New(),
		gateway: &gateways.Gateway{Dispatcher: ws.NewDispatcher()},
		services: &Services{
			PluginToolCatalog: fanout,
		},
		log:        log,
		httpPort:   1234,
		addCleanup: func(fn func() error) { cleanups = append(cleanups, fn) },
	}

	registerExternalMCP(params)

	fanout.mu.RLock()
	subscribers := len(fanout.subscribers)
	fanout.mu.RUnlock()
	require.Equal(t, 1, subscribers)
	require.NotEmpty(t, cleanups)

	for i := len(cleanups) - 1; i >= 0; i-- {
		require.NoError(t, cleanups[i]())
	}
	fanout.mu.RLock()
	subscribers = len(fanout.subscribers)
	fanout.mu.RUnlock()
	require.Zero(t, subscribers, "cleanup must unsubscribe the external server")
}
