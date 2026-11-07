package pluginsdk

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// NotificationFormatter is optional. Implementations render only the requested
// immutable configuration and never deliver or mutate a notification.
type NotificationFormatter interface {
	FormatNotification(context.Context, *NotificationFormatRequest) (*NotificationFormatResponse, error)
	InspectNotificationConfiguration(context.Context, *NotificationConfigurationRequest) (*NotificationConfigurationResponse, error)
}

type NotificationFormatRequest = pluginv1.NotificationFormatRequest
type NotificationFormatResponse = pluginv1.NotificationFormatResponse
type NotificationConfigurationRequest = pluginv1.NotificationConfigurationRequest
type NotificationConfigurationResponse = pluginv1.NotificationConfigurationResponse
type NotificationTaskContext = pluginv1.NotificationTaskContext
type NotificationInboxContext = pluginv1.NotificationInboxContext
type NotificationUpdateContext = pluginv1.NotificationUpdateContext

// ValidateNotificationFormat checks the entire replacement pair without
// including personal content in an error. Both peers enforce this boundary.
func ValidateNotificationFormat(result *NotificationFormatResponse) error {
	if result == nil {
		return errors.New("notification formatter returned no result")
	}
	if !result.Custom {
		return nil
	}
	if !validNotificationText(result.Title, 2048) || !validNotificationText(result.Body, 65536) || strings.ContainsAny(result.Title, "\r\n") {
		return errors.New("notification formatter returned invalid content")
	}
	return nil
}

func validNotificationText(value string, limit int) bool {
	return len(value) > 0 && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func (r *RemotePlugin) FormatNotification(ctx context.Context, req *NotificationFormatRequest) (*NotificationFormatResponse, error) {
	return r.client.FormatNotification(ctx, req)
}

func (r *RemotePlugin) InspectNotificationConfiguration(ctx context.Context, req *NotificationConfigurationRequest) (*NotificationConfigurationResponse, error) {
	return r.client.InspectNotificationConfiguration(ctx, req)
}

func (s *grpcPluginServer) FormatNotification(ctx context.Context, req *NotificationFormatRequest) (*NotificationFormatResponse, error) {
	formatter, ok := s.impl.(NotificationFormatter)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "notification formatting is not supported")
	}
	if req == nil || req.ContractVersion != 1 || req.ProviderId == "" || req.OwnerId == "" || req.FormatterKey == "" || req.ConfigurationRevision == "" || proto.Size(req) > 128*1024 {
		return nil, status.Error(codes.InvalidArgument, "invalid notification context")
	}
	result, err := formatter.FormatNotification(withPluginCallReference(ctx), req)
	if err == nil && ValidateNotificationFormat(result) != nil {
		return nil, status.Error(codes.Internal, "invalid notification format result")
	}
	return result, err
}

func (s *grpcPluginServer) InspectNotificationConfiguration(ctx context.Context, req *NotificationConfigurationRequest) (*NotificationConfigurationResponse, error) {
	formatter, ok := s.impl.(NotificationFormatter)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "notification formatting is not supported")
	}
	if req == nil || req.ProviderId == "" || req.OwnerId == "" || req.FormatterKey == "" || req.Revision == "" || proto.Size(req) > 4096 {
		return nil, status.Error(codes.InvalidArgument, "invalid notification configuration reference")
	}
	result, err := formatter.InspectNotificationConfiguration(withPluginCallReference(ctx), req)
	if err == nil && (result == nil || result.Revision != req.Revision || len(result.Digest) != 64) {
		return nil, status.Error(codes.Internal, "invalid notification configuration receipt")
	}
	return result, err
}
