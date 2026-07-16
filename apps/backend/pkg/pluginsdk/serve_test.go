package pluginsdk

import (
	"context"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
)

// fakeAuthorPlugin is a minimal author-facing Plugin used to exercise the
// real go-plugin gRPC + broker wiring end to end (no subprocess spawn —
// hcplugin.TestPluginGRPCConn wires a real unix-socket grpc.Server/Client
// pair with a real GRPCBroker, matching production transport behavior).
type fakeAuthorPlugin struct {
	UnimplementedPlugin
	events []*Event
}

func (p *fakeAuthorPlugin) OnEvent(_ context.Context, e *Event) error {
	p.events = append(p.events, e)
	return nil
}

func (p *fakeAuthorPlugin) InvokeTool(_ context.Context, req *ToolRequest) (*ToolResponse, error) {
	return &ToolResponse{Output: map[string]any{"echo": req.ToolName}}, nil
}

func (p *fakeAuthorPlugin) HandleWebhook(_ context.Context, req *WebhookRequest) (*WebhookResponse, error) {
	return &WebhookResponse{Status: 200, Body: append([]byte("got:"), req.Body...)}, nil
}

// TestServe_EndToEnd exercises the same GRPCPlugin wiring Serve() (plugin
// side) and kandev's runtime manager (host side, via plugin.NewClient)
// use, over a real go-plugin gRPC connection + broker
// (hcplugin.TestPluginGRPCConn). It covers all three Plugin RPCs plus the
// Host broker round trip (§3/§4 of docs/plans/plugins/GRPC-CONTRACT.md).
func TestServe_EndToEnd(t *testing.T) {
	author := &fakeAuthorPlugin{}
	hostImpl := &recordingHost{
		getStateFn: func(_ context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
			if scope == "task" && scopeID == "t1" && key == "k1" {
				return map[string]any{"greeting": "hi"}, true, nil
			}
			return nil, false, nil
		},
	}
	// hcplugin.TestPluginGRPCConn dispenses/serves the SAME Plugin value on
	// both ends of the harness (there's no real process boundary), so one
	// GRPCPlugin with both Impl and Host set plays both roles: GRPCServer
	// runs as the "plugin subprocess" would, GRPCClient runs as "kandev"
	// would. In production these are two separate processes each setting
	// only one of Impl/Host.
	gp := &GRPCPlugin{Impl: author, Host: hostImpl, HostDialTimeout: 5 * time.Second}

	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		PluginMapKey: gp,
	})
	defer func() { _ = client.Close() }()
	defer server.Stop()

	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote, ok := raw.(*RemotePlugin)
	require.True(t, ok, "Dispense(%q) should return *RemotePlugin, got %T", PluginMapKey, raw)

	t.Run("DeliverEvent", func(t *testing.T) {
		err := remote.DeliverEvent(context.Background(), &Event{
			EventID:   "e1",
			EventType: "task.created",
			Payload:   map[string]any{"a": float64(1)},
		})
		require.NoError(t, err)
		require.Len(t, author.events, 1)
		require.Equal(t, "e1", author.events[0].EventID)
		require.Equal(t, map[string]any{"a": float64(1)}, author.events[0].Payload)
	})

	t.Run("InvokeTool", func(t *testing.T) {
		resp, err := remote.InvokeTool(context.Background(), &ToolRequest{ToolName: "search"})
		require.NoError(t, err)
		require.Equal(t, "search", resp.Output["echo"])
	})

	t.Run("HandleWebhook", func(t *testing.T) {
		resp, err := remote.HandleWebhook(context.Background(), &WebhookRequest{Body: []byte("hi")})
		require.NoError(t, err)
		require.Equal(t, int32(200), resp.Status)
		require.Equal(t, []byte("got:hi"), resp.Body)
	})

	t.Run("HostBrokerRoundTrip", func(t *testing.T) {
		require.Eventually(t, func() bool {
			return author.Host() != nil
		}, 5*time.Second, 10*time.Millisecond, "Serve-equivalent GRPCServer should inject Host via the broker")

		value, found, err := author.Host().GetState(context.Background(), "task", "t1", "k1")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, map[string]any{"greeting": "hi"}, value)

		_, found, err = author.Host().GetState(context.Background(), "task", "t1", "missing")
		require.NoError(t, err)
		require.False(t, found)
	})
}
