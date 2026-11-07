package externalcatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format is the encoding of a catalog document.
type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// Stable LoadError reasons. They are safe to log; file content, parser
// messages and the file path are never part of a LoadError.
const (
	ReasonUnreadable           = "unreadable"
	ReasonUnsupportedExtension = "unsupported_extension"
	ReasonMalformed            = "malformed"
	ReasonUnknownField         = "unknown_field"
	ReasonUnsupportedVersion   = "unsupported_version"
	ReasonMissingField         = "missing_field"
	ReasonInvalidValue         = "invalid_value"
	ReasonDuplicateID          = "duplicate_id"
	ReasonInvalidPattern       = "invalid_pattern"
	ReasonInvalidDuration      = "invalid_duration"
)

// LoadError reports why a catalog was rejected. A rejected catalog is never
// partially applied. Index is the zero-based rule position, or -1 when the
// error is not about one rule.
type LoadError struct {
	Reason string
	Index  int
	RuleID string
	Field  string
}

func (e *LoadError) Error() string {
	msg := "provider error catalog rejected: " + e.Reason
	if e.Index >= 0 {
		msg += fmt.Sprintf(" (rule %d", e.Index)
		if e.RuleID != "" {
			msg += " " + e.RuleID
		}
		msg += ")"
	}
	if e.Field != "" {
		msg += " field " + e.Field
	}
	return msg
}

func documentError(reason string) *LoadError {
	return &LoadError{Reason: reason, Index: -1}
}

// FormatForPath selects the document format from the file extension.
func FormatForPath(path string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return FormatJSON, true
	case ".yaml", ".yml":
		return FormatYAML, true
	default:
		return "", false
	}
}

// Load reads and compiles the catalog at path. An empty path performs no
// file I/O and returns a nil catalog: the caller keeps its built-in rules
// only. Any error rejects the whole file.
func Load(path string, vocab Vocabulary) (*Catalog, error) {
	if path == "" {
		return nil, nil
	}
	format, ok := FormatForPath(path)
	if !ok {
		return nil, documentError(ReasonUnsupportedExtension)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, documentError(ReasonUnreadable)
	}
	return Parse(data, format, vocab)
}

// Parse decodes and compiles one catalog document.
func Parse(data []byte, format Format, vocab Vocabulary) (*Catalog, error) {
	var doc documentDTO
	var err error
	switch format {
	case FormatJSON:
		err = decodeJSON(data, &doc)
	case FormatYAML:
		err = decodeYAML(data, &doc)
	default:
		return nil, documentError(ReasonUnsupportedExtension)
	}
	if err != nil {
		return nil, err
	}
	return compile(doc, vocab)
}

func decodeJSON(data []byte, doc *documentDTO) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(doc); err != nil {
		return decodeError(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return documentError(ReasonMalformed)
	}
	return nil
}

func decodeYAML(data []byte, doc *documentDTO) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(doc); err != nil {
		return decodeError(err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return documentError(ReasonMalformed)
	}
	return nil
}

// decodeError maps a decoder failure to a stable reason without retaining
// the decoder message, which can quote file content.
func decodeError(err error) *LoadError {
	msg := err.Error()
	if strings.Contains(msg, "unknown field") || strings.Contains(msg, "not found in type") {
		return documentError(ReasonUnknownField)
	}
	return documentError(ReasonMalformed)
}
