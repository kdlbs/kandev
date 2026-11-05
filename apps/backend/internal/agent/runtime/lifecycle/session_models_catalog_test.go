package lifecycle

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/modelcatalog"
)

func groupedSnapshotCatalog() *modelcatalog.Catalog {
	return &modelcatalog.Catalog{
		Version: modelcatalog.SchemaVersion,
		Nodes: []modelcatalog.Node{
			{
				Kind: modelcatalog.NodeKindGroup, ID: "fusion", Label: "Fusion",
				Children: []modelcatalog.Node{
					{Kind: modelcatalog.NodeKindChoice, Value: "pair-a", Label: "Balanced", Description: "Lead A with Sidekick A"},
					{Kind: modelcatalog.NodeKindChoice, Value: "pair-b", Label: "Balanced"},
				},
			},
			{
				Kind: modelcatalog.NodeKindGroup, ID: "standard", Label: "Standard",
				Children: []modelcatalog.Node{
					{Kind: modelcatalog.NodeKindChoice, Value: "solo", Label: "Solo"},
				},
			},
		},
		Compositions: []modelcatalog.Composition{{
			ID: "fusion", GroupID: "fusion", Label: "Fusion",
			Dimensions: []modelcatalog.Dimension{
				{ID: "lead", Label: "Lead", Choices: []modelcatalog.Choose{{Value: "lead-a", Label: "Lead A"}}},
			},
			Bindings: []modelcatalog.Binding{
				{Values: map[string]string{"lead": "lead-a"}, Model: "pair-a"},
			},
		}},
	}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.3
func TestGroupedCatalogRestore(t *testing.T) {
	snapshot := SessionModelsSnapshot{
		CurrentModelID:       "pair-a",
		ConfigOptionsSettled: true,
		ConfigOptions: []streams.ConfigOption{{
			Type: "select", ID: "model", Name: "Model", CurrentValue: "pair-a",
			Options: []streams.ConfigOptionValue{
				{Value: "pair-a", Name: "Balanced", Description: "Lead A with Sidekick A"},
				{Value: "pair-b", Name: "Balanced"},
				{Value: "solo", Name: "Solo"},
			},
			Catalog: groupedSnapshotCatalog(),
		}},
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("rehydrate metadata: %v", err)
	}
	loaded, ok := LoadSessionModelsSnapshot(metadata)
	if !ok {
		t.Fatal("snapshot was not restored")
	}
	if loaded.CurrentModelID != "pair-a" || len(loaded.ConfigOptions) != 1 {
		t.Fatalf("restored snapshot = %#v", loaded)
	}
	option := loaded.ConfigOptions[0]
	if option.Catalog == nil {
		t.Fatal("restored snapshot lost the catalog")
	}
	if len(option.Catalog.Nodes) != 2 || option.Catalog.Nodes[0].ID != "fusion" ||
		option.Catalog.Nodes[1].Children[0].Value != "solo" {
		t.Fatalf("restored groups = %#v", option.Catalog.Nodes)
	}
	if option.Catalog.Nodes[0].Children[0].Description != "Lead A with Sidekick A" {
		t.Fatalf("restored choice description = %q", option.Catalog.Nodes[0].Children[0].Description)
	}
	composition := option.Catalog.Compositions[0]
	if model, ok := composition.Resolve(map[string]string{"lead": "lead-a"}); !ok || model != "pair-a" {
		t.Fatalf("restored composition resolved %q/%t", model, ok)
	}
	if len(option.Options) != 3 {
		t.Fatalf("restored flat projection = %#v", option.Options)
	}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1
func TestGroupedCatalogLegacyFlatSnapshot(t *testing.T) {
	snapshot := SessionModelsSnapshot{
		CurrentModelID: "model-a",
		ConfigOptions: []streams.ConfigOption{{
			Type: "select", ID: "model", Name: "Model", CurrentValue: "model-a",
			Options: []streams.ConfigOptionValue{{Value: "model-a", Name: "Model A"}},
		}},
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("rehydrate metadata: %v", err)
	}
	loaded, ok := LoadSessionModelsSnapshot(metadata)
	if !ok || len(loaded.ConfigOptions) != 1 {
		t.Fatalf("legacy snapshot = %#v/%t", loaded, ok)
	}
	if loaded.ConfigOptions[0].Catalog != nil {
		t.Fatalf("older flat snapshot invented a catalog: %#v", loaded.ConfigOptions[0].Catalog)
	}
	if len(loaded.ConfigOptions[0].Options) != 1 {
		t.Fatalf("legacy flat choices = %#v", loaded.ConfigOptions[0].Options)
	}
}

// @covers AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.3
func TestComposedConfigurationPartialApply(t *testing.T) {
	// The provider confirmed the exact model and one option before a later
	// option failed. Restore must keep that confirmed state and the catalog,
	// and must not borrow the previous model or an optimistic option value.
	snapshot := SessionModelsSnapshot{
		CurrentModelID:       "pair-b",
		ConfigOptionsSettled: true,
		SettingsAttemptID:    "attempt-1",
		ConfigOptions: []streams.ConfigOption{
			{Type: "select", ID: "model", Name: "Model", CurrentValue: "pair-b", Category: "model", Catalog: groupedSnapshotCatalog()},
			{Type: "select", ID: "thinking", Name: "Thinking", CurrentValue: "high"},
			{Type: "select", ID: "speed", Name: "Speed", CurrentValue: "standard"},
		},
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatalf("rehydrate metadata: %v", err)
	}
	loaded, ok := LoadSessionModelsSnapshot(metadata)
	if !ok {
		t.Fatal("partial composed configuration was not restored")
	}
	if loaded.CurrentModelID != "pair-b" {
		t.Fatalf("restored model = %q, want the confirmed composed model", loaded.CurrentModelID)
	}
	values := make(map[string]string, len(loaded.ConfigOptions))
	for _, option := range loaded.ConfigOptions {
		values[option.ID] = option.CurrentValue
	}
	if values["thinking"] != "high" || values["speed"] != "standard" {
		t.Fatalf("confirmed option values = %#v", values)
	}
	if values["model"] != "pair-b" {
		t.Fatalf("confirmed model option = %q", values["model"])
	}
	if len(loaded.ConfigOptions) != 3 || loaded.ConfigOptions[0].Catalog == nil {
		t.Fatalf("restored composed snapshot = %#v", loaded.ConfigOptions)
	}
}
