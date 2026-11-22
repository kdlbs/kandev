package acpcompat

import (
	"testing"

	"github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/common/modelcatalog"
)

func TestCatalogFromSelectOptionsPreservesNativeGroups(t *testing.T) {
	grouped := acp.SessionConfigSelectOptionsGrouped{
		{
			Group: "fusion", Name: "Fusion",
			Options: []acp.SessionConfigSelectOption{
				{Value: "a", Name: "Balanced"},
				{Value: "b", Name: "Balanced"},
			},
		},
		{
			Group: "standard", Name: "Standard",
			Options: []acp.SessionConfigSelectOption{
				{Value: "c", Name: "Balanced"},
			},
		},
	}
	catalog := CatalogFromSelectOptions(acp.SessionConfigSelectOptions{Grouped: &grouped})
	if catalog == nil {
		t.Fatal("grouped options produced no catalog")
	}
	if catalog.Version != 1 || len(catalog.Nodes) != 2 {
		t.Fatalf("catalog = %#v", catalog)
	}
	if catalog.Nodes[0].ID != "fusion" || catalog.Nodes[1].ID != "standard" {
		t.Fatalf("group order = %#v", catalog.Nodes)
	}
	choices := modelcatalog.LeafChoices(catalog.Nodes)
	if len(choices) != 3 || choices[0].Value != "a" || choices[2].Value != "c" {
		t.Fatalf("leaves = %#v", choices)
	}
	leaf := catalog.Nodes[0].Children[1]
	if leaf.Kind != "choice" || leaf.Label != "Balanced" {
		t.Fatalf("repeated label node = %#v", leaf)
	}
}

func TestCatalogFromSelectOptionsFallsBackForUnsupportedShapes(t *testing.T) {
	flat := acp.SessionConfigSelectOptionsUngrouped{{Value: "a", Name: "A"}}
	if catalog := CatalogFromSelectOptions(acp.SessionConfigSelectOptions{Ungrouped: &flat}); catalog != nil {
		t.Fatalf("flat options produced a catalog: %#v", catalog)
	}
	empty := acp.SessionConfigSelectOptionsGrouped{}
	if catalog := CatalogFromSelectOptions(acp.SessionConfigSelectOptions{Grouped: &empty}); catalog != nil {
		t.Fatalf("empty groups produced a catalog: %#v", catalog)
	}
	noID := acp.SessionConfigSelectOptionsGrouped{{
		Name: "Missing ID", Options: []acp.SessionConfigSelectOption{{Value: "a", Name: "A"}},
	}}
	if catalog := CatalogFromSelectOptions(acp.SessionConfigSelectOptions{Grouped: &noID}); catalog != nil {
		t.Fatalf("group without an id produced a catalog: %#v", catalog)
	}
}

func TestCatalogFromRawOptionsParsesGroupedMeta(t *testing.T) {
	raw := []any{
		map[string]any{
			"group": "fusion", "name": "Fusion",
			"options": []any{map[string]any{"value": "a", "name": "Balanced"}},
		},
	}
	catalog := CatalogFromRawOptions(raw)
	if catalog == nil || len(catalog.Nodes) != 1 || catalog.Nodes[0].ID != "fusion" {
		t.Fatalf("raw catalog = %#v", catalog)
	}
	if catalog.Nodes[0].Children[0].Value != "a" {
		t.Fatalf("raw leaves = %#v", catalog.Nodes[0].Children)
	}
	if catalog := CatalogFromRawOptions([]any{map[string]any{"value": "a", "name": "A"}}); catalog != nil {
		t.Fatalf("flat raw options produced a catalog: %#v", catalog)
	}
	if catalog := CatalogFromRawOptions("not-a-list"); catalog != nil {
		t.Fatalf("non-list raw options produced a catalog: %#v", catalog)
	}
}

func TestClientCapabilityMetaAddsQualifiedProviderExtensions(t *testing.T) {
	base := map[string]any{"terminal_output": true}
	devin := ClientCapabilityMeta(DevinAgentID, base)
	if devin[DevinGroupedSessionConfigOptionsMetaKey] != true {
		t.Fatalf("Devin meta = %#v", devin)
	}
	if _, ok := devin[ParameterizedModelPickerMetaKey]; ok {
		t.Fatalf("Devin inherited the Cursor picker opt-in: %#v", devin)
	}
	if len(base) != 1 {
		t.Fatalf("caller map was mutated: %#v", base)
	}
	cursor := ClientCapabilityMeta(CursorAgentID, base)
	if cursor[ParameterizedModelPickerMetaKey] != true {
		t.Fatalf("Cursor meta = %#v", cursor)
	}
	if _, ok := cursor[DevinGroupedSessionConfigOptionsMetaKey]; ok {
		t.Fatalf("Cursor inherited the Devin grouped extension: %#v", cursor)
	}
	other := ClientCapabilityMeta("codex-acp", base)
	if len(other) != 1 || other["terminal_output"] != true {
		t.Fatalf("unrelated agent meta = %#v", other)
	}
}

func TestCatalogFromSelectOptionsRejectsOversizedGroupsBeforeConversion(t *testing.T) {
	options := make([]acp.SessionConfigSelectOption, modelcatalog.MaxNodes+1)
	for i := range options {
		options[i] = acp.SessionConfigSelectOption{Value: acp.SessionConfigValueId("v" + string(rune('a'+i%26))), Name: "V"}
	}
	grouped := acp.SessionConfigSelectOptionsGrouped{{
		Group: "huge", Name: "Huge", Options: options,
	}}
	if catalog := CatalogFromSelectOptions(acp.SessionConfigSelectOptions{Grouped: &grouped}); catalog != nil {
		t.Fatalf("oversized ACP groups produced a catalog with %d nodes", len(catalog.Nodes))
	}
}
