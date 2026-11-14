package plugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kdlbs/kandev/pluginsdk"
)

func stateKeysWireHost(t *testing.T, store *state.Store, pluginID string, allowed bool) pluginsdk.Host {
	t.Helper()
	return dialPluginHostOverWire(t, &pluginHost{
		pluginID: pluginID, state: store, capabilities: manifest.Capabilities{State: allowed},
	})
}

func stateKeysWireReader(t *testing.T, host pluginsdk.Host) pluginsdk.StateKeyReader {
	t.Helper()
	reader, ok := pluginsdk.HostStateKeys(host)
	require.True(t, ok)
	return reader
}

func TestPluginHostStateKeysBoundedCleanupRestart(t *testing.T) {
	ctx := context.Background()
	store := newHostTestStateStore(t)
	large, err := json.Marshal(map[string]any{"payload": strings.Repeat("x", 3<<20)})
	require.NoError(t, err)
	expected := []string{""}
	require.NoError(t, store.Set(ctx, "owner", "legacy", "s", "", large))
	for i := range 452 {
		key := fmt.Sprintf("receipt-%03d", i)
		expected = append(expected, key)
		value := json.RawMessage(`{"v":true}`)
		if i == 0 {
			value = large
		}
		require.NoError(t, store.Set(ctx, "owner", "legacy", "s", key, value))
	}
	for _, id := range [][3]string{{"foreign", "legacy", "s"}, {"owner", "other", "s"}, {"owner", "legacy", "other"}} {
		require.NoError(t, store.Set(ctx, id[0], id[1], id[2], "sentinel", json.RawMessage(`{}`)))
	}
	var got []string
	for {
		// A fresh broker and Host simulate a plugin restart after each checkpoint.
		host := stateKeysWireHost(t, store, "owner", true)
		checkpoint, _, readErr := host.GetState(ctx, "migration", "", "cursor")
		require.NoError(t, readErr)
		cursor, _ := checkpoint["cursor"].(string)
		page, pageErr := stateKeysWireReader(t, host).ListKeys(ctx, pluginsdk.StateKeyQuery{Scope: "legacy", ScopeID: "s", Cursor: cursor})
		require.NoError(t, pageErr)
		require.LessOrEqual(t, len(page.Keys), 200)
		got = append(got, page.Keys...)
		for _, key := range page.Keys {
			require.NoError(t, host.DeleteState(ctx, "legacy", "s", key))
		}
		require.NoError(t, host.SetState(ctx, "migration", "", "cursor", map[string]any{"cursor": page.NextCursor}))
		if page.NextCursor == "" {
			break
		}
	}
	require.Equal(t, expected, got)
	for _, id := range [][3]string{{"foreign", "legacy", "s"}, {"owner", "other", "s"}, {"owner", "legacy", "other"}} {
		_, found, getErr := store.Get(ctx, id[0], id[1], id[2], "sentinel")
		require.NoError(t, getErr)
		require.True(t, found)
	}
}

func TestPluginHostStateKeysCursorAndCapability(t *testing.T) {
	ctx := context.Background()
	store := newHostTestStateStore(t)
	for _, key := range []string{"", "a", "b"} {
		require.NoError(t, store.Set(ctx, "owner", "legacy", "s", key, json.RawMessage(`{}`)))
	}
	reader := stateKeysWireReader(t, stateKeysWireHost(t, store, "owner", true))
	query := pluginsdk.StateKeyQuery{Scope: "legacy", ScopeID: "s", Limit: 1}
	first, err := reader.ListKeys(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []string{""}, first.Keys)
	require.NotEmpty(t, first.NextCursor)
	require.NoError(t, store.Delete(ctx, "owner", "legacy", "s", ""))
	query.Cursor = first.NextCursor
	second, err := reader.ListKeys(ctx, query)
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, second.Keys)
	assertStateKeyCursorsRejected(t, reader, query)
	foreign := stateKeysWireReader(t, stateKeysWireHost(t, store, "foreign", true))
	_, err = foreign.ListKeys(ctx, query)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	query.Cursor = ""
	empty, err := foreign.ListKeys(ctx, query)
	require.NoError(t, err)
	require.Empty(t, empty.Keys)
	require.Empty(t, empty.NextCursor)
	denied := stateKeysWireReader(t, stateKeysWireHost(t, store, "owner", false))
	_, err = denied.ListKeys(ctx, query)
	assertPermissionDenied(t, err, "state")
}

func assertStateKeyCursorsRejected(t *testing.T, reader pluginsdk.StateKeyReader, query pluginsdk.StateKeyQuery) {
	t.Helper()
	for _, cursor := range []string{"malformed", stateKeyCursorPrefix + "!", stateKeyCursorPrefix + base64.RawURLEncoding.EncodeToString([]byte(`{}`)), query.Cursor + "garbage"} {
		invalid := query
		invalid.Cursor = cursor
		_, err := reader.ListKeys(context.Background(), invalid)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
	for _, edit := range []func(*pluginsdk.StateKeyQuery){
		func(q *pluginsdk.StateKeyQuery) { q.Scope = "other" },
		func(q *pluginsdk.StateKeyQuery) { q.ScopeID = "other" },
		func(q *pluginsdk.StateKeyQuery) { q.Limit = 2 },
		func(q *pluginsdk.StateKeyQuery) { q.Limit = -1 },
		func(q *pluginsdk.StateKeyQuery) { q.Limit = 201 },
	} {
		invalid := query
		edit(&invalid)
		_, err := reader.ListKeys(context.Background(), invalid)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}

func TestPluginHostStateKeysRawLimits(t *testing.T) {
	host := &pluginHost{pluginID: "owner", state: newHostTestStateStore(t), capabilities: manifest.Capabilities{State: true}}
	for _, limit := range []int{-1, 201} {
		_, err := host.StateKeys().ListKeys(context.Background(), pluginsdk.StateKeyQuery{Limit: limit})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}
