package utility

import (
	"encoding/json"
	"testing"

	"github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/common/modelcatalog"
)

func groupedProbeOptions() []acp.SessionConfigOption {
	description := "Lead A with Sidekick A"
	grouped := acp.SessionConfigSelectOptionsGrouped{
		{
			Group: "fusion", Name: "Fusion",
			Options: []acp.SessionConfigSelectOption{
				{Value: "pair-a", Name: "Balanced", Description: &description},
				{Value: "pair-b", Name: "Balanced"},
			},
		},
		{
			Group: "standard", Name: "Standard",
			Options: []acp.SessionConfigSelectOption{
				{Value: "solo", Name: "Balanced"},
			},
		},
	}
	return []acp.SessionConfigOption{{
		Select: &acp.SessionConfigOptionSelect{
			Type: "select", Id: "model", Name: "Model", CurrentValue: "pair-a",
			Options: acp.SessionConfigSelectOptions{Grouped: &grouped},
		},
	}}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
func TestGroupedCatalogPreservedInProbe(t *testing.T) {
	converted := probeConfigOptions(groupedProbeOptions())
	if len(converted) != 1 {
		t.Fatalf("converted options = %d, want 1", len(converted))
	}
	option := converted[0]
	if len(option.Options) != 3 || option.Options[2].Value != "solo" {
		t.Fatalf("flat compatibility options = %#v", option.Options)
	}
	if option.Catalog == nil || option.Catalog.Version != modelcatalog.SchemaVersion {
		t.Fatalf("catalog = %#v", option.Catalog)
	}
	if len(option.Catalog.Nodes) != 2 || option.Catalog.Nodes[0].ID != "fusion" ||
		option.Catalog.Nodes[0].Label != "Fusion" {
		t.Fatalf("native groups = %#v", option.Catalog.Nodes)
	}
	if got := option.Catalog.Nodes[0].Children[0].Description; got != "Lead A with Sidekick A" {
		t.Fatalf("choice description = %q", got)
	}

	raw, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("marshal probe options: %v", err)
	}
	var roundTrip []ProbeConfigOption
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal probe options: %v", err)
	}
	if len(roundTrip) != 1 || roundTrip[0].Catalog == nil ||
		roundTrip[0].Catalog.Nodes[1].Children[0].Value != "solo" {
		t.Fatalf("probe catalog round trip = %s", raw)
	}
}
