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
