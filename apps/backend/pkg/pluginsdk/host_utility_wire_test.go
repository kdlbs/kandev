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

type utilityRPCProbeServer struct {
	pluginv1.UnimplementedHostServer
	oldCalls     int
	optionsCalls int
}

func (s *utilityRPCProbeServer) InvokeUtilityAgent(context.Context, *pluginv1.InvokeUtilityAgentRequest) (*pluginv1.InvokeUtilityAgentResponse, error) {
	s.oldCalls++
	return &pluginv1.InvokeUtilityAgentResponse{Text: "old/default"}, nil
}

func (s *utilityRPCProbeServer) InvokeUtilityAgentWithOptions(context.Context, *pluginv1.InvokeUtilityAgentWithOptionsRequest) (*pluginv1.InvokeUtilityAgentResponse, error) {
	s.optionsCalls++
	return nil, status.Error(codes.Unimplemented, "new rpc unavailable")
}

func dialRawHostClient(t *testing.T, register func(*grpc.Server)) (pluginv1.HostClient, Host) {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	register(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return pluginv1.NewHostClient(conn), newHostClient(conn)
}

func TestInvokeUtilityAgent_ExplicitOverrideDoesNotDowngradeOnUnimplemented(t *testing.T) {
	probe := &utilityRPCProbeServer{}
	_, host := dialRawHostClient(t, func(server *grpc.Server) {
		pluginv1.RegisterHostServer(server, probe)
	})

	_, err := host.InvokeUtilityAgent(context.Background(), "prompt", UtilityAgentOptions{ProfileID: "profile-1"})
	require.Equal(t, codes.Unimplemented, status.Code(err))
	require.Equal(t, 1, probe.optionsCalls)
	require.Zero(t, probe.oldCalls)
}

func TestInvokeUtilityAgent_LegacyWireUsesDefault(t *testing.T) {
	impl := &dataRecordingHost{utilityText: "default completion"}
	client, _ := dialRawHostClient(t, func(server *grpc.Server) {
		registerHostServer(server, impl)
	})

	response, err := client.InvokeUtilityAgent(context.Background(), &pluginv1.InvokeUtilityAgentRequest{Prompt: "prompt"})
	require.NoError(t, err)
	require.Equal(t, "default completion", response.GetText())
	require.Empty(t, impl.lastUtilityOptions)
}
