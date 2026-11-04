package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/common/mcpmode"
	p "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestBuiltinLiveUpdatesKeepManifestAndRefreshDispatch(t *testing.T) {
	status := http.StatusOK
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; w.WriteHeader(status) }))
	t.Cleanup(server.Close)
	mgr, execution := workspaceSourceTestManager(t, server.URL, nil)
	execution.TaskID, execution.WorkspaceID = "task-a", "workspace"
	profile := builtinDispatchProfile(p.PackageCore)
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	require.False(t, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPSetTaskTitle))
	require.NoError(t, mgr.SetMcpMode(context.Background(), execution.ID, mcpmode.TaskTitlePending))
	require.True(t, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPSetTaskTitle))
	require.NoError(t, mgr.SetMcpProvidersForSession(context.Background(), execution.SessionID, []string{"github"}))
	stored, _ := execution.metadataValue(p.BuiltinBindingMetadataKey)
	updated, err := p.ParseBuiltinBindingMetadata(stored)
	require.NoError(t, err)
	require.Equal(t, []string{"github"}, updated.Providers)
	require.Equal(t, profile.BuiltinPackages, updated.BuiltinPackages)
	require.NoError(t, mgr.SetMcpMode(context.Background(), execution.ID, ""))
	require.False(t, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPSetTaskTitle))
	require.Error(t, mgr.SetMcpMode(context.Background(), execution.ID, mcpmode.Config))
	require.Equal(t, 3, requests)
	status = http.StatusInternalServerError
	require.Error(t, mgr.SetMcpMode(context.Background(), execution.ID, mcpmode.TaskTitlePending))
	require.False(t, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPSetTaskTitle))
}

type builtinBindingWriter struct {
	captureExecutorRunningWriter
	writes int
	failAt int
}

func (w *builtinBindingWriter) UpdateBuiltinMCPBinding(_ context.Context, _, _ string, _ any) error {
	w.writes++
	if w.writes == w.failAt {
		return errors.New("write rejected")
	}
	return nil
}
func TestBuiltinUpdatePersistenceFencesRemoteChange(t *testing.T) {
	for _, failAt := range []int{0, 1, 2} {
		calls := 0
		helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(http.StatusOK) }))
		t.Cleanup(helper.Close)
		mgr, execution := workspaceSourceTestManager(t, helper.URL, nil)
		execution.TaskID, execution.WorkspaceID = "task-a", "workspace"
		encoded, err := p.MarshalBuiltinBinding(builtinDispatchProfile(p.PackageCore))
		require.NoError(t, err)
		execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
		writer := &builtinBindingWriter{failAt: failAt}
		mgr.SetExecutorRunningWriter(writer)
		err = mgr.SetMcpMode(context.Background(), execution.ID, mcpmode.TaskTitlePending)
		if failAt == 0 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		require.Nil(t, writer.running, "binding changes never upsert the runtime row")
		if failAt == 1 {
			require.Zero(t, calls)
		} else {
			require.Equal(t, 1, calls)
		}
		require.Equal(t, failAt == 0, execution.builtinMCPAuthority(builtinTestActions).Allows(ws.ActionMCPSetTaskTitle))
	}
}
