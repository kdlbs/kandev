package acp

import (
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/common/modelcatalog"
)

func catalogTestOptions() []acpsdk.SessionConfigOption {
	description := "Lead A with Sidekick A"
	grouped := acpsdk.SessionConfigSelectOptionsGrouped{
		{
			Group: "fusion", Name: "Fusion",
			Options: []acpsdk.SessionConfigSelectOption{
				{Value: "fusion-a", Name: "Balanced", Description: &description},
				{Value: "fusion-b", Name: "Balanced"},
			},
		},
		{
			Group: "standard", Name: "Standard",
			Options: []acpsdk.SessionConfigSelectOption{
				{Value: "solo", Name: "Balanced"},
			},
		},
	}
	return []acpsdk.SessionConfigOption{{
		Select: &acpsdk.SessionConfigOptionSelect{
			Type: "select", Id: "model", Name: "Model", CurrentValue: "fusion-a",
			Options: acpsdk.SessionConfigSelectOptions{Grouped: &grouped},
		},
	}}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
func TestGroupedCatalogPreservedLiveAndProbe(t *testing.T) {
	converted := convertACPConfigOptions(catalogTestOptions())
	if len(converted) != 1 {
		t.Fatalf("converted options = %d, want 1", len(converted))
	}
	option := converted[0]
	if len(option.Options) != 3 {
		t.Fatalf("flat compatibility options = %#v, want three exact choices", option.Options)
	}
	if option.Options[0].Value != "fusion-a" || option.Options[0].Description != "Lead A with Sidekick A" {
		t.Fatalf("flat choices lost descriptions or values: %#v", option.Options)
	}
	catalog := option.Catalog
	if catalog == nil || catalog.Version != modelcatalog.SchemaVersion {
		t.Fatalf("catalog = %#v, want version %d", catalog, modelcatalog.SchemaVersion)
	}
	if len(catalog.Nodes) != 2 {
		t.Fatalf("catalog nodes = %#v, want two native groups", catalog.Nodes)
	}
	fusion := catalog.Nodes[0]
	if fusion.Kind != modelcatalog.NodeKindGroup || fusion.ID != "fusion" || fusion.Label != "Fusion" {
		t.Fatalf("first group = %#v", fusion)
	}
	if len(fusion.Children) != 2 || fusion.Children[1].Label != "Balanced" || fusion.Children[0].Label != "Balanced" {
		t.Fatalf("repeated labels were collapsed or reordered: %#v", fusion.Children)
	}
	if fusion.Children[0].Value != "fusion-a" || fusion.Children[1].Value != "fusion-b" {
		t.Fatalf("exact values = %#v", fusion.Children)
	}
	standard := catalog.Nodes[1]
	if standard.ID != "standard" || len(standard.Children) != 1 || standard.Children[0].Value != "solo" {
		t.Fatalf("second group = %#v", standard)
	}

	raw, err := json.Marshal(option)
	if err != nil {
		t.Fatalf("marshal option: %v", err)
	}
	var roundTrip struct {
		Catalog *modelcatalog.Catalog `json:"catalog"`
		Options []struct {
			Value string `json:"value"`
			Name  string `json:"name"`
		} `json:"options"`
	}
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal option: %v", err)
	}
	if roundTrip.Catalog == nil || len(roundTrip.Catalog.Nodes) != 2 ||
		roundTrip.Catalog.Nodes[0].ID != "fusion" || roundTrip.Catalog.Nodes[1].Children[0].Value != "solo" {
		t.Fatalf("catalog round trip = %s", raw)
	}
	if len(roundTrip.Options) != 3 {
		t.Fatalf("flat projection round trip = %s", raw)
	}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
func TestFlatCatalogKeepsExistingBehavior(t *testing.T) {
	choices := acpsdk.SessionConfigSelectOptionsUngrouped{
		{Value: "low", Name: "Low"},
		{Value: "high", Name: "High"},
	}
	converted := convertACPConfigOptions([]acpsdk.SessionConfigOption{{
		Select: &acpsdk.SessionConfigOptionSelect{
			Type: "select", Id: "effort", Name: "Effort", CurrentValue: "high",
			Options: acpsdk.SessionConfigSelectOptions{Ungrouped: &choices},
		},
	}})
	if len(converted) != 1 || converted[0].Catalog != nil {
		t.Fatalf("flat provider published a catalog: %#v", converted)
	}
	if len(converted[0].Options) != 2 || converted[0].Options[0].Value != "low" {
		t.Fatalf("flat choices = %#v", converted[0].Options)
	}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.4
func TestInvalidRichCatalogFallsBackToFlatChoices(t *testing.T) {
	grouped := acpsdk.SessionConfigSelectOptionsGrouped{
		{
			Group: "dup", Name: "Dup",
			Options: []acpsdk.SessionConfigSelectOption{{Value: "a", Name: "A"}},
		},
		{
			Group: "dup", Name: "Dup again",
			Options: []acpsdk.SessionConfigSelectOption{{Value: "b", Name: "B"}},
		},
	}
	converted := convertACPConfigOptions([]acpsdk.SessionConfigOption{{
		Select: &acpsdk.SessionConfigOptionSelect{
			Type: "select", Id: "model", Name: "Model", CurrentValue: "a",
			Options: acpsdk.SessionConfigSelectOptions{Grouped: &grouped},
		},
	}})
	if len(converted) != 1 || converted[0].Catalog != nil {
		t.Fatalf("invalid richer metadata published a catalog: %#v", converted)
	}
	if len(converted[0].Options) != 2 || converted[0].Options[1].Value != "b" {
		t.Fatalf("validated flat choices were dropped: %#v", converted[0].Options)
	}
}

func TestUntypedGroupedMetadataPublishesCatalog(t *testing.T) {
	options := []any{
		map[string]any{
			"group": "fusion", "name": "Fusion",
			"options": []any{
				map[string]any{"value": "pair-a", "name": "Balanced", "description": "Lead A + Sidekick A"},
			},
		},
	}
	meta := map[string]any{
		"configOptions": []any{
			map[string]any{
				"type": "select", "id": "model", "name": "Model", "currentValue": "pair-a",
				"options": options,
			},
		},
	}
	converted := extractConfigOptions(meta)
	if len(converted) != 1 || converted[0].Catalog == nil {
		t.Fatalf("untyped grouped metadata was flattened: %#v", converted)
	}
	node := converted[0].Catalog.Nodes[0]
	if node.ID != "fusion" || node.Label != "Fusion" || node.Children[0].Value != "pair-a" ||
		node.Children[0].Description != "Lead A + Sidekick A" {
		t.Fatalf("untyped catalog = %#v", node)
	}
	if len(converted[0].Options) != 1 || converted[0].Options[0].Value != "pair-a" {
		t.Fatalf("untyped flat projection = %#v", converted[0].Options)
	}
}
