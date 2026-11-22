package plugins

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/stretchr/testify/require"
)

func TestSidebarGroupingUninstallPublishesRemovedIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, fsStore, _ := newTestServiceWithDir(t)
		record := installTestPlugin(t, svc, "kandev-plugin-slack")
		record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "snapshot"}}
		record.Actions = []manifest.Action{{Key: "snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
		require.NoError(t, fsStore.Save(record))
		svc.registry.Add(record)
		require.Len(t, svc.SidebarGroupingCatalog(), 1)
		events := bus.NewMemoryEventBus(testLogger(t))
		t.Cleanup(events.Close)
		svc.eventBus = events
		received := make(chan any, 4)
		sub, err := events.Subscribe("plugins.sidebar-grouping.updated", func(_ context.Context, e *bus.Event) error { received <- e.Data; return nil })
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
		require.NoError(t, svc.Uninstall(t.Context(), record.ID))
		synctest.Wait()
		require.Empty(t, svc.SidebarGroupingCatalog())
		scoped := false
		for len(received) > 0 {
			if payload, ok := (<-received).(map[string]any); ok && payload["plugin_id"] == record.ID {
				scoped = true
			}
		}
		require.True(t, scoped, "removal must fence old replies before catalog discovery recovers")
	})
}
