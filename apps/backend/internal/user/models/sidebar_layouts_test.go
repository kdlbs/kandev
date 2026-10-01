package models

import "testing"

func TestDefaultSidebarLayoutPrimaryActionOrder(t *testing.T) {
	layout := DefaultSidebarLayout()
	want := []string{"new_task", "home", "automations", "canvases", "integrations"}
	if len(layout.Nodes) != len(want) {
		t.Fatalf("got %d nodes, want %d", len(layout.Nodes), len(want))
	}
	for i, id := range want {
		if layout.Nodes[i].DestinationID != id || !layout.Nodes[i].Visible {
			t.Errorf("node %d: got %+v, want visible %s", i, layout.Nodes[i], id)
		}
	}
	layout.Nodes[0].Visible = false
	if !DefaultSidebarLayout().Nodes[0].Visible {
		t.Fatal("editing a default layout mutated future defaults")
	}
}
