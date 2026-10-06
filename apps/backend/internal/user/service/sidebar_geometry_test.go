package service

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/user/models"
	"testing"
)

func TestSidebarNavigationGeometry(t *testing.T) {
	for _, tc := range []struct {
		height int
		valid  bool
	}{{64, true}, {1600, true}, {0, false}, {63, false}, {1601, false}} {
		var layout models.SidebarLayout
		raw, _ := json.Marshal(map[string]any{"version": 1, "nodes": []any{}, "navigation_height": tc.height, "navigation_expanded": true})
		if err := json.Unmarshal(raw, &layout); err != nil {
			t.Fatal(err)
		}
		err := validateSidebarLayout(layout)
		if (err == nil) != tc.valid {
			t.Fatalf("height %d: %v", tc.height, err)
		}
		if tc.valid {
			saved, _ := json.Marshal(layout)
			var fields map[string]any
			_ = json.Unmarshal(saved, &fields)
			if fields["navigation_height"] != float64(tc.height) || fields["navigation_expanded"] != true {
				t.Fatalf("lost geometry: %s", saved)
			}
		}
	}
}

func TestSidebarInboxVisibilityIsCustomizable(t *testing.T) {
	for _, destination := range []string{"inbox", "needs_you_inbox"} {
		layout := models.SidebarLayout{Version: 1, Nodes: []models.SidebarLayoutNode{{ID: destination, Kind: "builtin", DestinationID: destination, Visible: false}}}
		if err := validateSidebarLayout(layout); err != nil {
			t.Fatal(err)
		}
	}
	layout := models.SidebarLayout{Version: 1, Nodes: []models.SidebarLayoutNode{{ID: "tasks", Kind: "builtin", DestinationID: "tasks"}}}
	if validateSidebarLayout(layout) == nil {
		t.Fatal("Tasks must remain fixed")
	}
}
