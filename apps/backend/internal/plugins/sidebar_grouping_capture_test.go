package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestSidebarResolverDoesNotObserveRolledBackStatus(t *testing.T) {
	for _, phase := range []string{"before RPC", "after RPC"} {
		t.Run(phase, func(t *testing.T) {
			svc, _, fsStore, _ := newTestServiceWithDir(t)
			record := installTestPlugin(t, svc, "kandev-plugin-slack")
			record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "snapshot"}}
			record.Actions = []manifest.Action{{Key: "snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
			require.NoError(t, fsStore.Save(record))
			svc.registry.Add(record)
			svc.taskData = &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "ws"}}}
			barrier := &sidebarStatusCommitBarrier{Store: fsStore, started: make(chan struct{}), release: make(chan struct{}), fail: true}
			svc.store = barrier
			statusDone := make(chan error, 1)
			startStatus := func() { go func() { statusDone <- svc.setStatus(record.ID, StatusError) }(); <-barrier.started }
			if phase == "before RPC" {
				startStatus()
			}
			ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "reader", Role: authn.RoleMember})
			resolved := make(chan error, 1)
			invoke := func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
				if phase == "after RPC" {
					startStatus()
				}
				return &pluginsdk.PluginActionResponse{Body: []byte(`{"version":1,"complete":true,"revision":"r","groups":[],"memberships":[]}`)}, nil
			}
			go func() {
				_, err := svc.resolveSidebarGrouping(ctx, "ws", "plugin:"+record.ID+":labels", invoke)
				resolved <- err
			}()
			if phase == "after RPC" {
				<-barrier.started
			}
			var early error
			completed := false
			select {
			case early = <-resolved:
				completed = true
			case <-time.After(100 * time.Millisecond):
			}
			close(barrier.release)
			require.Error(t, <-statusDone)
			if completed {
				require.NoError(t, early, "provisional status rejected healthy classification")
			} else {
				require.NoError(t, <-resolved)
			}
		})
	}
}

func TestSidebarCatalogProjectionUsesOneCommittedCapture(t *testing.T) {
	svc, _, fsStore, _ := newTestServiceWithDir(t)
	record := installTestPlugin(t, svc, "kandev-plugin-slack")
	record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "snapshot"}}
	record.Actions = []manifest.Action{{Key: "snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
	require.NoError(t, fsStore.Save(record))
	svc.registry.Add(record)
	before := svc.SidebarGroupingCatalog()
	captured := svc.captureSidebarGroupingCatalog()
	require.NoError(t, svc.Disable(record.ID))
	require.Empty(t, svc.SidebarGroupingCatalog())
	require.Equal(t, before, projectSidebarGroupingCatalog(captured), "projection must not pair an old active record with a successor epoch")
	require.NoError(t, svc.Enable(record.ID))
	after := svc.SidebarGroupingCatalog()
	require.NotEqual(t, before[0].Generation, after[0].Generation)
	require.Equal(t, before, projectSidebarGroupingCatalog(captured))
}
