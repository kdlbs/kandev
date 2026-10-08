package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
	"github.com/kandev/kandev/internal/user/store"
)

func TestSidebarLayoutDefaultsAreProjectedPerWorkspace(t *testing.T) {
	svc, _, _ := sidebarService(t)

	got, err := svc.GetUserSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, workspaceID := range []string{"a", "b"} {
		layout, ok := got.SidebarLayoutsByWorkspace[workspaceID]
		if !ok {
			t.Fatalf("workspace %q has no sidebar layout", workspaceID)
		}
		if layout.Version != models.SidebarLayoutVersion || layout.Revision != 0 {
			t.Fatalf("workspace %q layout = %+v, want version %d revision 0", workspaceID, layout, models.SidebarLayoutVersion)
		}
	}
}

func TestSidebarLayoutPatchIsScopedAndRevisionChecked(t *testing.T) {
	svc, repo, _ := sidebarService(t)
	if _, err := svc.GetUserSettings(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{
		"SidebarLayoutState": {
			"workspace_id": "a",
			"expected_revision": 0,
			"layout": {
				"version": 1,
				"nodes": [{"id":"home","kind":"builtin","visible":false,"destination_id":"home"}]
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarLayoutsByWorkspace["a"].Revision != 1 || got.SidebarLayoutsByWorkspace["a"].Nodes[0].Visible {
		t.Fatalf("workspace a layout = %+v, want hidden home at revision 1", got.SidebarLayoutsByWorkspace["a"])
	}
	if got.SidebarLayoutsByWorkspace["b"].Revision != 0 {
		t.Fatalf("workspace b layout changed: %+v", got.SidebarLayoutsByWorkspace["b"])
	}

	before := repo.snapshot().Revision
	_, err = svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{
		"SidebarLayoutState": {
			"workspace_id": "a",
			"expected_revision": 0,
			"layout": null
		}
	}`))
	if !errors.Is(err, ErrUserSettingsConflict) || repo.snapshot().Revision != before {
		t.Fatalf("stale sidebar layout write = %v, settings revision %d want unchanged %d", err, repo.snapshot().Revision, before)
	}

	reset, err := svc.UpdateUserSettings(context.Background(), sidebarPatch(t, `{
		"SidebarLayoutState": {
			"workspace_id": "a",
			"expected_revision": 1,
			"layout": null
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	resetLayout := reset.SidebarLayoutsByWorkspace["a"]
	if resetLayout.Revision != 2 {
		t.Fatalf("reset revision = %d, want 2", resetLayout.Revision)
	}
	var hasCoordinator bool
	for _, node := range resetLayout.Nodes {
		if node.DestinationID == "coordinators" && node.Visible {
			hasCoordinator = true
			break
		}
	}
	if !hasCoordinator {
		t.Fatalf("reset layout omits visible Coordinator entry: %+v", resetLayout.Nodes)
	}
}

func TestSidebarLayoutRejectsTooManyShortcutGroups(t *testing.T) {
	layout := models.DefaultSidebarLayout()
	for index := 0; index < maxSidebarLayoutShortcutsGroup+1; index++ {
		layout.Nodes = append(layout.Nodes, models.SidebarLayoutNode{
			ID:      fmt.Sprintf("group-%d", index),
			Kind:    models.SidebarLayoutNodeShortcuts,
			Visible: true,
			Name:    fmt.Sprintf("Group %d", index),
		})
	}

	if err := validateSidebarLayout(layout); err == nil {
		t.Fatal("validateSidebarLayout accepted too many shortcut groups")
	}
}

func TestProjectSidebarLayoutsMarksUnsupportedVersion(t *testing.T) {
	settings := &models.UserSettings{
		SidebarLayoutsByWorkspace: map[string]models.SidebarLayout{
			"workspace-1": {Version: models.SidebarLayoutVersion + 1, Revision: 7},
		},
	}

	projected := projectSidebarLayouts(settings, []string{"workspace-1"})["workspace-1"]
	if !projected.UnsupportedVersion {
		t.Fatal("unsupported layout was not marked for recovery")
	}
	if projected.Revision != 7 || projected.Version != models.SidebarLayoutVersion {
		t.Fatalf("projected unsupported layout = %+v", projected)
	}
}

func TestProjectSidebarLayoutsMaterializesCoordinatorWithoutMovingSavedNodes(t *testing.T) {
	saved := models.SidebarLayout{
		Version:  models.SidebarLayoutVersion,
		Revision: 8,
		Nodes: []models.SidebarLayoutNode{
			{ID: "home", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "home"},
			{ID: "canvases", Kind: models.SidebarLayoutNodeBuiltin, Visible: false, DestinationID: "canvases"},
			{ID: "automations", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "automations"},
			{ID: "integrations", Kind: models.SidebarLayoutNodeBuiltin, Visible: false, DestinationID: "integrations"},
		},
	}
	settings := &models.UserSettings{
		SidebarLayoutsByWorkspace: map[string]models.SidebarLayout{"workspace-1": saved},
	}

	projected := projectSidebarLayouts(settings, []string{"workspace-1"})["workspace-1"]
	wantIDs := []string{"home", "canvases", "coordinators", "automations", "integrations"}
	if len(projected.Nodes) != len(wantIDs) {
		t.Fatalf("projected %d nodes, want %d: %+v", len(projected.Nodes), len(wantIDs), projected.Nodes)
	}
	for index, wantID := range wantIDs {
		if projected.Nodes[index].ID != wantID {
			t.Errorf("node %d = %q, want %q", index, projected.Nodes[index].ID, wantID)
		}
	}
	if projected.Nodes[1].Visible || projected.Nodes[2].DestinationID != "coordinators" || !projected.Nodes[2].Visible {
		t.Fatalf("projected existing hidden choice or coordinator default incorrectly: %+v", projected.Nodes)
	}
	if projected.Revision != saved.Revision {
		t.Fatalf("projection revision = %d, want %d", projected.Revision, saved.Revision)
	}
	if !reflect.DeepEqual(settings.SidebarLayoutsByWorkspace["workspace-1"], saved) {
		t.Fatal("projection changed the saved layout")
	}
}

func TestGetUserSettingsMaterializesCoordinatorWithoutPersisting(t *testing.T) {
	saved := models.SidebarLayout{
		Version:  models.SidebarLayoutVersion,
		Revision: 5,
		Nodes: []models.SidebarLayoutNode{
			{ID: "home", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "home"},
			{ID: "coordinators", Kind: models.SidebarLayoutNodeBuiltin, Visible: false, DestinationID: "coordinators"},
			{ID: "automations", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "automations"},
		},
	}
	repo := newCASFakeRepo(&models.UserSettings{
		SidebarWorkspaceVersion:   1,
		SidebarLayoutsByWorkspace: map[string]models.SidebarLayout{"workspace-1": saved},
	})
	svc := newCASService(repo, nil)
	svc.SetSidebarWorkspaceAccess(func(context.Context) ([]string, error) { return []string{"workspace-1"}, nil })

	got, err := svc.GetUserSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	layout := got.SidebarLayoutsByWorkspace["workspace-1"]
	if len(layout.Nodes) != len(saved.Nodes) || layout.Nodes[1].ID != "coordinators" || layout.Nodes[1].Visible {
		t.Fatalf("existing coordinator choice was moved or changed: %+v", layout.Nodes)
	}
	if len(repo.upsertLog()) != 0 || repo.snapshot().Revision != 0 {
		t.Fatalf("ordinary settings read wrote state: upserts=%+v revision=%d", repo.upsertLog(), repo.snapshot().Revision)
	}
}

func TestProjectSidebarLayoutsAppendsCoordinatorWhenAutomationsAreAbsent(t *testing.T) {
	saved := models.SidebarLayout{
		Version: models.SidebarLayoutVersion,
		Nodes: []models.SidebarLayoutNode{
			{ID: "home", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "home"},
			{ID: "custom", Kind: models.SidebarLayoutNodeShortcuts, Visible: true, Name: "Custom"},
		},
	}
	projected := projectSidebarLayouts(&models.UserSettings{
		SidebarLayoutsByWorkspace: map[string]models.SidebarLayout{"workspace-1": saved},
	}, []string{"workspace-1"})["workspace-1"]

	if got := projected.Nodes[len(projected.Nodes)-1].DestinationID; got != "coordinators" {
		t.Fatalf("last projected destination = %q, want coordinators", got)
	}
	if projected.Nodes[1].ID != "custom" || !projected.Nodes[1].Visible {
		t.Fatalf("saved custom node changed position or visibility: %+v", projected.Nodes)
	}
}

func TestProjectSidebarLayoutsAvoidsCoordinatorIDCollisionWithShortcutGroup(t *testing.T) {
	saved := models.SidebarLayout{
		Version: models.SidebarLayoutVersion,
		Nodes: []models.SidebarLayoutNode{
			{
				ID:      "coordinators",
				Kind:    models.SidebarLayoutNodeShortcuts,
				Visible: true,
				Name:    "Pinned",
				Shortcuts: []models.SidebarShortcut{{
					ID:     "home-pin",
					Target: models.SidebarShortcutTarget{Kind: models.SidebarShortcutDestination, ID: "home"},
				}},
			},
			{ID: "automations", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "automations"},
		},
	}
	if err := validateSidebarLayout(saved); err != nil {
		t.Fatalf("legacy layout is invalid: %v", err)
	}

	projected := projectSidebarLayouts(&models.UserSettings{
		SidebarLayoutsByWorkspace: map[string]models.SidebarLayout{"workspace-1": saved},
	}, []string{"workspace-1"})["workspace-1"]
	if err := validateSidebarLayout(projected); err != nil {
		t.Fatalf("projected layout is invalid: %v", err)
	}
	if len(projected.Nodes) != 3 {
		t.Fatalf("projected nodes = %+v, want saved group, Coordinator, and Automations", projected.Nodes)
	}
	group, coordinator := projected.Nodes[0], projected.Nodes[1]
	if group.ID != "coordinators" || group.Kind != models.SidebarLayoutNodeShortcuts || group.Name != "Pinned" || len(group.Shortcuts) != 1 {
		t.Fatalf("saved shortcut group changed: %+v", group)
	}
	if coordinator.ID != "coordinators-1" || coordinator.Kind != models.SidebarLayoutNodeBuiltin || coordinator.DestinationID != "coordinators" {
		t.Fatalf("materialized Coordinator = %+v, want a collision-free builtin identity", coordinator)
	}
}

func TestProjectedLegacyFortyNodeLayoutCanSaveCoordinatorVisibility(t *testing.T) {
	const legacyNodeLimit = 40
	nodes := []models.SidebarLayoutNode{
		{ID: "inbox", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "inbox"},
		{ID: "needs-you-inbox", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "needs_you_inbox"},
		{ID: "automations", Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: "automations"},
	}
	for len(nodes) < legacyNodeLimit {
		id := fmt.Sprintf("legacy-%02d", len(nodes))
		nodes = append(nodes, models.SidebarLayoutNode{
			ID: id, Kind: models.SidebarLayoutNodeBuiltin, Visible: true, DestinationID: id,
		})
	}
	saved := models.SidebarLayout{Version: models.SidebarLayoutVersion, Nodes: nodes}
	if err := validateSidebarLayout(saved); err != nil {
		t.Fatalf("legacy layout is invalid: %v", err)
	}

	repo := newCASFakeRepo(&models.UserSettings{
		UserID:                  store.DefaultUserID,
		SidebarWorkspaceVersion: 1,
		SidebarLayoutsByWorkspace: map[string]models.SidebarLayout{
			"workspace-1": saved,
		},
	})
	svc := newCASService(repo, nil)
	svc.SetSidebarWorkspaceAccess(func(context.Context) ([]string, error) {
		return []string{"workspace-1"}, nil
	})
	projectedSettings, err := svc.GetUserSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projected := projectedSettings.SidebarLayoutsByWorkspace["workspace-1"]
	if len(projected.Nodes) != legacyNodeLimit+1 {
		t.Fatalf("projected node count = %d, want %d", len(projected.Nodes), legacyNodeLimit+1)
	}

	coordinatorIndex := -1
	for index, node := range projected.Nodes {
		if node.DestinationID == "coordinators" {
			coordinatorIndex = index
			projected.Nodes[index].Visible = false
			break
		}
	}
	if coordinatorIndex < 0 {
		t.Fatal("projected Coordinator node not found before save")
	}
	updated, err := svc.UpdateUserSettings(context.Background(), &UpdateUserSettingsRequest{
		SidebarLayoutState: &models.SidebarLayoutPatch{
			WorkspaceID:      "workspace-1",
			ExpectedRevision: projected.Revision,
			Layout:           &projected,
		},
	})
	if err != nil {
		t.Fatalf("save projected layout with Coordinator hidden: %v", err)
	}
	stored := repo.snapshot().SidebarLayoutsByWorkspace["workspace-1"]
	if len(stored.Nodes) != legacyNodeLimit+1 || stored.Revision != 1 {
		t.Fatalf("stored layout has %d nodes at revision %d, want %d at revision 1", len(stored.Nodes), stored.Revision, legacyNodeLimit+1)
	}
	if updated.SidebarLayoutsByWorkspace["workspace-1"].Nodes[coordinatorIndex].Visible {
		t.Fatal("saved Coordinator visibility was not retained")
	}
}
