package automation

import (
	"bytes"

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

// marshalExportDocument renders a document to YAML bytes with a pinned 2-space
// indent (AC-12: package-level yaml.Marshal defaults to 4 spaces, which this
// deliberately overrides via an explicit Encoder rather than relying on the
// package-level default).
func marshalExportDocument(doc *exportDocument) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
