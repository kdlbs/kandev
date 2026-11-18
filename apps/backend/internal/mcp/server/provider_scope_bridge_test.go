package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestBuiltinGenerationBridgeScopeCannotComeFromToolArguments(t *testing.T) {
	client := NewChannelBackendClient(nil)
	defer client.Close()
	scope := p.ProviderMCPScope{BindingID: "binding", ExecutionID: "execution", HelperID: "helper", ProcessGeneration: 2, HarnessGeneration: 3, ProfileDigest: strings.Repeat("a", 64)}
	for _, scoped := range []bool{false, true} {
		ctx := context.Background()
		if scoped {
			ctx = streams.WithProviderMCPScope(ctx, scope)
		}
		result := make(chan error, 1)
		go func() {
			result <- client.RequestPayload(ctx, "test.action", map[string]any{p.ProviderMCPScopeMetadataKey: scope}, nil)
		}()
		request := <-client.GetRequestChannel()
		if scoped {
			var received p.ProviderMCPScope
			require.NoError(t, json.Unmarshal([]byte(request.Metadata[p.ProviderMCPScopeMetadataKey]), &received))
			require.Equal(t, scope, received)
		} else {
			require.Empty(t, request.Metadata[p.ProviderMCPScopeMetadataKey])
		}
		response, err := ws.NewResponse(request.ID, request.Action, nil)
		require.NoError(t, err)
		client.HandleResponse(response)
		require.NoError(t, <-result)
	}
}
