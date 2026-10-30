package pluginsdk

import (
	"context"
	"sync"

	"github.com/google/uuid"
	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const pluginCallMetadata = "x-kandev-plugin-call"

// pluginHandlerReferenceKey carries the reference installed by the SDK at the
// plugin RPC entry point. Incoming metadata is forgeable by plugin code; the
// private value is not part of the public context surface.
type pluginHandlerReferenceKey struct{}

// pluginHandlerReference is a function value so accidental fmt.Sprint / %#v on
// a handler context prints an opaque pointer. It is not a secrecy boundary: a
// handler that wraps the context can observe Value and recover the reference.
type pluginHandlerReference func() string

func withPluginCallReference(ctx context.Context) context.Context {
	incoming, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	refs := incoming.Get(pluginCallMetadata)
	if len(refs) != 1 || refs[0] == "" {
		return ctx
	}
	clean := incoming.Copy()
	clean.Delete(pluginCallMetadata)
	reference := refs[0]
	return context.WithValue(metadata.NewIncomingContext(ctx, clean), pluginHandlerReferenceKey{}, pluginHandlerReference(func() string {
		return reference
	}))
}

// HostCallContextPolicy is implemented by the host application. The SDK does
// not interpret task identities; it carries the original server context only.
type HostCallContextPolicy interface {
	HostCallContextEnabled() bool
	ValidateHostCallContext(context.Context, bool) error
}

type pluginCallOrigins struct {
	mu     sync.Mutex
	policy HostCallContextPolicy
	live   map[string]context.Context
}

func newPluginCallOrigins(host Host) *pluginCallOrigins {
	policy, _ := host.(HostCallContextPolicy)
	return &pluginCallOrigins{policy: policy, live: make(map[string]context.Context)}
}

func (o *pluginCallOrigins) enabled() bool {
	return o.policy != nil && o.policy.HostCallContextEnabled()
}

func (o *pluginCallOrigins) bind(ctx context.Context, agentTool bool) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := o.policy.ValidateHostCallContext(ctx, agentTool); err != nil {
		return nil, nil, status.Error(codes.PermissionDenied, "plugin caller context is unavailable")
	}
	origin, cancel := context.WithCancel(ctx)
	// Tool InvocationID is caller correlation data, not a unique lifetime.
	// A new server reference prevents late callbacks from following its reuse.
	reference := uuid.NewString()
	o.mu.Lock()
	o.live[reference] = origin
	o.mu.Unlock()
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(pluginCallMetadata, reference)
	return metadata.NewOutgoingContext(ctx, md), func() {
		o.mu.Lock()
		delete(o.live, reference)
		o.mu.Unlock()
		cancel()
	}, nil
}

// Plugin-owned startup storage/configuration and capability-scoped Host
// operations keep their existing checks; conversation domain operations are
// classified in host_call_classification.go.
//
// A live call reference is a bearer capability for its lifetime. This wrapper
// prevents accidental context drops and expired or returned replays. In-process
// code that bypasses the wrapper and presents a live reference is exercising
// that invocation's existing authority; no cryptographic binding is added.
func (o *pluginCallOrigins) intercept(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if !o.enabled() {
		return handler(ctx, req)
	}
	emitting := info.FullMethod == pluginv1.Host_EmitEvent_FullMethodName
	if !emitting && !hostOperationNeedsCaller(info.FullMethod) {
		return handler(ctx, req)
	}
	md, _ := metadata.FromIncomingContext(ctx)
	refs := md.Get(pluginCallMetadata)
	if len(refs) != 1 || refs[0] == "" {
		if emitting {
			return handler(unidentifiedHostCallContext(ctx), req)
		}
		return nil, status.Error(codes.PermissionDenied, "plugin Host call requires a live caller context")
	}
	o.mu.Lock()
	origin := o.live[refs[0]]
	o.mu.Unlock()
	if origin == nil || origin.Err() != nil {
		if emitting {
			return handler(unidentifiedHostCallContext(ctx), req)
		}
		return nil, status.Error(codes.PermissionDenied, "plugin caller context has expired")
	}
	if !emitting && origin.Value(unidentifiedHostCallKey{}) != nil {
		return nil, status.Error(codes.PermissionDenied, "plugin event caller context is unavailable")
	}
	call, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(origin, cancel)
	defer stop()
	defer cancel()
	caller := pluginCallerContext{Context: call, origin: origin}
	return handler(context.WithValue(caller, hostCallReferenceKey{}, refs[0]), req)
}

type pluginCallerContext struct {
	context.Context
	origin context.Context
}

func (c pluginCallerContext) Value(key any) any {
	if value := c.origin.Value(key); value != nil {
		return value
	}
	return c.Context.Value(key)
}

type pluginCallClientConn struct {
	grpc.ClientConnInterface
	origins *pluginCallOrigins
}

func (c pluginCallClientConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	if !c.origins.enabled() {
		return c.ClientConnInterface.Invoke(ctx, method, args, reply, opts...)
	}
	call, release, err := c.origins.bind(ctx, method == pluginv1.Plugin_InvokeAgentTool_FullMethodName)
	if err != nil {
		return err
	}
	defer release()
	return c.ClientConnInterface.Invoke(call, method, args, reply, opts...)
}

type hostCallClientConn struct{ grpc.ClientConnInterface }

func (c hostCallClientConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	// Only the private reference installed at the SDK handler entry may reach
	// the Host. Incoming and outgoing metadata are plugin-controlled.
	outgoing, _ := metadata.FromOutgoingContext(ctx)
	outgoing = outgoing.Copy()
	outgoing.Delete(pluginCallMetadata)
	if get, ok := ctx.Value(pluginHandlerReferenceKey{}).(pluginHandlerReference); ok {
		if ref := get(); ref != "" {
			outgoing.Set(pluginCallMetadata, ref)
		}
	}
	ctx = metadata.NewOutgoingContext(ctx, outgoing)
	return c.ClientConnInterface.Invoke(ctx, method, args, reply, opts...)
}
