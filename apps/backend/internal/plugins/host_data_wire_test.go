// host_data_wire_test.go proves the Host data API (ADR 0043) end to end over
// the REAL go-plugin gRPC transport: a plugin-side pluginsdk.Host call
// travels through proto -> a real grpc.Server/Client pair with a real
// GRPCBroker (hcplugin.TestPluginGRPCConn — no subprocess, but the same
// transport code production uses) -> this package's real *pluginHost, which
// enforces the api_read:<resource> capability gate and calls the injected
// (fake) data sources.
//
// This complements two narrower test layers that already exist:
//   - pkg/pluginsdk/host_data_test.go exercises grpcHostServer/grpcHostClient
//     over a bufconn with a fake pluginsdk.Host — proves the SDK's wire
//     plumbing, not kandev's real capability gate or DTO mapping.
//   - internal/plugins/host_data_test.go calls *pluginHost's methods
//     directly (no transport) — proves the gate and DTO mapping in-process.
//
// Only this file drives a real *pluginHost from the plugin side of a real
// broker connection, so it is the one place proving a plugin cannot read
// undeclared resources no matter how it talks to kandev.
//
// Placement: this lives in internal/plugins (not pkg/pluginsdk) because it
// needs to construct a real *pluginHost, which imports pluginsdk — importing
// internal/plugins from a pkg/pluginsdk test would invert that dependency
// direction. internal/plugins already imports pluginsdk and hashicorp/go-plugin
// transitively, so the harness reuse (hcplugin.TestPluginGRPCConn) is a
// same-direction import here.
package plugins

import (
	"context"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// wireAuthorPlugin is a minimal author-facing pluginsdk.Plugin. It only
// exists to receive the Host injected over the broker (via the embedded
// UnimplementedPlugin's HostSetter/Host() accessor — see serve.go's "Host
// injection" doc), so these tests can call host.Sessions()... exactly as a
// real plugin author would from InvokeTool/OnEvent/HandleWebhook.
type wireAuthorPlugin struct {
	pluginsdk.UnimplementedPlugin
}

// dialPluginHostOverWire serves host (a real, capability-gated *pluginHost)
// as the kandev.plugin.v1 Host service over a real go-plugin gRPC broker —
// hcplugin.TestPluginGRPCConn wires a real unix-socket grpc.Server/Client
// pair with a real GRPCBroker, matching production transport behavior
// (pkg/pluginsdk/serve_test.go uses the identical harness). It returns the
// plugin-side pluginsdk.Host handle, so callers exercise the full
// SDK-call -> proto -> grpc -> broker -> grpcHostServer -> *pluginHost path.
func dialPluginHostOverWire(t *testing.T, host *pluginHost) pluginsdk.Host {
	t.Helper()
	author := &wireAuthorPlugin{}
	gp := &pluginsdk.GRPCPlugin{Impl: author, Host: host, HostDialTimeout: 5 * time.Second}

	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		pluginsdk.PluginMapKey: gp,
	})
	// Registered in the same order as pkg/pluginsdk/serve_test.go's
	// deferred cleanup (server.Stop before client.Close): t.Cleanup runs
	// LIFO, so registering Close first means Stop runs first at teardown.
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(server.Stop)

	raw, err := client.Dispense(pluginsdk.PluginMapKey)
	require.NoError(t, err)
	_, ok := raw.(*pluginsdk.RemotePlugin)
	require.True(t, ok, "Dispense(%q) should return *pluginsdk.RemotePlugin, got %T", pluginsdk.PluginMapKey, raw)

	require.Eventually(t, func() bool {
		return author.Host() != nil
	}, 5*time.Second, 10*time.Millisecond, "plugin-side Host should be injected via the broker")
	return author.Host()
}

// seedSessionsWireFixture wires d's fake task data source with a single
// workspace/task and n canned sessions (newest first: session-0 started
// most recently), plus n matching SessionCodeStats rows in the analytics
// fake, and a distinguishing acp_session_id on the first session so the DTO
// round-trip assertion has something to check besides IDs.
func seedSessionsWireFixture(d *testDataHost, n int) {
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}

	base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	sessions := make([]*taskmodels.TaskSession, n)
	stats := make([]*analyticsmodels.SessionCodeStats, n)
	for i := 0; i < n; i++ {
		id := "session-" + string(rune('0'+i))
		sessions[i] = &taskmodels.TaskSession{
			ID:        id,
			TaskID:    "task-1",
			State:     taskmodels.TaskSessionStateRunning,
			StartedAt: base.Add(time.Duration(n-i) * time.Hour), // session-0 newest
		}
		if i == 0 {
			sessions[i].Metadata = map[string]any{"acp": map[string]any{"session_id": "acp-session-0"}}
		}
		stats[i] = &analyticsmodels.SessionCodeStats{
			SessionID:           id,
			LinesAddedCommitted: int64(10 * (i + 1)),
		}
	}
	d.tasks.sessionsByTask = map[string][]*taskmodels.TaskSession{"task-1": sessions}
	d.codeStats.stats = stats
}

// TestPluginHostData_Wire_SessionsRoundTrip proves canned Sessions +
// SessionCodeStats data round-trips correctly, over the real broker
// transport, through a *pluginHost whose manifest declares api_read:sessions
// — the concrete "plugin reads kandev data over gRPC" scenario ADR 0043
// exists for.
func TestPluginHostData_Wire_SessionsRoundTrip(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	seedSessionsWireFixture(d, 1)

	host := dialPluginHostOverWire(t, d.host)

	sessions, pageInfo, err := host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "session-0", sessions[0].ID)
	require.Equal(t, "task-1", sessions[0].TaskID)
	require.Equal(t, "acp-session-0", sessions[0].ACPSessionID, "acp_session_id should round-trip from TaskSession.Metadata")
	require.Equal(t, "RUNNING", sessions[0].State)
	require.NotNil(t, pageInfo)
	require.False(t, pageInfo.HasMore)

	stats, _, err := host.Sessions().CodeStats(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "session-0", stats[0].SessionID)
	require.Equal(t, int64(10), stats[0].LinesAddedCommitted)
}

// TestPluginHostData_Wire_PermissionDeniedPerResource is the security
// assertion: a *pluginHost whose manifest does NOT declare api_read:sessions
// returns gRPC PermissionDenied with the exact "capability
// 'api_read:sessions' not declared" wire message when a plugin calls
// Sessions().CodeStats over the real transport — proving the gate is
// enforced host-side, not something a malicious or buggy plugin could route
// around by talking to the wire protocol directly. Run twice, with two
// different capability sets, to prove the gate is per-resource (not a single
// on/off switch): a manifest with only api_read:tasks still denies sessions,
// while its own declared resource (tasks) succeeds over the same connection.
func TestPluginHostData_Wire_PermissionDeniedPerResource(t *testing.T) {
	t.Run("NoCapabilitiesDeclared", func(t *testing.T) {
		d := newTestDataHost(manifest.Capabilities{})
		seedSessionsWireFixture(d, 1)
		host := dialPluginHostOverWire(t, d.host)

		_, _, err := host.Sessions().CodeStats(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok, "expected a gRPC status error, got %v", err)
		require.Equal(t, codes.PermissionDenied, st.Code())
		require.Equal(t, "capability 'api_read:sessions' not declared", st.Message())
	})

	t.Run("OnlyTasksCapabilityDeclared_TasksWorkSessionsDenied", func(t *testing.T) {
		d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
		seedSessionsWireFixture(d, 1)
		// seedSessionsWireFixture wires a bare task-1 fixture (Sessions()
		// needs one to hang sessions off of); give it a Title so the Tasks()
		// assertion below has something distinguishing to check.
		d.tasks.tasksByWorkspace["ws-1"][0].Title = "Fix the bug"
		host := dialPluginHostOverWire(t, d.host)

		// The declared resource (tasks) succeeds over the wire.
		tasks, _, err := host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{})
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		require.Equal(t, "Fix the bug", tasks[0].Title)

		// The undeclared resource (sessions) is denied, per-resource, on the
		// exact same connection/host.
		_, _, err = host.Sessions().CodeStats(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok, "expected a gRPC status error, got %v", err)
		require.Equal(t, codes.PermissionDenied, st.Code())
		require.Equal(t, "capability 'api_read:sessions' not declared", st.Message())

		_, _, err = host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
		require.Error(t, err)
		st, ok = status.FromError(err)
		require.True(t, ok, "expected a gRPC status error, got %v", err)
		require.Equal(t, codes.PermissionDenied, st.Code())
		require.Equal(t, "capability 'api_read:sessions' not declared", st.Message())
	})
}

// TestPluginHostData_Wire_SessionsPagination seeds three sessions and drives
// two Sessions().List calls over the real broker connection: Page{Limit: 2}
// must return exactly 2 items plus a non-empty PageInfo.NextCursor, and a
// follow-up call passing that cursor back must return the remaining item
// with HasMore=false — proving the opaque-cursor pagination contract (ADR
// 0042 "Conventions") survives the real proto round trip, not just the
// in-process paginate() helper.
func TestPluginHostData_Wire_SessionsPagination(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	seedSessionsWireFixture(d, 3)
	host := dialPluginHostOverWire(t, d.host)

	firstPage, pageInfo, err := host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{Limit: 2})
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.Equal(t, "session-0", firstPage[0].ID)
	require.Equal(t, "session-1", firstPage[1].ID)
	require.NotNil(t, pageInfo)
	require.True(t, pageInfo.HasMore)
	require.NotEmpty(t, pageInfo.NextCursor)

	secondPage, secondPageInfo, err := host.Sessions().List(
		context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{Limit: 2, Cursor: pageInfo.NextCursor},
	)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Equal(t, "session-2", secondPage[0].ID)
	require.NotNil(t, secondPageInfo)
	require.False(t, secondPageInfo.HasMore)
	require.Empty(t, secondPageInfo.NextCursor)
}
