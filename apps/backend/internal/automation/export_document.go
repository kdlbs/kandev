package automation

import (
	"bytes"
	"io"

	"gopkg.in/yaml.v3"
)

// newExportDocument builds the top-level export document with its fixed version/type
// values (AC-40's `version`, `type` keys).
func newExportDocument(automations []exportAutomation, warnings []string) *exportDocument {
	return &exportDocument{
		Version:     exportDocumentVersion,
		Type:        exportDocumentType,
		Automations: automations,
		Warnings:    warnings,
	}
}

// newPinnedYAMLEncoder returns an Encoder configured the one way this package ever
// emits YAML: 2-space indent (AC-12; yaml.Marshal's package-level default is 4). The
// prompt-fidelity probes (export_prompt.go) reuse this so a probe's encoder
// configuration can never drift from the real document encoder's.
func newPinnedYAMLEncoder(w io.Writer) *yaml.Encoder {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc
}

// marshalExportDocument renders a document to YAML bytes with the pinned encoder.
func marshalExportDocument(doc *exportDocument) ([]byte, error) {
	var buf bytes.Buffer
	enc := newPinnedYAMLEncoder(&buf)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
