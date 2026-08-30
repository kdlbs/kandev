package automation

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// yamlNewlineChars is YAML 1.2's line-break set, per
// docs/specs/office/requirements/automations-yaml-export.md's Definitions section:
// {U+000A, U+000D, U+0085, U+2028, U+2029}. AC-17 and AC-46 are stated against this
// set, not against "\n" alone — a CR-only, NEL-only, LS-only, or PS-only prompt still
// forces a non-literal emission and must still warn under AC-17.
const yamlNewlineChars = "\n\r\u0085\u2028\u2029"

func containsYAMLNewline(s string) bool {
	return strings.ContainsAny(s, yamlNewlineChars)
}

const (
	warnPromptInvalidUTF8    = "prompt not emitted as a block scalar: invalid UTF-8"
	warnPromptNotBlockScalar = "prompt not emitted as a block scalar"
	warnPromptRequoted       = "prompt re-quoted to preserve bytes"
)

// buildPromptNode resolves a non-empty, exported automation prompt to its final
// *yaml.Node form and an optional warning, following the ordered classification
// AC-15/16/17/46/47/49 require. Callers must not invoke this for an empty prompt:
// AC-4/AC-46 omit the `prompt` key entirely in that case, before any fidelity rule
// applies.
//
// Exactly one classification branch is taken, evaluated in this fixed order:
//  1. Not valid UTF-8 -> AC-47: emit as !!binary (base64), warn, never a failure.
//  2. Valid UTF-8, but the pinned marshaller's default-form probe does not decode
//     back byte-for-byte -> AC-49: re-probe an explicit double-quoted !!str form; if
//     that round-trips, emit it and warn "prompt re-quoted to preserve bytes"
//     (this suppresses any AC-17 warning for this prompt); if it too fails to
//     round-trip, that is a serialization failure (AC-36/AC-45) with no node.
//  3. Default form round-trips, but its emitted Style is not a literal block scalar
//     and the prompt contains a YAML newline -> AC-17: emit the default form and
//     warn, without overriding style.
//  4. Otherwise -> AC-16/AC-46: emit the default form, no warning.
//
// Both probes marshal the prompt alone through an encoder configured identically to
// the real document encoder (AC-12's 2-space indent, via newPinnedYAMLEncoder) and
// re-parse the result to read Tag/Style/Value — "observe, don't predict" per the
// spec's decision record, not an enumeration of which input characters are assumed
// to degrade.
func buildPromptNode(prompt string) (node *yaml.Node, warning string, err error) {
	if !utf8.ValidString(prompt) {
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!binary",
			Value: base64.StdEncoding.EncodeToString([]byte(prompt)),
		}, warnPromptInvalidUTF8, nil
	}

	defaultNode, err := probeScalarEmission(prompt)
	if err != nil {
		return nil, "", err
	}

	if defaultNode.Value != prompt {
		requoted := &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlStrTag, Style: yaml.DoubleQuotedStyle, Value: prompt}
		reprobed, err := probeScalarEmission(requoted)
		if err != nil {
			return nil, "", err
		}
		if reprobed.Value != prompt {
			return nil, "", fmt.Errorf("automation: prompt does not round-trip under the default or double-quoted emission form")
		}
		return requoted, warnPromptRequoted, nil
	}

	if defaultNode.Style != yaml.LiteralStyle && containsYAMLNewline(prompt) {
		return defaultNode, warnPromptNotBlockScalar, nil
	}
	return defaultNode, "", nil
}

// probeScalarEmission marshals v (a string or a *yaml.Node) through the pinned
// encoder, re-parses the result, and returns the resulting scalar node — from which
// Tag, Style, and Value can all be read. This is the probe step both branches of
// buildPromptNode share.
func probeScalarEmission(v any) (*yaml.Node, error) {
	var buf bytes.Buffer
	enc := newPinnedYAMLEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(buf.Bytes(), &doc); err != nil {
		return nil, err
	}
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return nil, fmt.Errorf("automation: pinned encoder produced an empty document")
		}
		return doc.Content[0], nil
	}
	return &doc, nil
}
