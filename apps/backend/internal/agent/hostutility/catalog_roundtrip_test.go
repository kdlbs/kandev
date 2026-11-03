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
	count := countGroupedCatalogProjection(converted)
	if count != 2 {
		t.Fatalf("catalog groups = %d, want 2 after JSON round trip", count)
	}
	cloned := cloneModelConfigResolution(ModelConfigResolution{ConfigOptions: converted})
	if cloned.ConfigOptions[0].Catalog == nil ||
		cloned.ConfigOptions[0].Catalog.Nodes[0].ID != "fusion" {
		t.Fatalf("cached resolution dropped the catalog: %#v", cloned)
	}
}

func countGroupedCatalogProjection(options []ConfigOption) int {
	raw, err := json.Marshal(options)
	if err != nil {
		return -1
	}
	var decoded []struct {
		Catalog *modelcatalog.Catalog `json:"catalog"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded) != 1 || decoded[0].Catalog == nil {
		return -1
	}
	return len(decoded[0].Catalog.Nodes)
}
