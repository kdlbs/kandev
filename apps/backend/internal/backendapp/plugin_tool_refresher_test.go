package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/mcp/plugintools"
)

type pluginToolSourceFake struct{ snapshot plugintools.Snapshot }

func (f pluginToolSourceFake) AgentToolCatalog() (plugintools.Snapshot, error) {
	return f.snapshot, nil
}

type pluginToolLifecycleFake struct{ calls chan plugintools.Snapshot }

func (f pluginToolLifecycleFake) SetPluginToolsForAllExecutions(_ context.Context, snapshot plugintools.Snapshot) error {
	f.calls <- snapshot
	return nil
}

func TestPluginToolCatalogRefresherPushesLifecycleNotification(t *testing.T) {
	want := plugintools.Snapshot{Generation: "g", Revision: 4}
	lifecycle := pluginToolLifecycleFake{calls: make(chan plugintools.Snapshot, 1)}
	refresher := newPluginToolCatalogRefresher(context.Background(), pluginToolSourceFake{snapshot: want}, lifecycle, nil)
	t.Cleanup(refresher.Stop)
	refresher.NotifyAgentToolCatalogChanged()
	select {
	case got := <-lifecycle.calls:
		if got.Generation != want.Generation || got.Revision != want.Revision {
			t.Fatalf("snapshot = %#v, want %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("catalog notification was not pushed")
	}
}
