package plugins

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
func TestSidebarPluginGroupingStateInvalidatesWithoutPublishingValues(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, _, _ := newTestService(t)
		svc.SetState(newHostTestStateStore(t))
		events := bus.NewMemoryEventBus(testLogger(t))
		t.Cleanup(events.Close)
		svc.eventBus = events
		svc.registry.Add(&store.Record{Manifest: manifest.Manifest{ID: "acme", Capabilities: manifest.Capabilities{State: true}, SidebarGroupings: []manifest.SidebarGrouping{{ID: "labels", Action: "labels.snapshot"}}}, Status: store.StatusActive})
		received := make(chan any, 4)
		sub, err := events.Subscribe("plugins.sidebar-grouping.updated", func(_ context.Context, e *bus.Event) error { received <- e.Data; return nil })
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sub.Unsubscribe() })
		host := svc.hostForPlugin("acme")
		if err := host.SetState(t.Context(), "workspace", "ws", "private", map[string]any{"secret": "must-not-leak"}); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case payload := <-received:
			if !reflect.DeepEqual(payload, map[string]any{"plugin_id": "acme", "workspace_id": "ws"}) {
				t.Fatalf("unexpected invalidation payload: %#v", payload)
			}
		default:
			t.Fatal("successful provider state write emitted no invalidation")
		}
		if err := host.SetState(t.Context(), "workspace", "ws", "private", map[string]any{"invalid": make(chan int)}); err == nil {
			t.Fatal("invalid state write succeeded")
		}
		synctest.Wait()
		select {
		case <-received:
			t.Fatal("failed state write emitted invalidation")
		default:
		}
		if err := host.DeleteState(t.Context(), "workspace", "ws", "private"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-received:
		default:
			t.Fatal("provider state deletion emitted no invalidation")
		}
	})
}
