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

func TestPluginToolCatalogRefresherProjectsAutomationSurface(t *testing.T) {
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
		if len(got.Tools) != 1 {
			t.Fatalf("pushed tools = %#v", got.Tools)
		}
		if got.Generation != want.Generation || got.Revision != want.Revision {
			t.Fatalf("projection changed catalog identity: %#v", got)
		}
		if surfaces := got.Tools[0].Surfaces; len(surfaces) != 1 || surfaces[0] != plugintools.SurfaceKanban {
			t.Fatalf("pushed surfaces = %#v", surfaces)
		}
	case <-time.After(time.Second):
		t.Fatal("catalog notification was not pushed")
	}
}
