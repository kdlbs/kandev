package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

// MaxExecutorTunnelFrameBytes is the largest data payload of one tunnel frame.
const MaxExecutorTunnelFrameBytes = 64 << 10

type ExecutorTunnelFrame = pluginv1.ExecutorTunnelFrame
type ExecutorTunnelHeader = pluginv1.ExecutorTunnelHeader
type ExecutorTunnelAck = pluginv1.ExecutorTunnelAck
type ExecutorTunnelClose = pluginv1.ExecutorTunnelClose

// ExecutorTunnelStream is the provider's end of one host-opened tunnel.
type ExecutorTunnelStream interface {
	Context() context.Context
	Recv() (*ExecutorTunnelFrame, error)
	Send(*ExecutorTunnelFrame) error
}

// ExecutorTunnelClientStream is the host's end of one tunnel.
type ExecutorTunnelClientStream = pluginv1.Plugin_OpenExecutorTunnelClient

// ExecutorTunnelPlugin is an optional extension for a provider that requires
// the tunnel_transport capability. The first received frame is the header; the
// provider sends exactly one ack frame before any data.
type ExecutorTunnelPlugin interface {
	OpenExecutorTunnel(context.Context, ExecutorTunnelStream) error
}

// OpenExecutorTunnel opens the tunnel stream of the plugin process.
func (r *RemotePlugin) OpenExecutorTunnel(ctx context.Context) (ExecutorTunnelClientStream, error) {
	return r.client.OpenExecutorTunnel(ctx)
}

func (s *grpcPluginServer) OpenExecutorTunnel(stream pluginv1.Plugin_OpenExecutorTunnelServer) error {
	provider, ok := s.impl.(ExecutorTunnelPlugin)
	if !ok {
		return unsupportedPluginExtension("executor tunnel")
	}
	return provider.OpenExecutorTunnel(stream.Context(), stream)
}
