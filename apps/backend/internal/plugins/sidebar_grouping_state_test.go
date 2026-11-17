package plugins

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/stretchr/testify/require"
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

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
func TestSidebarGroupingCatalogRefreshesAfterConfigRotation(t *testing.T) {
	for _, outcome := range []string{"restart succeeds", "not running", "config write fails", "restart fails"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc, _, fsStore, rt := newTestServiceWithDir(t)
				record := installTestPlugin(t, svc, "kandev-plugin-slack")
				record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
				record.Actions = []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
				require.NoError(t, fsStore.Save(record))
				svc.registry.Add(record)
				before := svc.SidebarGroupingCatalog()
				require.Len(t, before, 1)
				switch outcome {
				case "not running":
					rt.Stop(record.ID)
				case "config write fails":
					svc.store = &failingStore{Store: fsStore, setConfigErr: errors.New("config commit refused")}
				case "restart fails":
					rt.setStartErr(record.ID, errors.New("restart refused"))
				}
				events := bus.NewMemoryEventBus(testLogger(t))
				t.Cleanup(events.Close)
				svc.eventBus = events
				received := make(chan any, 4)
				sub, err := events.Subscribe("plugins.sidebar-grouping.updated", func(_ context.Context, e *bus.Event) error {
					received <- e.Data
					return nil
				})
				require.NoError(t, err)
				t.Cleanup(func() { _ = sub.Unsubscribe() })
				err = svc.UpdateConfig(t.Context(), record.ID, map[string]any{"label": "Updated"})
				if outcome == "config write fails" || outcome == "restart fails" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				synctest.Wait()
				select {
				case payload := <-received:
					require.Equal(t, map[string]any{"catalog": true}, payload)
				default:
					t.Fatal("admitted config rotation left connected sidebar consumers on the old catalog")
				}
				after := svc.SidebarGroupingCatalog()
				if outcome == "restart fails" {
					require.Empty(t, after, "a failed restart removes the unavailable provider")
				} else {
					require.Len(t, after, 1)
					require.NotEqual(t, before[0].Generation, after[0].Generation)
				}
			})
		})
	}
}
