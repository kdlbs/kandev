package plugins

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type sidebarDispatchCaptureContext struct {
	context.Context
	captured chan struct{}
	once     sync.Once
}

func (c *sidebarDispatchCaptureContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.captured) })
	return c.Context.Done()
}

type sidebarDispatchPlugin struct{ called chan struct{} }

func (*sidebarDispatchPlugin) OnEvent(context.Context, *pluginsdk.Event) error { return nil }
func (*sidebarDispatchPlugin) HandleWebhook(context.Context, *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	return &pluginsdk.WebhookResponse{Status: 200}, nil
}
func (p *sidebarDispatchPlugin) HandleAction(context.Context, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	close(p.called)
	return &pluginsdk.PluginActionResponse{Body: []byte(`{"version":1,"complete":true,"revision":"healthy","groups":[],"memberships":[]}`)}, nil
}

func TestSidebarSupervisedDispatchUsesOnlyCommittedStatus(t *testing.T) {
	for _, fail := range []bool{true, false} {
		t.Run(fmt.Sprintf("failed_save=%t", fail), func(t *testing.T) {
			svc, fsStore, runtime := newTestService(t)
			record := installTestPlugin(t, svc, "kandev-plugin-slack")
			record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "snapshot"}}
			record.Actions = []manifest.Action{{Key: "snapshot", ResourceScope: manifest.ActionScopeWorkspace}}
			require.NoError(t, fsStore.Save(record))
			svc.registry.Add(record)
			svc.taskData = &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "ws"}}}
			impl := &sidebarDispatchPlugin{called: make(chan struct{})}
			client, _ := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
				pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{Impl: impl},
			})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			raw, err := client.Dispense(pluginsdk.PluginMapKey)
			require.NoError(t, err)
			runtime.setRemote(raw.(*pluginsdk.RemotePlugin))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			capture := &sidebarDispatchCaptureContext{
				Context: authn.WithIdentity(ctx, authn.Identity{UserID: "reader", Role: authn.RoleMember}), captured: make(chan struct{}),
			}
			lease := svc.dispatchLocks.lockFor(record.ID)
			lease.Lock()
			locked := true
			defer func() {
				if locked {
					lease.Unlock()
				}
			}()
			type result struct {
				grouping *taskmodels.SidebarPluginGrouping
				err      error
			}
			resolved := make(chan result, 1)
			go func() {
				grouping, err := svc.ResolveSidebarGrouping(capture, "ws", "plugin:"+record.ID+":labels")
				resolved <- result{grouping, err}
			}()
			select {
			case <-capture.captured:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			barrier := &sidebarStatusCommitBarrier{Store: fsStore, started: make(chan struct{}), release: make(chan struct{}), fail: fail}
			svc.store = barrier
			released := false
			defer func() {
				if !released {
					close(barrier.release)
				}
			}()
			statusDone := make(chan error, 1)
			go func() { statusDone <- svc.setStatus(record.ID, StatusError) }()
			select {
			case <-barrier.started:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			lease.Unlock()
			locked = false
			var got result
			completed := false
			select {
			case got = <-resolved:
				completed = true
			case <-impl.called:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			close(barrier.release)
			released = true
			statusErr := <-statusDone
			if !completed {
				got = <-resolved
			}
			if fail {
				require.Error(t, statusErr)
				require.NoError(t, got.err, "failed persistence cannot revoke the admitted healthy action")
				require.Equal(t, "healthy", got.grouping.Snapshot.Revision)
			} else {
				require.NoError(t, statusErr)
				require.ErrorIs(t, got.err, taskmodels.ErrSidebarGroupingUnavailable)
				require.Nil(t, got.grouping)
			}
		})
	}
}
