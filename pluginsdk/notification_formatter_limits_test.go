package pluginsdk

import (
	"context"
	"strings"
	"testing"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
)

type largeNotificationFormatterFixture struct {
	notificationFormatterFixture
	size int
}

func (f *largeNotificationFormatterFixture) FormatNotification(context.Context, *NotificationFormatRequest) (*NotificationFormatResponse, error) {
	return &NotificationFormatResponse{Body: strings.Repeat("x", f.size)}, nil
}

func (f *largeNotificationFormatterFixture) InspectNotificationConfiguration(_ context.Context, req *NotificationConfigurationRequest) (*NotificationConfigurationResponse, error) {
	result := &NotificationConfigurationResponse{Revision: req.Revision, Digest: strings.Repeat("a", 64)}
	unknown := protowire.AppendTag(nil, 100, protowire.BytesType)
	result.ProtoReflect().SetUnknown(protowire.AppendBytes(unknown, []byte(strings.Repeat("x", f.size))))
	return result, nil
}

func TestNotificationFormatterTransportBoundsResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
		code codes.Code
	}{
		{name: "within-limit", size: 512*1024 - 1024, code: codes.OK},
		{name: "over-limit", size: 512*1024 + 1, code: codes.ResourceExhausted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
				PluginMapKey: &GRPCPlugin{Impl: &largeNotificationFormatterFixture{size: tc.size}},
			})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			raw, err := client.Dispense(PluginMapKey)
			require.NoError(t, err)
			remote := raw.(*RemotePlugin)
			_, err = remote.FormatNotification(t.Context(), &NotificationFormatRequest{
				ContractVersion: 1, ProviderId: "provider", OwnerId: "owner", FormatterKey: "text", ConfigurationRevision: "revision",
			})
			require.Equal(t, tc.code, status.Code(err), "format response limit")
			_, err = remote.InspectNotificationConfiguration(t.Context(), &NotificationConfigurationRequest{
				ProviderId: "provider", OwnerId: "owner", FormatterKey: "text", Revision: "revision",
			})
			require.Equal(t, tc.code, status.Code(err), "configuration response limit")
		})
	}
}
