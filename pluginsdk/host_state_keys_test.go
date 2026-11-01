package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stateKeysRecordingHost struct {
	recordingHost
	reader StateKeyReader
}

func (h *stateKeysRecordingHost) StateKeys() StateKeyReader { return h.reader }

type stateKeysRecordingReader struct{ query StateKeyQuery }

func (r *stateKeysRecordingReader) ListKeys(_ context.Context, query StateKeyQuery) (StateKeyPage, error) {
	r.query = query
	return StateKeyPage{Keys: []string{"", "legacy:receipt"}, NextCursor: "opaque-resume"}, nil
}

func TestHostStateKeysRoundTrip(t *testing.T) {
	impl := &stateKeysRecordingReader{}
	host := dialHostOverBufconn(t, &stateKeysRecordingHost{reader: impl})
	reader, ok := HostStateKeys(host)
	require.True(t, ok)
	query := StateKeyQuery{Scope: "legacy", ScopeID: "scope", Cursor: "opaque-start", Limit: 17}
	page, err := reader.ListKeys(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, query, impl.query)
	require.Equal(t, StateKeyPage{Keys: []string{"", "legacy:receipt"}, NextCursor: "opaque-resume"}, page)
	for _, limit := range []int{-1, 201, int(^uint(0) >> 1)} {
		_, err = reader.ListKeys(context.Background(), StateKeyQuery{Limit: limit})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}

func TestHostStateKeysUnsupported(t *testing.T) {
	for _, host := range []Host{&recordingHost{}, &stateKeysRecordingHost{}} {
		_, ok := HostStateKeys(host)
		require.False(t, ok)
		reader, ok := HostStateKeys(dialHostOverBufconn(t, host))
		require.True(t, ok)
		_, err := reader.ListKeys(context.Background(), StateKeyQuery{Scope: "legacy"})
		require.Equal(t, codes.Unimplemented, status.Code(err))
	}
}
