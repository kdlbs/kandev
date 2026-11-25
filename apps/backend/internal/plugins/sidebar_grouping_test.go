package plugins

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.4
func TestSidebarPluginGroupingResolver(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.taskData = &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "ws"}}}
	svc.registry.Add(&store.Record{Manifest: manifest.Manifest{ID: "acme", Version: "1.0.0", SidebarGroupings: []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}, Actions: []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace, MaxBodyBytes: 16}}}, Status: store.StatusActive, InstallationID: "install", InstalledAt: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)})
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "reader", Role: authn.RoleMember})
	calls := 0
	invoke := func(ctx context.Context, id string, generation pluginDispatchGeneration, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
		calls++
		if id != "acme" || req.ActionKey != "labels.snapshot" || req.Context.ActorID != "reader" || req.Context.WorkspaceID != "ws" || string(req.Body) != "{}" {
			t.Fatalf("unverified dispatch: %+v", req)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("grouping action has no five-second bound")
		}
		return &pluginsdk.PluginActionResponse{Body: []byte(`{"version":1,"revision":"r1","complete":true,"groups":[],"memberships":[]}`)}, nil
	}
	snapshot, err := svc.resolveSidebarGrouping(ctx, "ws", "plugin:acme:labels", invoke)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Reference != "plugin:acme:labels" || snapshot.Snapshot.Revision != "r1" || snapshot.Generation == "" {
		t.Fatalf("wrong snapshot identity: %+v", snapshot)
	}
	catalog := svc.SidebarGroupingCatalog()
	if len(catalog) != 1 || catalog[0].Reference != "plugin:acme:labels" || catalog[0].Label != "Labels" || catalog[0].Generation != snapshot.Generation {
		t.Fatalf("active provider missing from catalog: %+v", catalog)
	}
	for _, tc := range []struct {
		name, workspace, ref string
		ctx                  context.Context
	}{
		{"foreign workspace", "foreign", "plugin:acme:labels", ctx}, {"missing identity", "ws", "plugin:acme:labels", t.Context()}, {"undeclared grouping", "ws", "plugin:acme:other", ctx},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.resolveSidebarGrouping(tc.ctx, tc.workspace, tc.ref, invoke); err == nil {
				t.Fatal("unavailable/unauthorized provider accepted")
			}
		})
	}
	if calls != 1 {
		t.Fatalf("rejected lookup reached plugin: %d calls", calls)
	}
	svc.registry.SetStatus("acme", StatusDisabled)
	if _, err := svc.resolveSidebarGrouping(ctx, "ws", "plugin:acme:labels", invoke); err == nil {
		t.Fatal("disabled provider accepted")
	}
	if calls != 1 {
		t.Fatal("disabled provider invoked")
	}
	if len(svc.SidebarGroupingCatalog()) != 0 {
		t.Fatal("disabled provider still advertised")
	}
}

func TestSidebarPluginGroupingRejectsOldReplyAfterDisableEnable(t *testing.T) {
	svc, _, _ := newTestService(t)
	record := installTestPlugin(t, svc, "kandev-plugin-slack")
	record.SidebarGroupings = []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
	record.Actions = []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace, MaxBodyBytes: 16}}
	svc.registry.Add(record)
	svc.taskData = &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "ws"}}}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "reader", Role: authn.RoleMember})
	invoke := func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
		// The RPC lease has ended before the resolver publishes its reply.
		if err := svc.Disable(record.ID); err != nil {
			t.Fatal(err)
		}
		if err := svc.Enable(record.ID); err != nil {
			t.Fatal(err)
		}
		return &pluginsdk.PluginActionResponse{Body: []byte(`{"version":1,"revision":"old","complete":true,"groups":[],"memberships":[]}`)}, nil
	}
	if _, err := svc.resolveSidebarGrouping(ctx, "ws", "plugin:"+record.ID+":labels", invoke); err == nil {
		t.Fatal("an old process reply crossed a disable/enable generation with unchanged package metadata")
	}
}

func TestSidebarGroupingResolverRejectsAmbiguousWire(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.taskData = &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "ws"}}}
	svc.registry.Add(&store.Record{Manifest: manifest.Manifest{ID: "acme", Version: "1.0.0", SidebarGroupings: []manifest.SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}, Actions: []manifest.Action{{Key: "labels.snapshot", ResourceScope: manifest.ActionScopeWorkspace, MaxBodyBytes: 16}}}, Status: store.StatusActive})
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "reader", Role: authn.RoleMember})
	for _, body := range []string{
		`{"version":1,"revision":"r","complete":true,"groups":[{"id":"g","label":"G"}],"memberships":[{"task_id":"t","group_id":"g"}],"memberships":[]}`,
		`{"version":1,"revision":"r","complete":true,"groups":[],"GROUPS":[{"id":"g","label":"G"}],"memberships":[]}`,
	} {
		invoke := func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
			return &pluginsdk.PluginActionResponse{Body: []byte(body)}, nil
		}
		got, err := svc.resolveSidebarGrouping(ctx, "ws", "plugin:acme:labels", invoke)
		if got != nil || !errors.Is(err, taskmodels.ErrSidebarGroupingUnavailable) {
			t.Fatalf("ambiguous wire admitted instead of unavailable: %v", err)
		}
	}
}
