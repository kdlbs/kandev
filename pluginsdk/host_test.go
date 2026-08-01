package pluginsdk

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// recordingHost is a fake Go-native Host implementation used to drive the
// grpcHostServer adapter without a real state store. It embeds
// UnimplementedHostData so it satisfies the extended Host interface without
// wiring the Host data API (ADR 0043) accessors — those are covered by
// dataRecordingHost in host_data_test.go.
type recordingHost struct {
	UnimplementedHostData
	getStateFn func(ctx context.Context, scope, scopeID, key string) (map[string]any, bool, error)
	setState   struct {
		scope, scopeID, key string
		value               map[string]any
	}
	listStateEntries []StateEntry
	configValues     map[string]any
	ownedSecrets     map[string]string
	revealSecretFn   func(ctx context.Context, ref string) (string, error)
	emitEvent        struct {
		name    string
		payload map[string]any
	}
	deleteStateCalled bool
}

func (h *recordingHost) GetState(ctx context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
	return h.getStateFn(ctx, scope, scopeID, key)
}

func (h *recordingHost) SetState(_ context.Context, scope, scopeID, key string, value map[string]any) error {
	h.setState.scope, h.setState.scopeID, h.setState.key, h.setState.value = scope, scopeID, key, value
	return nil
}

func (h *recordingHost) DeleteState(context.Context, string, string, string) error {
	h.deleteStateCalled = true
	return nil
}

func (h *recordingHost) ListState(context.Context, string, string) ([]StateEntry, error) {
	return h.listStateEntries, nil
}

func (h *recordingHost) GetConfig(context.Context) (map[string]any, error) {
	return h.configValues, nil
}

func (h *recordingHost) RevealSecret(ctx context.Context, ref string) (string, error) {
	return h.revealSecretFn(ctx, ref)
}

func (h *recordingHost) GetSecret(_ context.Context, key string) (string, bool, error) {
	value, ok := h.ownedSecrets[key]
	return value, ok, nil
}

func (h *recordingHost) SetSecret(_ context.Context, key, value string) error {
	if h.ownedSecrets == nil {
		h.ownedSecrets = map[string]string{}
	}
	h.ownedSecrets[key] = value
	return nil
}

func (h *recordingHost) DeleteSecret(_ context.Context, key string) error {
	delete(h.ownedSecrets, key)
	return nil
}

func (h *recordingHost) EmitEvent(_ context.Context, name string, payload map[string]any) error {
	h.emitEvent.name, h.emitEvent.payload = name, payload
	return nil
}

// dialHostOverBufconn wires a grpcHostServer (wrapping impl) to a
// grpcHostClient over an in-memory bufconn listener, bypassing go-plugin's
// broker entirely. The broker handshake itself is covered by the real
// end-to-end test in serve_test.go.
func dialHostOverBufconn(t *testing.T, impl Host) Host {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	registerHostServer(srv, impl)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return newHostClient(conn)
}

func TestHost_GetState_FoundAndNotFound(t *testing.T) {
	impl := &recordingHost{
		getStateFn: func(_ context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
			if scope == "task" && scopeID == "t1" && key == "k1" {
				return map[string]any{"v": float64(1)}, true, nil
			}
			return nil, false, nil
		},
	}
	host := dialHostOverBufconn(t, impl)

	value, found, err := host.GetState(context.Background(), "task", "t1", "k1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, map[string]any{"v": float64(1)}, value)

	_, found, err = host.GetState(context.Background(), "task", "t1", "missing")
	require.NoError(t, err)
	require.False(t, found)
}

func TestHost_SetState(t *testing.T) {
	impl := &recordingHost{}
	host := dialHostOverBufconn(t, impl)

	err := host.SetState(context.Background(), "workspace", "ws1", "cfg", map[string]any{"enabled": true})
	require.NoError(t, err)
	require.Equal(t, "workspace", impl.setState.scope)
	require.Equal(t, "ws1", impl.setState.scopeID)
	require.Equal(t, "cfg", impl.setState.key)
	require.Equal(t, map[string]any{"enabled": true}, impl.setState.value)
}

func TestHost_DeleteState(t *testing.T) {
	impl := &recordingHost{}
	host := dialHostOverBufconn(t, impl)

	require.NoError(t, host.DeleteState(context.Background(), "task", "t1", "k1"))
	require.True(t, impl.deleteStateCalled)
}

func TestHost_ListState(t *testing.T) {
	impl := &recordingHost{listStateEntries: []StateEntry{
		{Key: "a", Value: map[string]any{"x": float64(1)}, UpdatedAt: "2026-07-15T00:00:00Z"},
		{Key: "b", Value: nil, UpdatedAt: "2026-07-15T00:00:01Z"},
	}}
	host := dialHostOverBufconn(t, impl)

	entries, err := host.ListState(context.Background(), "instance", "")
	require.NoError(t, err)
	require.Equal(t, impl.listStateEntries, entries)
}

func TestHost_GetConfig(t *testing.T) {
	impl := &recordingHost{configValues: map[string]any{
		"github_token": "ghp_real",
		"org":          "kdlbs",
		"max_items":    float64(25),
	}}
	host := dialHostOverBufconn(t, impl)

	config, err := host.GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, impl.configValues, config)
}

func TestHost_GetConfig_EmptyIsNonNil(t *testing.T) {
	host := dialHostOverBufconn(t, &recordingHost{})

	config, err := host.GetConfig(context.Background())
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Empty(t, config)
}

func TestHost_RevealSecret(t *testing.T) {
	impl := &recordingHost{revealSecretFn: func(_ context.Context, ref string) (string, error) {
		if ref == "known" {
			return "sh-secret", nil
		}
		return "", errors.New("not found")
	}}
	host := dialHostOverBufconn(t, impl)

	value, err := host.RevealSecret(context.Background(), "known")
	require.NoError(t, err)
	require.Equal(t, "sh-secret", value)

	_, err = host.RevealSecret(context.Background(), "unknown")
	require.Error(t, err)
}

func TestHost_PluginSecrets_RoundTripOverWire(t *testing.T) {
	impl := &recordingHost{}
	host := dialHostOverBufconn(t, impl)
	ctx := context.Background()

	_, found, err := host.GetSecret(ctx, "pat")
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, host.SetSecret(ctx, "pat", "ghp_owned"))
	value, found, err := host.GetSecret(ctx, "pat")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "ghp_owned", value)

	require.NoError(t, host.DeleteSecret(ctx, "pat"))
	_, found, err = host.GetSecret(ctx, "pat")
	require.NoError(t, err)
	require.False(t, found)
}

func TestHost_EmitEvent(t *testing.T) {
	impl := &recordingHost{}
	host := dialHostOverBufconn(t, impl)

	require.NoError(t, host.EmitEvent(context.Background(), "custom.thing", map[string]any{"a": float64(1)}))
	require.Equal(t, "custom.thing", impl.emitEvent.name)
	require.Equal(t, map[string]any{"a": float64(1)}, impl.emitEvent.payload)
}
