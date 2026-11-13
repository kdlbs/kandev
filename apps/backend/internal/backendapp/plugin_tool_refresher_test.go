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

func TestPluginToolCatalogRefresherForwardsFullCatalog(t *testing.T) {
	want := plugintools.Snapshot{
		Generation: "g", Revision: 4,
		Tools: []plugintools.Definition{
			{PluginID: "mixed", LocalName: "mixed", ExposedName: plugintools.ExposedName("mixed", "mixed"), Description: "mixed", Surfaces: []string{plugintools.SurfaceKanban, plugintools.SurfaceAutomation}},
			{PluginID: "automation", LocalName: "automation", ExposedName: plugintools.ExposedName("automation", "automation"), Description: "automation", Surfaces: []string{plugintools.SurfaceAutomation}},
		},
	}
	lifecycle := pluginToolLifecycleFake{calls: make(chan plugintools.Snapshot, 1)}
	refresher := newPluginToolCatalogRefresher(context.Background(), pluginToolSourceFake{snapshot: want}, lifecycle, nil)
	t.Cleanup(refresher.Stop)
	refresher.NotifyAgentToolCatalogChanged()
	select {
	case got := <-lifecycle.calls:
		if len(got.Tools) != len(want.Tools) {
			t.Fatalf("pushed tools = %#v, want the full catalog for per-execution projection", got.Tools)
		}
		if got.Generation != want.Generation || got.Revision != want.Revision {
			t.Fatalf("catalog identity changed: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("catalog notification was not pushed")
	}
}

type pluginToolSubscriberFake struct {
	refreshes int
}

func (f *pluginToolSubscriberFake) RefreshPluginTools(context.Context) error {
	f.refreshes++
	return nil
}

func TestPluginToolCatalogFanoutRefreshesSubscribersWithExecutions(t *testing.T) {
	lifecycle := pluginToolLifecycleFake{calls: make(chan plugintools.Snapshot, 1)}
	fanout := newPluginToolCatalogFanout(lifecycle, nil)
	subscriber := &pluginToolSubscriberFake{}
	unsubscribe := fanout.Subscribe(subscriber)

	snapshot := plugintools.Snapshot{Generation: "g", Revision: 7}
	if err := fanout.SetPluginToolsForAllExecutions(context.Background(), snapshot); err != nil {
		t.Fatalf("fanout: %v", err)
	}
	if got := <-lifecycle.calls; got.Revision != snapshot.Revision {
		t.Fatalf("execution revision = %d, want %d", got.Revision, snapshot.Revision)
	}
	if subscriber.refreshes != 1 {
		t.Fatalf("subscriber refreshes = %d, want 1", subscriber.refreshes)
	}

	unsubscribe()
	unsubscribe()
	if err := fanout.SetPluginToolsForAllExecutions(context.Background(), snapshot); err != nil {
		t.Fatalf("fanout after unsubscribe: %v", err)
	}
	if got := <-lifecycle.calls; got.Revision != snapshot.Revision {
		t.Fatalf("execution revision after unsubscribe = %d, want %d", got.Revision, snapshot.Revision)
	}
	if subscriber.refreshes != 1 {
		t.Fatalf("unsubscribed server refreshed %d times", subscriber.refreshes)
	}
}
