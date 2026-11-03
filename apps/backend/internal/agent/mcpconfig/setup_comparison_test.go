package mcpconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func comparisonSetup(servers ...MCPSetupServer) *MCPSetupSnapshot {
	return &MCPSetupSnapshot{Version: 1, Servers: append([]MCPSetupServer{}, servers...)}
}

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.15
func TestMCPSetupComparisonNamesAddRemoveUpdate(t *testing.T) {
	old := MCPSetupServer{DefinitionID: "old", Revision: 1, Enabled: true, DisplayName: "Deleted server", RuntimeName: "old"}
	updated := MCPSetupServer{DefinitionID: "changed", Revision: 1, Enabled: true, DisplayName: "Old name", RuntimeName: "changed"}
	next := updated
	next.Revision = 2
	next.DisplayName = "New name"
	added := MCPSetupServer{DefinitionID: "new", Revision: 1, Enabled: true, DisplayName: "New server", RuntimeName: "new"}
	state := SessionMCPSelectionState{DesiredRevision: 3, AppliedRevision: 2, AcceptedSetup: comparisonSetup(old, updated)}
	got := CompareMCPSetups(state, comparisonSetup(next, added))
	if got.Status != "known" || len(got.Changes) != 3 {
		t.Fatalf("comparison = %#v", got)
	}
	actions := map[string]MCPSetupChange{}
	for _, change := range got.Changes {
		actions[change.DefinitionID] = change
	}
	if actions["old"].Action != "remove" || actions["old"].Before.DisplayName != "Deleted server" || actions["new"].Action != "add" || actions["changed"].Action != "update" {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestMCPSetupComparisonEmptyUnknownAndInvalid(t *testing.T) {
	for _, test := range []struct {
		name          string
		before, after *MCPSetupSnapshot
		want          string
	}{
		{"empty", comparisonSetup(), comparisonSetup(), "known"},
		{"unknown", nil, comparisonSetup(), "unavailable"},
		{"desired unreadable", comparisonSetup(), nil, "unavailable"},
		{"duplicate", comparisonSetup(), comparisonSetup(MCPSetupServer{DefinitionID: "x", Revision: 1, Enabled: true, DisplayName: "X", RuntimeName: "x"}, MCPSetupServer{DefinitionID: "x", Revision: 1, Enabled: true, DisplayName: "X", RuntimeName: "x"}), "unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := CompareMCPSetups(SessionMCPSelectionState{AcceptedSetup: test.before}, test.after)
			if got.Status != test.want || len(got.Changes) != 0 {
				t.Fatalf("comparison = %#v", got)
			}
		})
	}
	var unsafe MCPSetupSnapshot
	if err := json.Unmarshal([]byte(`{"version":1,"servers":[{"definition_id":"x","revision":1,"enabled":true,"display_name":"X","runtime_name":"x","env":{"SECRET":"hidden"}}]}`), &unsafe); err != nil {
		t.Fatal(err)
	}
	got := CompareMCPSetups(SessionMCPSelectionState{AcceptedSetup: comparisonSetup()}, &unsafe)
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "hidden") {
		t.Fatalf("secret in comparison: %s", encoded)
	}
	unsafe.Servers[0].DisplayName = strings.Repeat("x", maxMCPSetupBytes)
	if CompareMCPSetups(SessionMCPSelectionState{AcceptedSetup: comparisonSetup()}, &unsafe).Status != "unavailable" {
		t.Fatal("oversized snapshot treated as exact")
	}
}

func TestMCPSetupComparisonBoundsCombinedProjection(t *testing.T) {
	before := comparisonSetup(MCPSetupServer{DefinitionID: "x", Revision: 1, Enabled: true, DisplayName: strings.Repeat("a", 140*1024), RuntimeName: "x"})
	after := comparisonSetup(MCPSetupServer{DefinitionID: "x", Revision: 2, Enabled: true, DisplayName: strings.Repeat("b", 140*1024), RuntimeName: "x"})
	if !ValidMCPSetup(before) || !ValidMCPSetup(after) {
		t.Fatal("fixture snapshots must individually fit")
	}
	got := CompareMCPSetups(SessionMCPSelectionState{AcceptedSetup: before}, after)
	if got.Status != "unavailable" || len(got.Changes) != 0 {
		t.Fatal("oversized combined projection treated as exact")
	}
}
