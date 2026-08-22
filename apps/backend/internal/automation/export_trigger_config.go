package automation

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// AC-39's three warning messages, checked and emitted in this exact order:
// UTF-8 validity is checked on the raw stored bytes before any decoding, so
// it takes precedence over a JSON error a subsequent decode might also
// report for the same bytes; JSON validity is checked before the
// decoded-value's shape.
const (
	warnTriggerConfigInvalidUTF8 = "config is not valid UTF-8"
	warnTriggerConfigInvalidJSON = "config is not valid JSON"
	warnTriggerConfigNotObject   = "config is not a JSON object"
)

// buildTriggerConfigNode renders a trigger's stored config as a YAML mapping
// node (AC-39). A config that is not valid UTF-8, not valid JSON, or valid
// JSON that does not decode to an object is rendered as an empty mapping
// instead of any partial or corrupted value — a partially-mangled config in
// a committed artifact is worse than an absent one — paired with exactly one
// warning naming the first-failing condition. Export never fails because of
// a bad trigger config.
func buildTriggerConfigNode(raw json.RawMessage) (*yaml.Node, string, error) {
	emptyMapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}

	if !utf8.Valid(raw) {
		return emptyMapping, warnTriggerConfigInvalidUTF8, nil
	}

	decoded, err := decodeJSONWithNumbers(raw)
	if err != nil {
		return emptyMapping, warnTriggerConfigInvalidJSON, nil
	}

	obj, ok := decoded.(map[string]any)
	if !ok {
		return emptyMapping, warnTriggerConfigNotObject, nil
	}

	node, err := jsonObjectToYAMLNode(obj)
	if err != nil {
		return nil, "", fmt.Errorf("build trigger config node: %w", err)
	}
	return node, "", nil
}

// newTriggerConfigWarning builds a trigger-scoped exportWarning for
// buildTriggerConfigNode's warning string. DedupKey is the trigger's ID, not
// the automation's: AC-42 scopes trigger-message dedup per-trigger, so two
// different triggers on the same automation that happen to produce the same
// message each keep their own warning line.
func newTriggerConfigWarning(a *Automation, trigger AutomationTrigger, kind string) exportWarning {
	return exportWarning{
		AutomationName: a.Name,
		AutomationID:   a.ID,
		DedupKey:       trigger.ID,
		Message:        fmt.Sprintf("trigger %s: %s", escapeWarningText(string(trigger.Type)), kind),
	}
}
