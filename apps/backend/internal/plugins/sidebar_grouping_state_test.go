package plugins

import (
	"context"
	"errors"
	"fmt"
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
					require.Equal(t, map[string]any{"catalog": true, "plugin_id": record.ID}, payload)
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

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
func TestSidebarGroupingCatalogTracksCommittedStatusOnCleanupFailure(t *testing.T) {
	for _, operation := range []string{"disable", "unhealthy"} {
		for _, failSave := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/save-failure=%t", operation, failSave), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					svc, _, fsStore, _ := newTestServiceWithDir(t)
					record := installTestPlugin(t, svc, "kandev-plugin-slack")
					record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
					record.Actions = []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
					require.NoError(t, fsStore.Save(record))
					svc.registry.Add(record)
					before := svc.SidebarGroupingCatalog()
					require.Len(t, before, 1)
					if failSave {
						failing := &failMatchingSaveStore{Store: fsStore, match: func(r *store.Record) bool { return r.Status != StatusActive }, failErr: errors.New("status commit refused")}
						failing.arm()
						svc.store = failing
					}
					svc.SetAutomationRevoker(func(string) error { return errors.New("cancel refused") })
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
					want := StatusDisabled
					if operation == "disable" {
						require.Error(t, svc.Disable(record.ID))
					} else {
						want = StatusError
						svc.handleStatusChange(record.ID, false, errors.New("health failed"))
					}
					synctest.Wait()
					current, err := svc.Get(record.ID)
					require.NoError(t, err)
					persisted, err := fsStore.Get(record.ID)
					require.NoError(t, err)
					if failSave {
						require.Equal(t, StatusActive, current.Status)
						require.Equal(t, StatusActive, persisted.Status)
						require.Equal(t, before, svc.SidebarGroupingCatalog())
						require.Empty(t, received, "a rolled-back status must not invalidate the catalog")
					} else {
						require.Equal(t, want, current.Status)
						require.Equal(t, want, persisted.Status)
						require.Empty(t, svc.SidebarGroupingCatalog(), "committed inactive provider is unavailable")
						select {
						case <-received:
						default:
							t.Fatal("committed inactive status left connected sidebar consumers on the old catalog")
						}
					}
				})
			})
		}
	}
}

func TestSidebarGroupingCatalogPreservesUnrelatedProvider(t *testing.T) {
	for _, operation := range []string{"config", "disable"} {
		t.Run(operation, func(t *testing.T) {
			svc, _, fsStore, _ := newTestServiceWithDir(t)
			provider := installTestPlugin(t, svc, "kandev-plugin-slack")
			provider.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
			provider.Actions = []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
			require.NoError(t, fsStore.Save(provider))
			svc.registry.Add(provider)
			unrelated := installTestPlugin(t, svc, "kandev-plugin-other")
			before := svc.SidebarGroupingCatalog()
			if operation == "config" {
				require.NoError(t, svc.UpdateConfig(t.Context(), unrelated.ID, map[string]any{"label": "Changed"}))
			} else {
				require.NoError(t, svc.Disable(unrelated.ID))
			}
			require.Equal(t, before, svc.SidebarGroupingCatalog(), "unrelated lifecycle must not change provider generations")
		})
	}
}
