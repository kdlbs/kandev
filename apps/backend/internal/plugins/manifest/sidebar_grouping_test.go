package manifest

import (
	"fmt"
	"strings"
	"testing"
)

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
func TestSidebarPluginGroupingManifestRejectsInvalidDeclarations(t *testing.T) {
	cases := []struct{ name, actions, groupings string }{
		{"undeclared action", "", "  - {id: labels, label: Labels, action: labels.snapshot}"},
		{"task scoped action", "actions:\n  - {key: labels.snapshot, scope: task, max_body_bytes: 16}\n", "  - {id: labels, label: Labels, action: labels.snapshot}"},
		{"admin action", "actions:\n  - {key: labels.snapshot, scope: workspace, access: admin, max_body_bytes: 16}\n", "  - {id: labels, label: Labels, action: labels.snapshot}"},
		{"invalid identity", groupingActionYAML, "  - {id: 'not:canonical', label: Labels, action: labels.snapshot}"},
		{"long identity", groupingActionYAML, fmt.Sprintf("  - {id: '%s', label: Labels, action: labels.snapshot}", strings.Repeat("x", 129))},
		{"empty label", groupingActionYAML, "  - {id: labels, label: '', action: labels.snapshot}"},
		{"duplicate identity", groupingActionYAML, "  - {id: labels, label: Labels, action: labels.snapshot}\n  - {id: labels, label: Other, action: labels.snapshot}"},
		{"too many groupings", groupingActionYAML, repeatedGroupingYAML(17)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Parse([]byte(validManifestYAML + "\nmin_kandev_version: '" + MinimumSidebarGroupingVersion + "'\n" + tc.actions + "sidebar_groupings:\n" + tc.groupings + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "sidebar grouping") {
				t.Fatalf("Validate() = %v; want sidebar grouping declaration rejection", err)
			}
		})
	}
}

const groupingActionYAML = "actions:\n  - {key: labels.snapshot, scope: workspace, max_body_bytes: 16}\n"

func repeatedGroupingYAML(count int) string {
	var out strings.Builder
	for i := range count {
		fmt.Fprintf(&out, "  - {id: labels%d, label: Labels, action: labels.snapshot}\n", i)
	}
	return out.String()
}

func TestSidebarPluginGroupingManifestRoundTripAndClone(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + "\nmin_kandev_version: '" + MinimumSidebarGroupingVersion + "'\n" + groupingActionYAML + `sidebar_groupings:
  - {id: labels, label: Labels, label_key: plugin:labels, action: labels.snapshot}
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(m.SidebarGroupings) != 1 || m.SidebarGroupings[0].Action != "labels.snapshot" || m.SidebarGroupings[0].LabelKey != "plugin:labels" {
		t.Fatalf("grouping declaration not preserved: %+v", m.SidebarGroupings)
	}
	clone := m.Clone()
	clone.SidebarGroupings[0].Label = "Changed"
	if m.SidebarGroupings[0].Label != "Labels" {
		t.Fatal("clone changed the stored grouping label")
	}
}

func TestSidebarPluginGroupingMinimumHostVersion(t *testing.T) {
	for _, minimum := range []string{"", "0.101.0", "0.102.0", "0.103.0"} {
		t.Run(minimum, func(t *testing.T) {
			m := managedManifest(t)
			m.MinKandevVersion = minimum
			m.Actions = []Action{{Key: "labels.snapshot", ResourceScope: ActionScopeWorkspace, MaxBodyBytes: 16}}
			m.SidebarGroupings = []SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
			err := m.Validate()
			if minimum == "0.103.0" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "sidebar_groupings requires min_kandev_version") {
				t.Fatalf("unsupported host can silently install a complete-grouping provider: %v", err)
			}
		})
	}
}

func TestSidebarGroupingPackageRejectsReleasedHostWithoutProviders(t *testing.T) {
	if err := CheckMinimumKandevVersion(MinimumSidebarGroupingVersion, "0.102.0"); err == nil {
		t.Fatal("released host without sidebar providers accepted the package")
	}
	if err := CheckMinimumKandevVersion(MinimumSidebarGroupingVersion, "0.103.0"); err != nil {
		t.Fatal(err)
	}
}

func TestSidebarPluginGroupingRequiresAddressablePluginIdentity(t *testing.T) {
	m := managedManifest(t)
	m.ID = strings.Repeat("a", 129)
	m.MinKandevVersion = MinimumSidebarGroupingVersion
	m.Actions = []Action{{Key: "labels.snapshot", ResourceScope: ActionScopeWorkspace, MaxBodyBytes: 16}}
	m.SidebarGroupings = []SidebarGrouping{{ID: "labels", Label: "Labels", Action: "labels.snapshot"}}
	if err := m.Validate(); err == nil {
		t.Fatal("installed a provider whose canonical reference cannot be queried")
	}
}
