package acpcompat

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/common/modelcatalog"
)

func devinChoiceNode(value, label string) modelcatalog.Node {
	return modelcatalog.Node{Kind: modelcatalog.NodeKindChoice, Value: value, Label: label}
}

func devinGroupNode(id, label string, children ...modelcatalog.Node) modelcatalog.Node {
	return modelcatalog.Node{Kind: modelcatalog.NodeKindGroup, ID: id, Label: label, Children: children}
}

func devinTestCatalog(nodes ...modelcatalog.Node) *modelcatalog.Catalog {
	return &modelcatalog.Catalog{Version: modelcatalog.SchemaVersion, Nodes: nodes}
}

// devinTestNative mirrors the qualified native relationships: a Fusion record
// names a Lead variant and a Sidekick variant through its exact identifier and
// label. It is synthetic evidence shaped by the design contract; the captured
// fixture gate in Task 02 remains open.
func devinTestNative() *DevinNativeCatalog {
	return &DevinNativeCatalog{Families: []DevinNativeFamily{
		{
			UID:   "gpt-family",
			Label: "GPT-6.1",
			Variants: []DevinNativeVariant{
				{UID: "gpt-sol", Label: "Sol"},
				{UID: "gpt-sol-high", Label: "Sol High"},
			},
		},
		{
			UID:   "luna-family",
			Label: "GPT-6 Luna",
			Variants: []DevinNativeVariant{
				{UID: "luna-high", Label: "Luna High"},
				{UID: "luna-low", Label: "Luna Low"},
			},
		},
		{
			UID:   "fusion-family",
			Label: "Fusion",
			Variants: []DevinNativeVariant{
				{UID: "fusion-gpt-sol-sidekick-luna-high", Label: DevinFusionLabel("Sol", "Luna High")},
				{UID: "fusion-gpt-sol-sidekick-luna-low", Label: DevinFusionLabel("Sol", "Luna Low")},
			},
		},
	}}
}

func devinFusionGroup() modelcatalog.Node {
	return devinGroupNode("fusion", "Fusion",
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-high", DevinFusionLabel("Sol", "Luna High")),
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-low", DevinFusionLabel("Sol", "Luna Low")),
		devinChoiceNode("unknown-pairing", "Unknown pairing"),
	)
}

func TestDevinNativeCatalogParsing(t *testing.T) {
	valid := `{"families":[{"family_uid":"fam","family_label":"Family","variants":[{"model_uid":"model","label":"Model"}]}]}`
	native, err := ParseDevinNativeCatalog([]byte(valid))
	if err != nil {
		t.Fatalf("parse valid catalog: %v", err)
	}
	if len(native.Families) != 1 || len(native.Families[0].Variants) != 1 {
		t.Fatalf("unexpected parsed shape: %+v", native)
	}

	cases := map[string]string{
		"malformed":        `{"families":[`,
		"trailing":         valid + `{}`,
		"no families":      `{"families":[]}`,
		"missing family":   `{"families":[{"family_label":"Family","variants":[{"model_uid":"m","label":"M"}]}]}`,
		"missing variant":  `{"families":[{"family_uid":"fam","family_label":"Family","variants":[{"label":"M"}]}]}`,
		"duplicate family": `{"families":[{"family_uid":"fam","family_label":"A","variants":[{"model_uid":"a","label":"A"}]},{"family_uid":"fam","family_label":"B","variants":[{"model_uid":"b","label":"B"}]}]}`,
		"duplicate variant": `{"families":[{"family_uid":"fam","family_label":"A","variants":[{"model_uid":"a","label":"A"}]},` +
			`{"family_uid":"fam2","family_label":"B","variants":[{"model_uid":"a","label":"A"}]}]}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDevinNativeCatalog([]byte(payload)); !errors.Is(err, ErrInvalidDevinCatalog) {
				t.Fatalf("expected ErrInvalidDevinCatalog, got %v", err)
			}
		})
	}

	if _, err := ParseDevinNativeCatalog(nil); !errors.Is(err, ErrInvalidDevinCatalog) {
		t.Fatalf("empty payload should be rejected, got %v", err)
	}
	oversized := append([]byte(valid), make([]byte, MaxDevinCatalogBytes)...)
	if _, err := ParseDevinNativeCatalog(oversized); !errors.Is(err, ErrInvalidDevinCatalog) {
		t.Fatalf("oversized payload should be rejected, got %v", err)
	}
}

func TestDevinCompositionBindings(t *testing.T) {
	catalog := devinTestCatalog(devinFusionGroup(), devinGroupNode("standard", "Standard", devinChoiceNode("solo", "Solo")))
	enriched := EnrichDevinCatalog(catalog, devinTestNative())
	if enriched == catalog {
		t.Fatal("expected an enriched catalog")
	}
	if err := modelcatalog.Validate(enriched); err != nil {
		t.Fatalf("enriched catalog does not validate: %v", err)
	}
	if len(enriched.Compositions) != 1 {
		t.Fatalf("expected one composition, got %d", len(enriched.Compositions))
	}
	composition := enriched.Compositions[0]
	if composition.GroupID != "fusion" || composition.ID != "fusion" || composition.Label != "Fusion" {
		t.Fatalf("unexpected composition identity: %+v", composition)
	}
	if len(composition.Dimensions) != 2 {
		t.Fatalf("expected lead and sidekick dimensions, got %+v", composition.Dimensions)
	}
	lead, sidekick := composition.Dimensions[0], composition.Dimensions[1]
	if lead.ID != DevinLeadDimensionID || lead.Label != "Lead" {
		t.Fatalf("unexpected lead dimension: %+v", lead)
	}
	// Lead uses the native family label, not the variant label and not a
	// suffix-stripped variant name.
	if len(lead.Choices) != 1 || lead.Choices[0].Value != "gpt-family" || lead.Choices[0].Label != "GPT-6.1" {
		t.Fatalf("unexpected lead choices: %+v", lead.Choices)
	}
	if sidekick.ID != DevinSidekickDimensionID {
		t.Fatalf("unexpected sidekick dimension: %+v", sidekick)
	}
	if got := []string{sidekick.Choices[0].Value, sidekick.Choices[1].Value}; !reflect.DeepEqual(got, []string{"luna-high", "luna-low"}) {
		t.Fatalf("sidekick choices must preserve source order: %+v", sidekick.Choices)
	}
	if len(composition.Bindings) != 2 {
		t.Fatalf("expected two bindings, got %+v", composition.Bindings)
	}
	first := composition.Bindings[0]
	if first.Model != "fusion-gpt-sol-sidekick-luna-high" {
		t.Fatalf("unexpected binding model: %+v", first)
	}
	if first.Values[DevinLeadDimensionID] != "gpt-family" || first.Values[DevinSidekickDimensionID] != "luna-high" {
		t.Fatalf("unexpected binding values: %+v", first.Values)
	}
	// The unknown pairing stays an ordinary leaf and the flat projection is
	// unchanged.
	if got, want := modelcatalog.LeafChoices(enriched.Nodes), modelcatalog.LeafChoices(catalog.Nodes); !reflect.DeepEqual(got, want) {
		t.Fatalf("flat projection changed: %+v != %+v", got, want)
	}
	if !enriched.ChoiceValue("unknown-pairing") {
		t.Fatal("unknown pairing should remain advertised")
	}
}

func TestDevinCompositionIntersectsCurrentCatalog(t *testing.T) {
	catalog := devinTestCatalog(devinGroupNode("fusion", "Fusion",
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-high", DevinFusionLabel("Sol", "Luna High")),
		devinChoiceNode("fusion-withdrawn-sidekick-luna-low", DevinFusionLabel("Withdrawn", "Luna Low")),
	))
	enriched := EnrichDevinCatalog(catalog, devinTestNative())
	if len(enriched.Compositions) != 1 {
		t.Fatalf("expected one composition, got %d", len(enriched.Compositions))
	}
	composition := enriched.Compositions[0]
	if len(composition.Bindings) != 1 {
		t.Fatalf("native-only and withdrawn leaves must not be bound: %+v", composition.Bindings)
	}
	if len(composition.Dimensions[1].Choices) != 1 {
		t.Fatalf("dimension choices must follow the current catalog: %+v", composition.Dimensions[1].Choices)
	}
}

func TestDevinCompositionTupleCollisions(t *testing.T) {
	native := &DevinNativeCatalog{Families: []DevinNativeFamily{
		{
			UID:   "gpt-family",
			Label: "GPT-6.1",
			Variants: []DevinNativeVariant{
				{UID: "gpt-sol", Label: "Sol"},
				{UID: "gpt-sol-high", Label: "Sol High"},
			},
		},
		{UID: "luna-family", Label: "GPT-6 Luna", Variants: []DevinNativeVariant{
			{UID: "luna-high", Label: "Luna High"},
			{UID: "luna-low", Label: "Luna Low"},
		}},
		{UID: "fusion-family", Label: "Fusion", Variants: []DevinNativeVariant{
			// Both variants belong to the same family and share one Sidekick,
			// so their (lead family, sidekick variant) tuple collides.
			{UID: "fusion-gpt-sol-sidekick-luna-high", Label: DevinFusionLabel("Sol", "Luna High")},
			{UID: "fusion-gpt-sol-high-sidekick-luna-high", Label: DevinFusionLabel("Sol High", "Luna High")},
			{UID: "fusion-gpt-sol-sidekick-luna-low", Label: DevinFusionLabel("Sol", "Luna Low")},
		}},
	}}
	catalog := devinTestCatalog(devinGroupNode("fusion", "Fusion",
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-high", DevinFusionLabel("Sol", "Luna High")),
		devinChoiceNode("fusion-gpt-sol-high-sidekick-luna-high", DevinFusionLabel("Sol High", "Luna High")),
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-low", DevinFusionLabel("Sol", "Luna Low")),
	))
	enriched := EnrichDevinCatalog(catalog, native)
	if len(enriched.Compositions) != 1 {
		t.Fatalf("expected the non-colliding pair to keep a composition, got %d", len(enriched.Compositions))
	}
	bindings := enriched.Compositions[0].Bindings
	if len(bindings) != 1 || bindings[0].Model != "fusion-gpt-sol-sidekick-luna-low" {
		t.Fatalf("colliding tuples must stay unbound: %+v", bindings)
	}
	if len(enriched.Compositions[0].Dimensions[0].Choices) != 1 {
		t.Fatalf("colliding lead families must not appear: %+v", enriched.Compositions[0].Dimensions[0].Choices)
	}
}

func TestDevinCompositionAmbiguousPair(t *testing.T) {
	native := &DevinNativeCatalog{Families: []DevinNativeFamily{
		{UID: "lead-a", Label: "A", Variants: []DevinNativeVariant{{UID: "x", Label: "A"}}},
		{UID: "lead-b", Label: "A", Variants: []DevinNativeVariant{{UID: "x-sidekick-y", Label: "A"}}},
		{UID: "sidekick-a", Label: "BZ", Variants: []DevinNativeVariant{{UID: "z", Label: "BZ"}}},
		{UID: "sidekick-b", Label: "BZ", Variants: []DevinNativeVariant{{UID: "y-sidekick-z", Label: "BZ"}}},
		{UID: "fusion-family", Label: "Fusion", Variants: []DevinNativeVariant{
			{UID: "fusion-x-sidekick-y-sidekick-z", Label: DevinFusionLabel("A", "BZ")},
		}},
	}}
	catalog := devinTestCatalog(devinGroupNode("fusion", "Fusion",
		devinChoiceNode("fusion-x-sidekick-y-sidekick-z", DevinFusionLabel("A", "BZ")),
	))
	if enriched := EnrichDevinCatalog(catalog, native); enriched != catalog {
		t.Fatalf("an ambiguous pair must not produce a composition: %+v", enriched.Compositions)
	}
}

func TestDevinCompositionRejectsUnexplainedLeaves(t *testing.T) {
	cases := map[string]modelcatalog.Node{
		"label mismatch": devinGroupNode("fusion", "Fusion",
			devinChoiceNode("fusion-gpt-sol-sidekick-luna-high", "Fusion (Sol + Luna Ultra)"),
		),
		"not fusion shaped": devinGroupNode("fusion", "Fusion",
			devinChoiceNode("gpt-sol", "Sol"),
		),
		"unknown native record": devinGroupNode("fusion", "Fusion",
			devinChoiceNode("fusion-unknown-sidekick-unknown", DevinFusionLabel("Unknown", "Unknown")),
		),
	}
	for name, group := range cases {
		t.Run(name, func(t *testing.T) {
			catalog := devinTestCatalog(group)
			if enriched := EnrichDevinCatalog(catalog, devinTestNative()); enriched != catalog {
				t.Fatalf("expected the ordinary catalog, got compositions %+v", enriched.Compositions)
			}
		})
	}
	unchanged := devinTestCatalog(devinFusionGroup())
	if enriched := EnrichDevinCatalog(unchanged, nil); enriched != unchanged {
		t.Fatal("nil native evidence must keep the catalog")
	}
}

func TestDevinEnrichmentDoesNotMutateOriginal(t *testing.T) {
	catalog := devinTestCatalog(devinFusionGroup())
	before, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("snapshot catalog: %v", err)
	}
	if EnrichDevinCatalog(catalog, devinTestNative()) == catalog {
		t.Fatal("expected an enriched copy")
	}
	after, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("snapshot catalog: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("enrichment mutated the original catalog: %s", after)
	}
}

func TestDevinCompositionSkipsExistingComposition(t *testing.T) {
	catalog := devinTestCatalog(devinFusionGroup())
	existing := modelcatalog.Composition{
		ID:      "existing",
		GroupID: "fusion",
		Dimensions: []modelcatalog.Dimension{
			{ID: "family", Label: "Family", Choices: []modelcatalog.Choose{{Value: "one", Label: "One"}}},
		},
		Bindings: []modelcatalog.Binding{
			{Values: map[string]string{"family": "one"}, Model: "fusion-gpt-sol-sidekick-luna-high"},
		},
	}
	catalog.Compositions = []modelcatalog.Composition{existing}
	enriched := EnrichDevinCatalog(catalog, devinTestNative())
	if len(enriched.Compositions) != 1 || enriched.Compositions[0].ID != "existing" {
		t.Fatalf("an existing composition must be preserved: %+v", enriched.Compositions)
	}
}

func TestDevinFusionLabelExpression(t *testing.T) {
	if got := DevinFusionLabel("Sol", "Luna High"); got != "Fusion (Sol + Luna High)" {
		t.Fatalf("unexpected native label expression: %q", got)
	}
}

func TestDevinCompositionKeepsExactValuesAndSourceOrder(t *testing.T) {
	native := &DevinNativeCatalog{Families: []DevinNativeFamily{
		{UID: "gpt-family", Label: "GPT-6.1", Variants: []DevinNativeVariant{
			{UID: "gpt-sol", Label: "Sol"},
			{UID: "gpt-sol-high", Label: "Sol High"},
		}},
		{UID: "claude-family", Label: "Claude Opus 5.5", Variants: []DevinNativeVariant{
			{UID: "opus-medium", Label: "Opus Medium"},
		}},
		{UID: "luna-family", Label: "GPT-6 Luna", Variants: []DevinNativeVariant{
			{UID: "luna-high", Label: "Luna High"},
			{UID: "luna-low", Label: "Luna Low"},
		}},
		{UID: "fusion-family", Label: "Fusion", Variants: []DevinNativeVariant{
			{UID: "fusion-gpt-sol-high-sidekick-luna-low", Label: DevinFusionLabel("Sol High", "Luna Low")},
			{UID: "fusion-opus-medium-sidekick-luna-high", Label: DevinFusionLabel("Opus Medium", "Luna High")},
			{UID: "fusion-gpt-sol-sidekick-luna-high", Label: DevinFusionLabel("Sol", "Luna High")},
		}},
	}}
	catalog := devinTestCatalog(devinGroupNode("fusion", "Fusion",
		devinChoiceNode("fusion-gpt-sol-high-sidekick-luna-low", DevinFusionLabel("Sol High", "Luna Low")),
		devinChoiceNode("fusion-opus-medium-sidekick-luna-high", DevinFusionLabel("Opus Medium", "Luna High")),
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-high", DevinFusionLabel("Sol", "Luna High")),
	))
	enriched := EnrichDevinCatalog(catalog, native)
	if len(enriched.Compositions) != 1 {
		t.Fatalf("expected one composition, got %d", len(enriched.Compositions))
	}
	composition := enriched.Compositions[0]
	if len(composition.Dimensions) != 2 {
		t.Fatalf("an effort-like suffix must not add a dimension: %+v", composition.Dimensions)
	}
	leads := composition.Dimensions[0].Choices
	wantLeads := []modelcatalog.Choose{
		{Value: "gpt-family", Label: "GPT-6.1"},
		{Value: "claude-family", Label: "Claude Opus 5.5"},
	}
	if !reflect.DeepEqual(leads, wantLeads) {
		t.Fatalf("lead choices must follow source order: %+v", leads)
	}
	sidekicks := composition.Dimensions[1].Choices
	wantSidekicks := []modelcatalog.Choose{
		{Value: "luna-low", Label: "Luna Low"},
		{Value: "luna-high", Label: "Luna High"},
	}
	if !reflect.DeepEqual(sidekicks, wantSidekicks) {
		t.Fatalf("sidekick choices must follow source order: %+v", sidekicks)
	}
	if got := composition.Bindings[0].Model; got != "fusion-gpt-sol-high-sidekick-luna-low" {
		t.Fatalf("the exact advertised value must survive an effort-like suffix: %q", got)
	}
}

func TestDevinCompositionRejectsInvalidNativeEvidence(t *testing.T) {
	cases := map[string]func(*DevinNativeCatalog){
		"duplicate family": func(native *DevinNativeCatalog) {
			native.Families = append(native.Families, DevinNativeFamily{
				UID:      "gpt-family",
				Label:    "GPT-6.1",
				Variants: []DevinNativeVariant{{UID: "gpt-other", Label: "Other"}},
			})
		},
		"duplicate variant": func(native *DevinNativeCatalog) {
			native.Families = append(native.Families, DevinNativeFamily{
				UID:      "duplicate-family",
				Label:    "Duplicate",
				Variants: []DevinNativeVariant{{UID: "gpt-sol", Label: "Sol"}},
			})
		},
		"empty families": func(native *DevinNativeCatalog) {
			native.Families = nil
		},
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			native := devinTestNative()
			corrupt(native)
			catalog := devinTestCatalog(devinFusionGroup())
			if enriched := EnrichDevinCatalog(catalog, native); enriched != catalog {
				t.Fatalf("invalid native evidence must keep the ordinary catalog: %+v", enriched.Compositions)
			}
		})
	}
}

func TestDevinCompositionLeavesRepeatedValuesUnbound(t *testing.T) {
	catalog := devinTestCatalog(devinGroupNode("fusion", "Fusion",
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-high", DevinFusionLabel("Sol", "Luna High")),
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-high", DevinFusionLabel("Sol", "Luna High")),
		devinChoiceNode("fusion-gpt-sol-sidekick-luna-low", DevinFusionLabel("Sol", "Luna Low")),
	))
	enriched := EnrichDevinCatalog(catalog, devinTestNative())
	if len(enriched.Compositions) != 1 {
		t.Fatalf("expected one composition, got %d", len(enriched.Compositions))
	}
	bindings := enriched.Compositions[0].Bindings
	if len(bindings) != 1 || bindings[0].Model != "fusion-gpt-sol-sidekick-luna-low" {
		t.Fatalf("a repeated advertised value must stay unbound: %+v", bindings)
	}
	if got, want := modelcatalog.LeafChoices(enriched.Nodes), modelcatalog.LeafChoices(catalog.Nodes); !reflect.DeepEqual(got, want) {
		t.Fatalf("repeated leaves must remain advertised: %+v != %+v", got, want)
	}
}
