package pluginsdk

import (
	"context"
	"strings"
	"testing"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type notificationFormatterFixture struct{ UnimplementedPlugin }

func (*notificationFormatterFixture) FormatNotification(_ context.Context, req *NotificationFormatRequest) (*NotificationFormatResponse, error) {
	return &NotificationFormatResponse{Custom: true, Title: req.ConfigurationRevision, Body: req.Task.Title}, nil
}

func (*notificationFormatterFixture) InspectNotificationConfiguration(_ context.Context, req *NotificationConfigurationRequest) (*NotificationConfigurationResponse, error) {
	return &NotificationConfigurationResponse{Revision: req.Revision, Digest: strings.Repeat("a", 64)}, nil
}

func TestNotificationFormatterTransport(t *testing.T) {
	client, _ := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		PluginMapKey: &GRPCPlugin{Impl: &notificationFormatterFixture{}},
	})
	// Client.Close owns the server's gRPC shutdown.
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	remote := raw.(*RemotePlugin)
	request := &NotificationFormatRequest{
		ContractVersion: 1, ProviderId: "provider", OwnerId: "owner", FormatterKey: "text",
		ConfigurationRevision: "revision-1", EventType: "session.turn_finished",
		DefaultTitle: "Default", DefaultBody: "Default body",
		Task: &NotificationTaskContext{TaskId: "task", Title: "Literal {{not.reparsed}}", SessionId: "session"},
	}
	result, err := remote.FormatNotification(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.Custom)
	require.Equal(t, "revision-1", result.Title)
	require.Equal(t, "Literal {{not.reparsed}}", result.Body)
	inspected, err := remote.InspectNotificationConfiguration(context.Background(), &NotificationConfigurationRequest{
		ProviderId: "provider", OwnerId: "owner", FormatterKey: "text", Revision: "revision-1",
	})
	require.NoError(t, err)
	require.Equal(t, "revision-1", inspected.Revision)
}

func TestNotificationFormatterRemainsOptional(t *testing.T) {
	server := &grpcPluginServer{impl: &UnimplementedPlugin{}}
	_, err := server.FormatNotification(context.Background(), nil)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = server.InspectNotificationConfiguration(context.Background(), nil)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestNotificationFormatterOutputBoundary(t *testing.T) {
	for name, result := range map[string]*NotificationFormatResponse{
		"missing":   nil,
		"partial":   {Custom: true, Title: "Only title"},
		"multiline": {Custom: true, Title: "injected\r\nheader", Body: "body"},
		"nul":       {Custom: true, Title: "title", Body: "unsafe\x00body"},
		"oversize":  {Custom: true, Title: "title", Body: strings.Repeat("x", 65537)},
	} {
		t.Run(name, func(t *testing.T) { require.Error(t, ValidateNotificationFormat(result)) })
	}
	require.NoError(t, ValidateNotificationFormat(&NotificationFormatResponse{}))
	require.NoError(t, ValidateNotificationFormat(&NotificationFormatResponse{Custom: true, Title: "日本語", Body: "body"}))
}
