package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/user/models"
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
