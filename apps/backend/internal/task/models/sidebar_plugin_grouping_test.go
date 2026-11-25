package models

import "testing"

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
func TestSidebarPluginGroupingQueryIdentity(t *testing.T) {
	q := SidebarTaskViewQuery{Sort: SidebarTaskViewSort{Key: "title", Direction: "asc"}, Group: "plugin:acme.labels:shared-tags", Page: 1, PageSize: 100, Locale: "en"}
	if err := q.Validate(); err != nil {
		t.Fatalf("syntactically valid saved provider rejected without live registry: %v", err)
	}
	for _, bad := range []string{"plugin:acme", "plugin::labels", "plugin:acme:labels:extra", "plugin:Acme:labels", "plugin:acme:../labels", "facet:acme:labels"} {
		q.Group = bad
		if err := q.Validate(); err == nil {
			t.Errorf("accepted malformed grouping %q", bad)
		}
	}
}
