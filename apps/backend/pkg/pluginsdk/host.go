// host.go implements the Host side of the kandev.plugin.v1.Host service
// (§3 of docs/plans/plugins/GRPC-CONTRACT.md) in both directions:
//
//   - grpcHostClient: used inside the plugin subprocess. Wraps a
//     pluginv1.HostClient dialed over the go-plugin broker (see serve.go)
//     and satisfies the Go-native Host interface that Serve injects into
//     the author's Plugin.
//   - grpcHostServer: used inside kandev. Wraps kandev's own Go-native Host
//     implementation (state store, secrets, event bus) and satisfies the
//     generated pluginv1.HostServer interface so it can be registered on
//     the broker-served grpc.Server that GRPCPlugin.GRPCClient spins up
//     (see serve.go's "Host injection" section).
//
// Both directions share the same Go-native Host interface and the same
// proto conversion helpers in types.go, so kandev's runtime manager
// implements Host exactly once and gets both the client and server wiring
// for free via GRPCPlugin.
package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
)

// Host is the set of operations kandev exposes back to a running plugin,
// per §3's Host service. On the plugin side, Serve injects an
// SDK-provided implementation that proxies these calls to kandev over the
// go-plugin broker. On the kandev side, the runtime manager provides its
// own Go-native implementation of this same interface (backed by the real
// state store / secrets / event bus) and hands it to GRPCPlugin.Host; the
// SDK wraps it into the generated pluginv1.HostServer for registration.
type Host interface {
	// GetState looks up a single state entry. found is false, err is nil
	// when the key does not exist.
	GetState(ctx context.Context, scope, scopeID, key string) (value map[string]any, found bool, err error)

	// SetState upserts a single state entry.
	SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error

	// DeleteState removes a single state entry. Deleting a missing key is
	// not an error.
	DeleteState(ctx context.Context, scope, scopeID, key string) error

	// ListState returns every state entry for a scope/scopeID pair.
	ListState(ctx context.Context, scope, scopeID string) ([]StateEntry, error)

	// RevealSecret resolves a secret reference to its cleartext value.
	RevealSecret(ctx context.Context, ref string) (string, error)

	// EmitEvent publishes a plugin-originated event onto kandev's bus.
	EmitEvent(ctx context.Context, name string, payload map[string]any) error
}

// newHostClient wraps a *grpc.ClientConn (dialed over the go-plugin broker)
// as a Go-native Host implementation.
func newHostClient(conn *grpc.ClientConn) Host {
	return &grpcHostClient{client: pluginv1.NewHostClient(conn)}
}

type grpcHostClient struct {
	client pluginv1.HostClient
}

func (h *grpcHostClient) GetState(ctx context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
	resp, err := h.client.GetState(ctx, &pluginv1.GetStateRequest{Scope: scope, ScopeId: scopeID, Key: key})
	if err != nil {
		return nil, false, err
	}
	if !resp.GetFound() {
		return nil, false, nil
	}
	value, err := structToMap(resp.GetValue())
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (h *grpcHostClient) SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error {
	protoValue, err := mapToStruct(value)
	if err != nil {
		return err
	}
	_, err = h.client.SetState(ctx, &pluginv1.SetStateRequest{Scope: scope, ScopeId: scopeID, Key: key, Value: protoValue})
	return err
}

func (h *grpcHostClient) DeleteState(ctx context.Context, scope, scopeID, key string) error {
	_, err := h.client.DeleteState(ctx, &pluginv1.DeleteStateRequest{Scope: scope, ScopeId: scopeID, Key: key})
	return err
}

func (h *grpcHostClient) ListState(ctx context.Context, scope, scopeID string) ([]StateEntry, error) {
	resp, err := h.client.ListState(ctx, &pluginv1.ListStateRequest{Scope: scope, ScopeId: scopeID})
	if err != nil {
		return nil, err
	}
	return stateEntriesFromProto(resp.GetEntries())
}

func (h *grpcHostClient) RevealSecret(ctx context.Context, ref string) (string, error) {
	resp, err := h.client.RevealSecret(ctx, &pluginv1.RevealSecretRequest{Ref: ref})
	if err != nil {
		return "", err
	}
	return resp.GetValue(), nil
}

func (h *grpcHostClient) EmitEvent(ctx context.Context, name string, payload map[string]any) error {
	protoPayload, err := mapToStruct(payload)
	if err != nil {
		return err
	}
	_, err = h.client.EmitEvent(ctx, &pluginv1.EmitEventRequest{EventName: name, Payload: protoPayload})
	return err
}

var _ Host = (*grpcHostClient)(nil)

// registerHostServer registers a grpc server that dispatches
// kandev.plugin.v1.Host RPCs to impl (kandev's Go-native Host
// implementation), converting proto<->Go-native types at the boundary.
func registerHostServer(s grpc.ServiceRegistrar, impl Host) {
	pluginv1.RegisterHostServer(s, &grpcHostServer{impl: impl})
}

type grpcHostServer struct {
	pluginv1.UnimplementedHostServer
	impl Host
}

func (s *grpcHostServer) GetState(ctx context.Context, req *pluginv1.GetStateRequest) (*pluginv1.GetStateResponse, error) {
	value, found, err := s.impl.GetState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey())
	if err != nil {
		return nil, err
	}
	if !found {
		return &pluginv1.GetStateResponse{Found: false}, nil
	}
	protoValue, err := mapToStruct(value)
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetStateResponse{Found: true, Value: protoValue}, nil
}

func (s *grpcHostServer) SetState(ctx context.Context, req *pluginv1.SetStateRequest) (*pluginv1.SetStateResponse, error) {
	value, err := structToMap(req.GetValue())
	if err != nil {
		return nil, err
	}
	if err := s.impl.SetState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey(), value); err != nil {
		return nil, err
	}
	return &pluginv1.SetStateResponse{}, nil
}

func (s *grpcHostServer) DeleteState(ctx context.Context, req *pluginv1.DeleteStateRequest) (*pluginv1.DeleteStateResponse, error) {
	if err := s.impl.DeleteState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey()); err != nil {
		return nil, err
	}
	return &pluginv1.DeleteStateResponse{}, nil
}

func (s *grpcHostServer) ListState(ctx context.Context, req *pluginv1.ListStateRequest) (*pluginv1.ListStateResponse, error) {
	entries, err := s.impl.ListState(ctx, req.GetScope(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	protoEntries := make([]*pluginv1.StateEntry, len(entries))
	for i := range entries {
		converted, err := entries[i].toProto()
		if err != nil {
			return nil, err
		}
		protoEntries[i] = converted
	}
	return &pluginv1.ListStateResponse{Entries: protoEntries}, nil
}

func (s *grpcHostServer) RevealSecret(ctx context.Context, req *pluginv1.RevealSecretRequest) (*pluginv1.RevealSecretResponse, error) {
	value, err := s.impl.RevealSecret(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	return &pluginv1.RevealSecretResponse{Value: value}, nil
}

func (s *grpcHostServer) EmitEvent(ctx context.Context, req *pluginv1.EmitEventRequest) (*pluginv1.EmitEventResponse, error) {
	payload, err := structToMap(req.GetPayload())
	if err != nil {
		return nil, err
	}
	if err := s.impl.EmitEvent(ctx, req.GetEventName(), payload); err != nil {
		return nil, err
	}
	return &pluginv1.EmitEventResponse{}, nil
}

var _ pluginv1.HostServer = (*grpcHostServer)(nil)
