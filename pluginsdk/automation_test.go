package pluginsdk

import (
	"context"
	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

type automationTestPlugin struct{ UnimplementedPlugin }

func (*automationTestPlugin) DescribeAutomationCondition(_ context.Context, req *AutomationConditionRequest) (*AutomationConditionResponse, error) {
	return &AutomationConditionResponse{Available: true, ConnectionId: req.WorkspaceId, ConnectionRevision: "rev-1"}, nil
}
func (*automationTestPlugin) VerifyAutomationWebhook(_ context.Context, req *AutomationWebhookRequest) (*AutomationWebhookResponse, error) {
	return &AutomationWebhookResponse{Outcome: "accepted", EventKind: req.ConditionKey, Data: req.Body}, nil
}
func TestAutomationAdapterTransport(t *testing.T) {
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{PluginMapKey: &GRPCPlugin{Impl: &automationTestPlugin{}}})
	defer func() { require.NoError(t, client.Close()) }()
	defer server.Stop()
	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote := raw.(*RemotePlugin)
	description, err := remote.DescribeAutomationCondition(context.Background(), &AutomationConditionRequest{WorkspaceId: "workspace"})
	require.NoError(t, err)
	require.Equal(t, "workspace", description.ConnectionId)
	body := []byte("{\n  \"message\": \"こんにちは\"\n}")
	result, err := remote.VerifyAutomationWebhook(context.Background(), &AutomationWebhookRequest{ConditionKey: "push", Body: body, Secret: "secret"})
	require.NoError(t, err)
	require.Equal(t, body, result.Data)
	require.Equal(t, "push", result.EventKind)
}
func TestAutomationAdapterOptional(t *testing.T) {
	server := &grpcPluginServer{impl: &UnimplementedPlugin{}}
	_, err := server.DescribeAutomationCondition(context.Background(), nil)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = server.VerifyAutomationWebhook(context.Background(), nil)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
