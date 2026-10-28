package pluginsdk

import (
	"context"
	"net"
	"testing"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type tunnelEchoPlugin struct {
	Plugin
}

func (tunnelEchoPlugin) OpenExecutorTunnel(_ context.Context, stream ExecutorTunnelStream) error {
	header, err := stream.Recv()
	if err != nil {
		return err
	}
	if err := stream.Send(&ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Ack{
		Ack: &ExecutorTunnelAck{BindingNonce: header.GetHeader().GetBindingNonce()},
	}}); err != nil {
		return err
	}
	data, err := stream.Recv()
	if err != nil {
		return err
	}
	return stream.Send(&ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Data{Data: data.GetData()}})
}

func dialPluginServer(t *testing.T, impl Plugin) *RemotePlugin {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pluginv1.RegisterPluginServer(srv, &grpcPluginServer{impl: impl})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &RemotePlugin{client: pluginv1.NewPluginClient(conn)}
}

func TestExecutorTunnelRoundTripsAckAndData(t *testing.T) {
	remote := dialPluginServer(t, tunnelEchoPlugin{})
	stream, err := remote.OpenExecutorTunnel(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Header{
		Header: &ExecutorTunnelHeader{Port: 7000, BindingNonce: "n-1"},
	}}))
	ack, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "n-1", ack.GetAck().GetBindingNonce())
	require.NoError(t, stream.Send(&ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Data{Data: []byte("ping")}}))
	data, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, []byte("ping"), data.GetData())
}

func TestExecutorTunnelIsUnimplementedWhenThePluginLacksTheExtension(t *testing.T) {
	remote := dialPluginServer(t, struct{ Plugin }{})
	stream, err := remote.OpenExecutorTunnel(context.Background())
	require.NoError(t, err)
	_, err = stream.Recv()
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
