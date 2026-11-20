package controller

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/common/modelcatalog"
)

func groupedHostUtilityOption() hostutility.ConfigOption {
	return hostutility.ConfigOption{
		Type: "select", ID: "model", Name: "Model", CurrentValue: "pair-a",
		Options: []hostutility.ConfigOptionChoice{
			{Value: "pair-a", Name: "Balanced", Description: "Lead A with Sidekick A"},
			{Value: "solo", Name: "Balanced"},
		},
		Catalog: &modelcatalog.Catalog{
			Version: modelcatalog.SchemaVersion,
			Nodes: []modelcatalog.Node{
				{
					Kind: modelcatalog.NodeKindGroup, ID: "fusion", Label: "Fusion",
					Children: []modelcatalog.Node{
						{Kind: modelcatalog.NodeKindChoice, Value: "pair-a", Label: "Balanced", Description: "Lead A with Sidekick A"},
					},
				},
				{
					Kind: modelcatalog.NodeKindGroup, ID: "standard", Label: "Standard",
					Children: []modelcatalog.Node{
						{Kind: modelcatalog.NodeKindChoice, Value: "solo", Label: "Balanced"},
					},
				},
			},
		},
	}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
func TestGroupedCatalogDTOProjection(t *testing.T) {
	projected := configOptionDTOs([]hostutility.ConfigOption{groupedHostUtilityOption()})
	if len(projected) != 1 || projected[0].Catalog == nil {
		t.Fatalf("settings projection dropped the catalog: %#v", projected)
	}
	if len(projected[0].Options) != 2 {
		t.Fatalf("flat compatibility projection = %#v", projected[0].Options)
	}
	raw, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal DTOs: %v", err)
	}
	var decoded []struct {
		Catalog *modelcatalog.Catalog `json:"catalog"`
		Options []struct {
			Value string `json:"value"`
		} `json:"options"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal DTOs: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Catalog == nil {
		t.Fatalf("catalog missing after JSON round trip: %s", raw)
	}
	if decoded[0].Catalog.Nodes[0].ID != "fusion" ||
		decoded[0].Catalog.Nodes[1].Children[0].Value != "solo" {
		t.Fatalf("catalog structure changed in transit: %s", raw)
	}
	if len(decoded[0].Options) != 2 || decoded[0].Options[1].Value != "solo" {
		t.Fatalf("flat choices changed in transit: %s", raw)
	}
}

func TestGroupedCatalogDTOProjectionOmittedForFlatOptions(t *testing.T) {
	projected := configOptionDTOs([]hostutility.ConfigOption{{
		Type: "select", ID: "effort", Name: "Effort", CurrentValue: "high",
		Options: []hostutility.ConfigOptionChoice{{Value: "high", Name: "High"}},
	}})
	raw, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal DTOs: %v", err)
	}
	if len(projected) != 1 || projected[0].Catalog != nil {
		t.Fatalf("flat option published a catalog: %s", raw)
	}
}
