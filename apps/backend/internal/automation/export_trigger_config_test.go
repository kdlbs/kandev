package automation

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// AC-39: a trigger's stored config is emitted as an empty mapping, with a
// warning naming which of three conditions occurred, when it is not valid
// UTF-8 (checked on the raw bytes, before decoding, and taking precedence
// over the other two), is not valid JSON, or is valid JSON that is not an
// object.

func TestBuildTriggerConfigNode_ValidObject(t *testing.T) {
	node, warning, err := buildTriggerConfigNode(json.RawMessage(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("buildTriggerConfigNode: %v", err)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 4 {
		t.Fatalf("node = %+v, want a 2-entry mapping", node)
	}
	// AC-8: keys sorted byte-wise ascending.
	if node.Content[0].Value != "a" || node.Content[2].Value != "b" {
		t.Errorf("keys not sorted: %q, %q", node.Content[0].Value, node.Content[2].Value)
	}
}

func TestBuildTriggerConfigNode_EmptyObject(t *testing.T) {
	node, warning, err := buildTriggerConfigNode(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("buildTriggerConfigNode: %v", err)
	}
	if warning != "" {
		t.Errorf("warning = %q, want none", warning)
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 0 {
		t.Errorf("node = %+v, want an empty mapping", node)
	}
}

func TestBuildTriggerConfigNode_InvalidUTF8(t *testing.T) {
	node, warning, err := buildTriggerConfigNode(json.RawMessage("{\"a\":\"b\xffc\"}"))
	if err != nil {
		t.Fatalf("buildTriggerConfigNode: %v", err)
	}
	if warning != "config is not valid UTF-8" {
		t.Errorf("warning = %q, want %q", warning, "config is not valid UTF-8")
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 0 {
		t.Errorf("node = %+v, want an empty mapping", node)
	}
}

func TestBuildTriggerConfigNode_InvalidJSON(t *testing.T) {
	node, warning, err := buildTriggerConfigNode(json.RawMessage(`{not json`))
	if err != nil {
		t.Fatalf("buildTriggerConfigNode: %v", err)
	}
	if warning != "config is not valid JSON" {
		t.Errorf("warning = %q, want %q", warning, "config is not valid JSON")
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 0 {
		t.Errorf("node = %+v, want an empty mapping", node)
	}
}

// AC-39: a valid JSON value followed by trailing bytes is not valid JSON as a
// whole. json.Decoder.Decode only consumes one JSON value and stops, so a
// naive decode-and-check-error implementation silently accepts trailing
// garbage. Assert the "config is not valid JSON" warning path is taken, not
// the value itself, since a decoder that behaves this way would otherwise
// export the leading value as if the stored bytes were entirely valid.
func TestBuildTriggerConfigNode_TrailingGarbageAfterValidJSON(t *testing.T) {
	node, warning, err := buildTriggerConfigNode(json.RawMessage(`{"cron_expression":"0 9 * * *"} trailing garbage`))
	if err != nil {
		t.Fatalf("buildTriggerConfigNode: %v", err)
	}
	if warning != "config is not valid JSON" {
		t.Errorf("warning = %q, want %q", warning, "config is not valid JSON")
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 0 {
		t.Errorf("node = %+v, want an empty mapping", node)
	}
}

func TestBuildTriggerConfigNode_ValidJSONNotAnObject(t *testing.T) {
	cases := map[string]string{
		"null":   `null`,
		"array":  `[1,2]`,
		"string": `"str"`,
		"number": `42`,
		"bool":   `true`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			node, warning, err := buildTriggerConfigNode(json.RawMessage(raw))
			if err != nil {
				t.Fatalf("buildTriggerConfigNode: %v", err)
			}
			if warning != "config is not a JSON object" {
				t.Errorf("warning = %q, want %q", warning, "config is not a JSON object")
			}
			if node.Kind != yaml.MappingNode || len(node.Content) != 0 {
				t.Errorf("node = %+v, want an empty mapping", node)
			}
		})
	}
}

func TestBuildTriggerConfigNode_InvalidUTF8TakesPrecedenceOverInvalidJSON(t *testing.T) {
	// Invalid UTF-8 bytes that also fail to parse as JSON: exactly one
	// warning, and it must be the UTF-8 one, so precedence is unambiguous.
	node, warning, err := buildTriggerConfigNode(json.RawMessage("not json \xff"))
	if err != nil {
		t.Fatalf("buildTriggerConfigNode: %v", err)
	}
	if warning != "config is not valid UTF-8" {
		t.Errorf("warning = %q, want %q", warning, "config is not valid UTF-8")
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 0 {
		t.Errorf("node = %+v, want an empty mapping", node)
	}
}

// AC-10: two stored configs that differ only in JSON whitespace must produce
// identical config YAML. The live store contains both a compact and a
// space-separated serialization of the same scheduled trigger config.
func TestBuildTriggerConfigNode_AC10_WhitespaceInsensitive(t *testing.T) {
	compact := json.RawMessage(`{"cron_expression":"0 9 * * *","timezone":"Asia/Singapore"}`)
	spaced := json.RawMessage(`{"cron_expression": "0 9 * * *", "timezone": "Asia/Singapore"}`)

	compactNode, warning, err := buildTriggerConfigNode(compact)
	if err != nil {
		t.Fatalf("buildTriggerConfigNode(compact): %v", err)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want none", warning)
	}
	spacedNode, warning, err := buildTriggerConfigNode(spaced)
	if err != nil {
		t.Fatalf("buildTriggerConfigNode(spaced): %v", err)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want none", warning)
	}

	compactOut, err := yaml.Marshal(compactNode)
	if err != nil {
		t.Fatalf("yaml.Marshal(compactNode): %v", err)
	}
	spacedOut, err := yaml.Marshal(spacedNode)
	if err != nil {
		t.Fatalf("yaml.Marshal(spacedNode): %v", err)
	}
	if string(compactOut) != string(spacedOut) {
		t.Errorf("config YAML differs by input whitespace alone:\ncompact: %q\nspaced:  %q", compactOut, spacedOut)
	}
}

func TestNewTriggerConfigWarning_EscapesTypeAndScopesDedupToTrigger(t *testing.T) {
	a := &Automation{ID: "auto-1", Name: "Daily Sync"}
	trigger := AutomationTrigger{ID: "trig-1", Type: TriggerType("weird\ntype")}

	w := newTriggerConfigWarning(a, trigger, "config is not valid JSON")

	if w.AutomationName != "Daily Sync" || w.AutomationID != "auto-1" {
		t.Errorf("automation identity = %+v", w)
	}
	if w.DedupKey != "trig-1" {
		t.Errorf("DedupKey = %q, want trigger ID", w.DedupKey)
	}
	want := `trigger weird\ntype: config is not valid JSON`
	if w.Message != want {
		t.Errorf("Message = %q, want %q", w.Message, want)
	}
}
