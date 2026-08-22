package automation

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// decodeJSONNode is a test helper: decode raw JSON with UseNumber, build the YAML
// node via jsonToYAMLNode, marshal it, and return the marshaled bytes for assertion.
func marshalJSONAsYAMLNode(t *testing.T, rawJSON string) string {
	t.Helper()
	v, err := decodeJSONWithNumbers([]byte(rawJSON))
	if err != nil {
		t.Fatalf("decodeJSONWithNumbers(%q): %v", rawJSON, err)
	}
	node, err := jsonToYAMLNode(v)
	if err != nil {
		t.Fatalf("jsonToYAMLNode(%v): %v", v, err)
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		t.Fatalf("yaml.Marshal(node): %v", err)
	}
	return string(out)
}

// AC-8: object keys are sorted byte-wise ascending, not by yaml.v3's own digit-aware
// map sorter (which would order v1, v2, v10 — wrong for a byte-wise contract).
func TestJSONToYAMLNode_ByteWiseKeyOrder(t *testing.T) {
	got := marshalJSONAsYAMLNode(t, `{"v2": 1, "v10": 2, "v1": 3}`)
	want := "v1: 3\nv10: 2\nv2: 1\n"
	if got != want {
		t.Errorf("byte-wise key order mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

// AC-8: nested objects are sorted at every depth, not just the top level.
func TestJSONToYAMLNode_SortedAtEveryDepth(t *testing.T) {
	got := marshalJSONAsYAMLNode(t, `{"outer": {"z": 1, "a": 2}}`)
	want := "outer:\n    a: 2\n    z: 1\n"
	if got != want {
		t.Errorf("nested sort mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

// AC-8: arrays preserve stored order and are never sorted.
func TestJSONToYAMLNode_ArrayOrderPreserved(t *testing.T) {
	got := marshalJSONAsYAMLNode(t, `["z", "a", "m"]`)
	want := "- z\n- a\n- m\n"
	if got != want {
		t.Errorf("array order mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

// AC-41: numbers are emitted unquoted, untagged, and character-identical to the
// stored lexeme — including values outside int64/uint64/float64 range, which must
// never be rounded, reformatted, or coerced through a Go numeric type.
func TestJSONToYAMLNode_NumberFidelity(t *testing.T) {
	cases := []string{
		"0",
		"12",
		"-7",
		"1.50",
		"0.0",
		"18446744073709551616", // overflows both int64 and uint64
		"1e400",                // overflows float64
		"-0",
	}
	for _, lexeme := range cases {
		t.Run(lexeme, func(t *testing.T) {
			v, err := decodeJSONWithNumbers([]byte(lexeme))
			if err != nil {
				t.Fatalf("decodeJSONWithNumbers(%q): %v", lexeme, err)
			}
			node, err := jsonToYAMLNode(v)
			if err != nil {
				t.Fatalf("jsonToYAMLNode(%v): %v", v, err)
			}
			if node.Tag != "" {
				t.Errorf("number node Tag = %q, want unset (empty)", node.Tag)
			}
			if node.Value != lexeme {
				t.Errorf("number node Value = %q, want %q (lexeme preserved)", node.Value, lexeme)
			}
		})
	}
}

// AC-8/AC-23: a string whose text would resolve to another YAML type must carry an
// explicit !!str tag so it re-parses as a string, never retyped to bool/null/int/float.
func TestJSONToYAMLNode_AmbiguousStringsStayStrings(t *testing.T) {
	cases := []string{"true", "false", "null", "1.0", "0755", "12", "~", "yes", "no"}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			v, err := decodeJSONWithNumbers([]byte(`"` + s + `"`))
			if err != nil {
				t.Fatalf("decodeJSONWithNumbers: %v", err)
			}
			node, err := jsonToYAMLNode(v)
			if err != nil {
				t.Fatalf("jsonToYAMLNode: %v", err)
			}
			out, err := yaml.Marshal(node)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			var roundTrip yaml.Node
			if err := yaml.Unmarshal(out, &roundTrip); err != nil {
				t.Fatalf("yaml.Unmarshal(%q): %v", out, err)
			}
			// Document node wraps the actual scalar.
			scalar := &roundTrip
			if roundTrip.Kind == yaml.DocumentNode {
				scalar = roundTrip.Content[0]
			}
			if scalar.Tag != "!!str" {
				t.Errorf("round-tripped tag = %q, want !!str (emitted: %q)", scalar.Tag, out)
			}
			if scalar.Value != s {
				t.Errorf("round-tripped value = %q, want %q", scalar.Value, s)
			}
		})
	}
}

// AC-8: bool and null get their standard explicit tags.
func TestJSONToYAMLNode_BoolAndNull(t *testing.T) {
	boolNode, err := jsonToYAMLNode(true)
	if err != nil {
		t.Fatalf("jsonToYAMLNode(true): %v", err)
	}
	if boolNode.Tag != "!!bool" || boolNode.Value != "true" {
		t.Errorf("bool node = {Tag:%q Value:%q}, want {Tag:!!bool Value:true}", boolNode.Tag, boolNode.Value)
	}

	nullNode, err := jsonToYAMLNode(nil)
	if err != nil {
		t.Fatalf("jsonToYAMLNode(nil): %v", err)
	}
	if nullNode.Tag != "!!null" || nullNode.Value != "null" {
		t.Errorf("null node = {Tag:%q Value:%q}, want {Tag:!!null Value:null}", nullNode.Tag, nullNode.Value)
	}
}
