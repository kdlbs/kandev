package mcp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestChannelBackendClientRedactsPluginInvocationPayload(t *testing.T) {
	core, observed := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	client := NewChannelBackendClient(log)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = client.RequestPayload(ctx, ws.ActionMCPInvokePluginTool, map[string]any{
		"plugin_id": "echo", "arguments": map[string]any{"token": "secret-value"},
	}, nil)

	entries := observed.FilterMessage("sending MCP request through agent stream").All()
	require.Len(t, entries, 1)
	payload, ok := entries[0].ContextMap()["payload"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "echo", payload["plugin_id"])
	_, loggedArguments := payload["arguments"]
	require.False(t, loggedArguments)
}
