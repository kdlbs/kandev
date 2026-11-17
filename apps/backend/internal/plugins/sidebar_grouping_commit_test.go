package plugins

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type sidebarStatusCommitBarrier struct {
	store.Store
	started chan struct{}
	release chan struct{}
	fail    bool
}

func (s *sidebarStatusCommitBarrier) Save(record *store.Record) error {
	close(s.started)
	<-s.release
	if s.fail {
		return errors.New("status commit refused")
	}
	return s.Store.Save(record)
}

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.5
func TestSidebarCatalogWaitsForStatusCommit(t *testing.T) {
	for _, target := range []Status{StatusDisabled, StatusError, StatusActive} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%t", target, fail), func(t *testing.T) {
				svc, fsStore, _ := newTestService(t)
				record := installTestPlugin(t, svc, "kandev-plugin-slack")
				record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
				record.Actions = []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
				if target == StatusActive {
					record.Status = StatusDisabled
				}
				require.NoError(t, fsStore.Save(record))
				svc.registry.Add(record)
				before := svc.SidebarGroupingCatalog()
				barrier := &sidebarStatusCommitBarrier{Store: fsStore, started: make(chan struct{}), release: make(chan struct{}), fail: fail}
				svc.store = barrier
				released := false
				t.Cleanup(func() {
					if !released {
						close(barrier.release)
					}
				})
				changed := make(chan error, 1)
				go func() { changed <- svc.SetStatus(record.ID, target) }()
				<-barrier.started
				reading := make(chan struct{})
				result := make(chan []pluginsdk.SidebarGroupingProvider, 1)
				go func() { close(reading); result <- svc.SidebarGroupingCatalog() }()
				<-reading
				// The bounded pending-commit window must not yield authoritative membership.
				var early []pluginsdk.SidebarGroupingProvider
				returned := false
				select {
				case early = <-result:
					returned = true
				case <-time.After(time.Second):
				}
				close(barrier.release)
				released = true
				err := <-changed
				if fail {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				if !returned {
					early = <-result
				}
				expected := before
				if !fail {
					if target == StatusActive {
						require.Len(t, early, 1)
					} else {
						require.Empty(t, early)
					}
				} else {
					require.Equal(t, expected, early, "catalog exposed status that failed to commit")
				}
				require.False(t, returned, "catalog completed while status persistence was pending")
			})
		}
	}
}

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
func TestSidebarPolicyPublicationWaitsForCommit(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%t", fail), func(t *testing.T) {
			svc, fsStore, _ := newTestService(t)
			record := installTestPlugin(t, svc, "kandev-plugin-slack")
			record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
			record.Actions = []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
			require.NoError(t, fsStore.Save(record))
			svc.registry.Add(record)
			svc.taskData = &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "ws"}}}
			before := svc.SidebarGroupingCatalog()
			barrier := &sidebarStatusCommitBarrier{Store: fsStore, started: make(chan struct{}), release: make(chan struct{}), fail: fail}
			svc.store = barrier
			released := false
			t.Cleanup(func() {
				if !released {
					close(barrier.release)
				}
			})
			changed := make(chan error, 1)
			go func() { changed <- svc.UpdateConfig(t.Context(), record.ID, map[string]any{"label": "updated"}) }()
			<-barrier.started
			pending := svc.SidebarGroupingCatalog()
			entered := make(chan struct{})
			resolved := make(chan error, 1)
			ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "reader", Role: authn.RoleMember})
			go func() {
				_, err := svc.resolveSidebarGrouping(ctx, "ws", before[0].Reference,
					func(_ context.Context, _ string, _ pluginDispatchGeneration, _ *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
						close(entered)
						lease := svc.dispatchLocks.lockFor(record.ID)
						lease.RLock()
						defer lease.RUnlock()
						return &pluginsdk.PluginActionResponse{Body: []byte(`{"version":1,"revision":"stable","complete":true,"groups":[],"memberships":[]}`)}, nil
					})
				resolved <- err
			}()
			<-entered
			close(barrier.release)
			released = true
			err := <-changed
			resolveErr := <-resolved
			require.Equal(t, before, pending, "pending policy write exposed a provisional catalog generation")
			if fail {
				require.Error(t, err)
				require.NoError(t, resolveErr, "failed policy write rejected an unchanged healthy provider")
				require.Equal(t, before, svc.SidebarGroupingCatalog())
			} else {
				require.NoError(t, err)
				require.Error(t, resolveErr, "old generation reply crossed a committed config change")
				require.NotEqual(t, before[0].Generation, svc.SidebarGroupingCatalog()[0].Generation)
			}
		})
	}
}
