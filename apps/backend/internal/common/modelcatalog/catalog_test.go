package modelcatalog

import (
	"errors"
	"strconv"
	"testing"
)

func testCatalog() *Catalog {
	return &Catalog{
		Version: SchemaVersion,
		Nodes: []Node{
			{
				Kind: NodeKindGroup, ID: "fusion", Label: "Fusion",
				Description: "Composed model families",
				Children: []Node{
					{Kind: NodeKindChoice, Value: "fusion-lead-a__sidekick-a", Label: "Fusion pairing", Description: "Exact advertised pairing"},
				},
			},
			{
				Kind: NodeKindGroup, ID: "standard", Label: "Standard",
				Children: []Node{
					{Kind: NodeKindChoice, Value: "plain", Label: "Fusion"},
				},
			},
		},
		Compositions: []Composition{{
			ID: "fusion", GroupID: "fusion", Label: "Fusion",
			Dimensions: []Dimension{
				{ID: "lead", Label: "Lead", Choices: []Choose{
					{Value: "lead-a", Label: "Lead A"},
					{Value: "lead-b", Label: "Lead B"},
				}},
				{ID: "sidekick", Label: "Sidekick", Choices: []Choose{
					{Value: "sidekick-a", Label: "Sidekick A"},
				}},
			},
			Bindings: []Binding{
				{Values: map[string]string{"lead": "lead-a", "sidekick": "sidekick-a"}, Model: "fusion-lead-a__sidekick-a"},
			},
		}},
	}
}

func TestValidateAcceptsBoundedCatalog(t *testing.T) {
	if err := Validate(testCatalog()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestLeafChoicesPreserveOrderAndRepeatedLabels(t *testing.T) {
	choices := LeafChoices(testCatalog().Nodes)
	if len(choices) != 2 {
		t.Fatalf("choices = %#v, want two leaves", choices)
	}
	if choices[0].Value != "fusion-lead-a__sidekick-a" || choices[1].Value != "plain" {
		t.Fatalf("choices = %#v, want ordered exact values", choices)
	}
	if choices[0].Description != "Exact advertised pairing" {
		t.Fatalf("description = %q", choices[0].Description)
	}
}

func TestCompositionResolveRequiresOneExactBinding(t *testing.T) {
	composition := &testCatalog().Compositions[0]
	model, ok := composition.Resolve(map[string]string{"lead": "lead-a", "sidekick": "sidekick-a"})
	if !ok || model != "fusion-lead-a__sidekick-a" {
		t.Fatalf("Resolve = %q/%t, want the exact advertised pairing", model, ok)
	}
	cases := []struct {
		name   string
		values map[string]string
	}{
		{name: "incomplete", values: map[string]string{"lead": "lead-a"}},
		{name: "unknown dimension", values: map[string]string{"lead": "lead-a", "sidekick": "sidekick-a", "extra": "x"}},
		{name: "unknown value", values: map[string]string{"lead": "lead-b", "sidekick": "sidekick-a"}},
		{name: "unknown choice", values: map[string]string{"lead": "lead-a", "sidekick": "sidekick-z"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if model, ok := composition.Resolve(tt.values); ok {
				t.Fatalf("Resolve = %q, want no resolution for %#v", model, tt.values)
			}
		})
	}
}

func TestCompositionResolveRejectsAmbiguousBindings(t *testing.T) {
	composition := &Composition{
		ID: "ambiguous", GroupID: "g",
		Dimensions: []Dimension{{ID: "lead", Choices: []Choose{{Value: "a", Label: "A"}}}},
		Bindings: []Binding{
			{Values: map[string]string{"lead": "a"}, Model: "m1"},
			{Values: map[string]string{"lead": "a"}, Model: "m2"},
		},
	}
	if model, ok := composition.Resolve(map[string]string{"lead": "a"}); ok {
		t.Fatalf("Resolve = %q, want ambiguity to stay unresolved", model)
	}
}

func TestValidateRejectsUnboundedOrInconsistentData(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Catalog)
	}{
		{name: "wrong version", mutate: func(c *Catalog) { c.Version = 2 }},
		{name: "no nodes", mutate: func(c *Catalog) { c.Nodes = nil }},
		{name: "unknown kind", mutate: func(c *Catalog) { c.Nodes[0].Kind = "widget" }},
		{name: "group without id", mutate: func(c *Catalog) { c.Nodes[0].ID = "" }},
		{name: "duplicate group id", mutate: func(c *Catalog) { c.Nodes[1].ID = "fusion" }},
		{name: "group without children", mutate: func(c *Catalog) {
			c.Nodes = c.Nodes[:1]
			c.Nodes[0].Children = nil
		}},
		{name: "choice with children", mutate: func(c *Catalog) {
			c.Nodes[0].Children[0].Children = []Node{{Kind: NodeKindChoice, Value: "nested", Label: "Nested"}}
		}},
		{name: "empty choice value", mutate: func(c *Catalog) { c.Nodes[1].Children[0].Value = "" }},
		{name: "composition references unknown group", mutate: func(c *Catalog) { c.Compositions[0].GroupID = "missing" }},
		{name: "composition without dimensions", mutate: func(c *Catalog) { c.Compositions[0].Dimensions = nil }},
		{name: "composition without bindings", mutate: func(c *Catalog) { c.Compositions[0].Bindings = nil }},
		{name: "binding names an unknown model", mutate: func(c *Catalog) { c.Compositions[0].Bindings[0].Model = "synthetic" }},
		{name: "binding misses a dimension", mutate: func(c *Catalog) {
			c.Compositions[0].Bindings[0].Values = map[string]string{"lead": "lead-a"}
		}},
		{name: "binding names an unknown dimension value", mutate: func(c *Catalog) {
			c.Compositions[0].Bindings[0].Values = map[string]string{"lead": "lead-c", "sidekick": "sidekick-a"}
		}},
		{name: "duplicate binding tuple", mutate: func(c *Catalog) {
			c.Compositions[0].Bindings = append(c.Compositions[0].Bindings, c.Compositions[0].Bindings[0])
		}},
		{name: "duplicate composition id", mutate: func(c *Catalog) {
			c.Compositions = append(c.Compositions, c.Compositions[0])
		}},
		{name: "oversized label", mutate: func(c *Catalog) {
			c.Nodes[1].Children[0].Label = string(make([]byte, MaxStringLength+1))
		}},
		{name: "too many nodes", mutate: func(c *Catalog) {
			children := make([]Node, 0, MaxNodes+1)
			for i := 0; i <= MaxNodes; i++ {
				children = append(children, Node{Kind: NodeKindChoice, Value: "v" + strconv.Itoa(i), Label: "V"})
			}
			c.Nodes = []Node{{Kind: NodeKindGroup, ID: "big", Label: "Big", Children: children}}
			c.Compositions = nil
		}},
		{name: "too deep", mutate: func(c *Catalog) {
			node := Node{Kind: NodeKindChoice, Value: "leaf", Label: "Leaf"}
			for i := 0; i < MaxDepth+1; i++ {
				node = Node{Kind: NodeKindGroup, ID: "g" + strconv.Itoa(i), Label: "G", Children: []Node{node}}
			}
			c.Nodes = []Node{node}
			c.Compositions = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := testCatalog()
			tt.mutate(catalog)
			err := Validate(catalog)
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("Validate error = %v, want ErrInvalidCatalog", err)
			}
		})
	}
}

func TestValidateBoundsCompositionsAndLabels(t *testing.T) {
	tooMany := testCatalog()
	tooMany.Compositions = make([]Composition, MaxCompositions+1)
	for i := range tooMany.Compositions {
		tooMany.Compositions[i] = testCatalog().Compositions[0]
		tooMany.Compositions[i].ID = "composition-" + strconv.Itoa(i)
	}
	if err := Validate(tooMany); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("oversized composition list error = %v", err)
	}

	oversizedLabel := testCatalog()
	oversizedLabel.Compositions[0].Label = string(make([]byte, MaxStringLength+1))
	if err := Validate(oversizedLabel); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("oversized composition label error = %v", err)
	}
}
