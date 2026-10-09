package toolschema

import "testing"

func TestCompileRequiresObjectRoot(t *testing.T) {
	if _, err := Compile("test", map[string]any{"type": "string"}); err == nil {
		t.Fatal("Compile() expected non-object root error")
	}
}

func TestCompileAcceptsObjectSchema(t *testing.T) {
	if _, err := Compile("test", map[string]any{
		"type":       "object",
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
	}); err != nil {
		t.Fatalf("Compile() unexpected error: %v", err)
	}
}

func TestCompileRejectsRootCombinators(t *testing.T) {
	for _, combinator := range []string{"oneOf", "allOf", "anyOf"} {
		t.Run(combinator, func(t *testing.T) {
			document := map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
				combinator: []any{
					map[string]any{"required": []any{"name"}},
				},
			}
			if _, err := Compile("test", document); err == nil {
				t.Fatalf("Compile() expected root %q rejection", combinator)
			}
		})
	}
}

func TestCompileAcceptsNestedCombinator(t *testing.T) {
	document := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"blocks": map[string]any{
				"type": "object",
				"oneOf": []any{
					map[string]any{"required": []any{"a"}},
					map[string]any{"required": []any{"b"}},
				},
			},
		},
	}
	if _, err := Compile("test", document); err != nil {
		t.Fatalf("Compile() rejected a nested combinator: %v", err)
	}
}
