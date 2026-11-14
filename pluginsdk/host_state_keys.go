package pluginsdk

import (
	"context"

	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MaxStateKeyPageSize is the maximum number of keys returned by ListKeys.
const MaxStateKeyPageSize = 200

// StateKeyQuery selects exactly one scope/scope ID in the connected plugin's
// state. Limit zero defaults to 200. Cursor is opaque and exclusive.
type StateKeyQuery struct {
	Scope   string
	ScopeID string
	Cursor  string
	Limit   int
}

// StateKeyPage contains no state values. An empty NextCursor marks completion.
type StateKeyPage struct {
	Keys       []string
	NextCursor string
}

// StateKeyReader is an optional Host extension for bounded state enumeration.
// Pages use stable key ordering, not a snapshot. Deleting processed keys cannot
// skip subsequent keys. New keys at or before the cursor require a fresh pass.
type StateKeyReader interface {
	ListKeys(context.Context, StateKeyQuery) (StateKeyPage, error)
}

// StateKeysHost supplies the optional state key reader without changing Host.
type StateKeysHost interface {
	StateKeys() StateKeyReader
}

// HostStateKeys returns false for an in-process Host without this extension.
// A gRPC reader can return codes.Unimplemented when the remote host is older.
// Callers must not fall back to the unbounded ListState API.
func HostStateKeys(host Host) (StateKeyReader, bool) {
	provider, ok := host.(StateKeysHost)
	if !ok {
		return nil, false
	}
	reader := provider.StateKeys()
	return reader, reader != nil
}

type grpcStateKeyReader struct{ client pluginv1.HostClient }

func (h *grpcHostClient) StateKeys() StateKeyReader {
	return grpcStateKeyReader{client: h.client}
}

func (r grpcStateKeyReader) ListKeys(ctx context.Context, query StateKeyQuery) (StateKeyPage, error) {
	if query.Limit < 0 || query.Limit > MaxStateKeyPageSize {
		return StateKeyPage{}, status.Error(codes.InvalidArgument, "state key page limit must be between 1 and 200, or zero for the default")
	}
	response, err := r.client.ListStateKeys(ctx, &pluginv1.ListStateKeysRequest{
		Scope: query.Scope, ScopeId: query.ScopeID, Cursor: query.Cursor, Limit: int32(query.Limit),
	})
	if err != nil {
		return StateKeyPage{}, err
	}
	return StateKeyPage{Keys: response.GetKeys(), NextCursor: response.GetNextCursor()}, nil
}

func (s *grpcHostServer) ListStateKeys(ctx context.Context, request *pluginv1.ListStateKeysRequest) (*pluginv1.ListStateKeysResponse, error) {
	reader, ok := HostStateKeys(s.impl)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "state key enumeration is not supported")
	}
	page, err := reader.ListKeys(ctx, StateKeyQuery{
		Scope: request.GetScope(), ScopeID: request.GetScopeId(), Cursor: request.GetCursor(), Limit: int(request.GetLimit()),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListStateKeysResponse{Keys: page.Keys, NextCursor: page.NextCursor}, nil
}

var _ StateKeyReader = grpcStateKeyReader{}
var _ StateKeysHost = (*grpcHostClient)(nil)
