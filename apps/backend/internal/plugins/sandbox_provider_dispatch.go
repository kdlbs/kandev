package plugins

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type sandboxRequest interface {
	proto.Message
	GetContext() *pluginsdk.SandboxRequestContext
}

type sandboxResponse interface {
	proto.Message
	GetContext() *pluginsdk.SandboxResponseContext
}

func invokeSandboxProvider[Q sandboxRequest, R sandboxResponse](
	ctx context.Context, service *Service, binding SandboxProviderBinding, request Q, timeout time.Duration,
	call func(*pluginsdk.RemotePlugin, context.Context, Q) (R, error),
) (R, error) {
	var empty R
	if !service.sandboxEnabled() {
		return empty, ErrSandboxProviderUnavailable
	}
	if !validSandboxMessage(request) || !validSandboxRequestPayload(request) ||
		!validSandboxRequestContext(request.GetContext(), binding) {
		return empty, ErrSandboxProviderUnavailable
	}
	ctx, cancel := sandboxCallContext(ctx, request.GetContext(), timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return empty, errors.Join(ErrSandboxProviderUnavailable, err)
	}
	_, release, err := service.admitSandboxProvider(binding)
	if err != nil {
		return empty, err
	}
	defer release()
	ctx, finish, err := service.beginProviderDispatch(ctx, binding.PluginID)
	if err != nil {
		return empty, ErrSandboxProviderUnavailable
	}
	defer finish()
	remote, running := service.pluginRemote(binding.PluginID)
	if !running || remote == nil {
		return empty, ErrSandboxProviderUnavailable
	}
	frozen := proto.Clone(request).(Q)
	deadline, _ := ctx.Deadline()
	frozen.GetContext().DeadlineUnixMs = deadline.UnixMilli()
	reply, err := call(remote, ctx, frozen)
	if err != nil || ctx.Err() != nil {
		return empty, errors.Join(ErrSandboxProviderUnavailable, ctx.Err())
	}
	if !validSandboxMessage(reply) ||
		!validSandboxResponseContext(reply.GetContext(), frozen.GetContext()) {
		return empty, errSandboxProviderReply
	}
	if err := validateSandboxReplyPayload(frozen, reply); err != nil {
		return empty, err
	}
	return reply, nil
}

func validSandboxMessage(message proto.Message) bool {
	return message.ProtoReflect().IsValid() && proto.Size(message) <= maxSandboxMessageBytes &&
		!hasUnknownSandboxFields(message.ProtoReflect())
}

func hasUnknownSandboxFields(message protoreflect.Message) bool {
	if len(message.GetUnknown()) != 0 {
		return true
	}
	unknown := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for index := 0; index < list.Len(); index++ {
				if hasUnknownSandboxFields(list.Get(index).Message()) {
					unknown = true
					return false
				}
			}
		} else {
			unknown = hasUnknownSandboxFields(value.Message())
		}
		return !unknown
	})
	return unknown
}

func sandboxCallContext(ctx context.Context, request *pluginsdk.SandboxRequestContext, timeout time.Duration) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(timeout)
	if request.DeadlineUnixMs > 0 && time.UnixMilli(request.DeadlineUnixMs).Before(deadline) {
		deadline = time.UnixMilli(request.DeadlineUnixMs)
	}
	return context.WithDeadline(ctx, deadline)
}

func validSandboxRequestContext(value *pluginsdk.SandboxRequestContext, binding SandboxProviderBinding) bool {
	return value != nil && value.ProtocolMajor == manifest.SandboxProviderProtocolMajor &&
		value.InstallationId != "" && value.InstallationId == binding.InstallationID &&
		value.ProviderKey != "" && value.ProviderKey == binding.ProviderKey &&
		value.OperationId != "" && value.AttemptId != "" && value.ExecutionId != ""
}

func validSandboxResponseContext(value *pluginsdk.SandboxResponseContext, request *pluginsdk.SandboxRequestContext) bool {
	return value != nil && value.SchemaVersion == manifest.SandboxProviderProtocolMajor &&
		value.OperationId == request.OperationId && value.ErrorCode == ""
}

func (s *Service) admitSandboxProvider(binding SandboxProviderBinding) (*store.Record, func(), error) {
	record, err := s.Get(binding.PluginID)
	if err != nil {
		return nil, nil, ErrSandboxProviderUnavailable
	}
	record, release, err := s.beginPluginDispatch(binding.PluginID, dispatchGeneration(record))
	if err != nil {
		return nil, nil, ErrSandboxProviderUnavailable
	}
	if !sandboxBindingMatches(record, binding) {
		release()
		return nil, nil, ErrSandboxProviderUnavailable
	}
	return record, release, nil
}

func sandboxBindingMatches(record *store.Record, binding SandboxProviderBinding) bool {
	if record == nil || !record.Capabilities.SandboxProvider || record.InstallationID != binding.InstallationID ||
		record.Version != binding.Version || record.PackageDigest != binding.PackageDigest || !record.InstalledAt.Equal(binding.InstalledAt) {
		return false
	}
	for _, provider := range record.SandboxProviders {
		if provider.Key == binding.ProviderKey && provider.ProtocolMajor == manifest.SandboxProviderProtocolMajor {
			return true
		}
	}
	return false
}
