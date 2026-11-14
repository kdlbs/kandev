package hostutility

import (
	"encoding/json"
	"testing"

	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/modelcatalog"
)

func groupedProbeConfigOption() agentctlutil.ProbeConfigOption {
	return agentctlutil.ProbeConfigOption{
		Type: "select", ID: "model", Name: "Model", CurrentValue: "pair-a",
		Options: []agentctlutil.ProbeConfigOptionChoice{
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

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
func TestGroupedCatalogRoundTrip(t *testing.T) {
	converted := configOptionsFromProbe([]agentctlutil.ProbeConfigOption{groupedProbeConfigOption()})
	if len(converted) != 1 || converted[0].Catalog == nil {
		t.Fatalf("host utility projection dropped the catalog: %#v", converted)
	}
	if len(converted[0].Options) != 2 {
		t.Fatalf("flat projection = %#v", converted[0].Options)
	}
	catalog := decodeGroupedCatalogProjection(t, converted)
	assertGroupedCatalogShape(t, catalog)

	cloned := cloneModelConfigResolution(ModelConfigResolution{ConfigOptions: converted})
	cloneCatalog := cloned.ConfigOptions[0].Catalog
	assertGroupedCatalogShape(t, cloneCatalog)
	// The cache clone must isolate nested catalog data from later mutation.
	cloneCatalog.Nodes[0].Children[0].Description = "mutated"
	if converted[0].Catalog.Nodes[0].Children[0].Description == "mutated" {
		t.Fatal("cloneModelConfigResolution shares catalog nodes with the cached result")
	}
}

func decodeGroupedCatalogProjection(t *testing.T, options []ConfigOption) *modelcatalog.Catalog {
	t.Helper()
	raw, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	var decoded []struct {
		Catalog *modelcatalog.Catalog `json:"catalog"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded) != 1 || decoded[0].Catalog == nil {
		t.Fatalf("catalog missing after JSON round trip: %s", raw)
	}
	return decoded[0].Catalog
}

func assertGroupedCatalogShape(t *testing.T, catalog *modelcatalog.Catalog) {
	t.Helper()
	if catalog == nil || len(catalog.Nodes) != 2 {
		t.Fatalf("catalog = %#v, want two groups", catalog)
	}
	if catalog.Nodes[0].ID != "fusion" || catalog.Nodes[0].Label != "Fusion" {
		t.Fatalf("first group = %#v", catalog.Nodes[0])
	}
	if catalog.Nodes[1].ID != "standard" || len(catalog.Nodes[1].Children) != 1 {
		t.Fatalf("second group = %#v", catalog.Nodes[1])
	}
	leaf := catalog.Nodes[0].Children[0]
	if leaf.Value != "pair-a" || leaf.Label != "Balanced" || leaf.Description != "Lead A with Sidekick A" {
		t.Fatalf("first leaf = %#v", leaf)
	}
	if catalog.Nodes[1].Children[0].Value != "solo" {
		t.Fatalf("second group leaf = %#v", catalog.Nodes[1].Children[0])
	}
}
