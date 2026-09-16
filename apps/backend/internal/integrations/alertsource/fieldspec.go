package alertsource

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

// fieldEntry is a copied-by-value snapshot of a *Field, taken at the moment
// it is added to a FieldSpec. Storing values (not the builder pointer) means
// nothing the caller does to their *Field afterward can affect the spec.
// isNil marks an entry produced by Field(nil): every other field is the
// zero value and carries no information of its own.
type fieldEntry struct {
	isNil       bool
	name        string
	kind        FieldKind
	secret      bool
	optional    bool
	advanced    bool
	hasDefault  bool
	defaultVal  any
	hasExample  bool
	exampleVal  any
	description string
}

// FieldSpec is a declarative configuration contract, built by chaining
// Field() calls (Benthos NewConfigSpec-style). One FieldSpec declaration
// drives both config validation (ValidateConfig/LoadConfig) and JSON Schema
// emission (Schema) — there is exactly one place a field is described.
type FieldSpec struct {
	fields []fieldEntry
}

// NewFieldSpec returns an empty FieldSpec ready for chained Field() calls.
func NewFieldSpec() *FieldSpec {
	return &FieldSpec{}
}

// Field appends f's current state to the spec and returns the spec for
// chaining. A nil f is recorded rather than panicking: FieldSpec.Validate()
// surfaces ErrCodeNilField so the mistake is reported through the same error
// channel as every other declaration problem instead of crashing a source's
// init-time registration.
func (s *FieldSpec) Field(f *Field) *FieldSpec {
	if f == nil {
		s.fields = append(s.fields, fieldEntry{isNil: true})
		return s
	}
	s.fields = append(s.fields, fieldEntry{
		name:        f.name,
		kind:        f.kind,
		secret:      f.secret,
		optional:    f.optional,
		advanced:    f.advanced,
		hasDefault:  f.hasDefault,
		defaultVal:  f.defaultVal,
		hasExample:  f.hasExample,
		exampleVal:  f.exampleVal,
		description: f.description,
	})
	return s
}

// isIdentifier reports whether s is a valid field/metadata-key identifier:
// non-empty and restricted to [A-Za-z0-9_] (D11), with no constraint on the
// first character. Shared by field-name validation (FieldSpec.Validate) and
// Descriptor.WatchMetadataKey validation (D2), which use the same alphabet
// for the same reason: both are spliced into a JSON object key / SQL path
// and must reject anything a naive non-empty check would still let through.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		isUpper := r >= 'A' && r <= 'Z'
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if !isUpper && !isLower && !isDigit && r != '_' {
			return false
		}
	}
	return true
}

// Validate checks the spec's own declaration for internal consistency: valid
// non-duplicate field names, no nil entries, Default/Example values matching
// their field's kind, and Secret fields never carrying a Default or Example
// (a literal secret value must never be capable of reaching config_json or a
// rendered schema — A7's ordering: declaration-shape errors are reported
// before any resolution is attempted). These are declaration defects, not
// bad configuration values, so they are reported as *SpecError. Errors are
// collected (not fail-fast) and returned sorted, so a spec author sees every
// problem in one pass.
func (s *FieldSpec) Validate() error {
	var errs []FieldError
	counts := map[string]int{}
	for i, e := range s.fields {
		if e.isNil {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("[%d]", i),
				Code:    ErrCodeNilField,
				Message: "field declaration is nil",
			})
			continue
		}
		if e.name == "" {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("[%d]", i),
				Code:    ErrCodeEmptyFieldName,
				Message: "field name must not be empty",
			})
			continue
		}
		if !isIdentifier(e.name) {
			errs = append(errs, FieldError{
				Field:   e.name,
				Code:    ErrCodeInvalidFieldName,
				Message: "field name must contain only letters, digits and underscores",
			})
		}
		// A7: reported once per repeated DECLARATION after the first, with
		// the 1-based declaration ordinal in Message so entries that
		// otherwise tie on Field and Code still sort deterministically.
		counts[e.name]++
		if counts[e.name] > 1 {
			errs = append(errs, FieldError{
				Field:   e.name,
				Code:    ErrCodeDuplicateField,
				Message: fmt.Sprintf("field declared more than once; this is declaration %d", counts[e.name]),
			})
		}
		errs = append(errs, validateFieldEntry(e)...)
	}
	return newSpecError(errs)
}

// validateFieldEntry checks one entry's Default/Example/Secret consistency.
func validateFieldEntry(e fieldEntry) []FieldError {
	var errs []FieldError
	if e.secret && e.hasDefault {
		errs = append(errs, FieldError{
			Field:   e.name,
			Code:    ErrCodeSecretHasLiteral,
			Message: "secret field must not declare a Default value",
		})
	}
	if e.secret && e.hasExample {
		errs = append(errs, FieldError{
			Field:   e.name,
			Code:    ErrCodeSecretHasLiteral,
			Message: "secret field must not declare an Example value",
		})
	}
	if e.secret && e.kind != FieldKindString {
		errs = append(errs, FieldError{
			Field:   e.name,
			Code:    ErrCodeSecretNonStringKind,
			Message: "secret field must be declared with NewStringField",
		})
	}
	if e.hasDefault && !kindMatches(e.kind, e.defaultVal) {
		errs = append(errs, FieldError{
			Field:   e.name,
			Code:    ErrCodeDefaultKindMismatch,
			Message: fmt.Sprintf("default value does not match field kind %s", e.kind.tag()),
		})
	}
	if e.hasExample && !kindMatches(e.kind, e.exampleVal) {
		errs = append(errs, FieldError{
			Field:   e.name,
			Code:    ErrCodeExampleKindMismatch,
			Message: fmt.Sprintf("example value does not match field kind %s", e.kind.tag()),
		})
	}
	return errs
}

// schemaRoot is the fixed-key-order root of an emitted JSON Schema document
// (D9). A struct (not a map) is used here specifically so encoding/json
// preserves this declaration order; Properties below is a map because D9
// only requires its keys to be deterministic, and encoding/json already
// sorts map keys alphabetically. Required is the only root key that is ever
// absent (D9: "omitted entirely when empty"); every other key, including
// FieldOrder and RequiredMode, is present even for a spec with zero fields.
type schemaRoot struct {
	Schema               string                    `json:"$schema"`
	Type                 string                    `json:"type"`
	AdditionalProperties bool                      `json:"additionalProperties"`
	Required             []string                  `json:"required,omitempty"`
	Properties           map[string]schemaProperty `json:"properties"`
	FieldOrder           []string                  `json:"x-kandev-field-order"`
	RequiredMode         string                    `json:"x-kandev-required-mode"`
}

// schemaProperty is one field's JSON Schema property object (D9), including
// the x-kandev-* extension keywords the frontend form renderer reads.
// Examples is Draft 7's array form ("examples": [v]), holding at most the
// one declared Example value. WriteOnly and Secret both come from the same
// Secret() modifier; a Secret field can never carry Default or Examples
// (FieldSpec.Validate rejects the declaration), so those keys are
// structurally impossible to leak for one, not merely unset for one.
type schemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Default     any    `json:"default,omitempty"`
	Examples    []any  `json:"examples,omitempty"`
	WriteOnly   bool   `json:"writeOnly,omitempty"`
	FieldKind   string `json:"x-kandev-field-kind"`
	Secret      bool   `json:"x-kandev-secret,omitempty"`
	Advanced    bool   `json:"x-kandev-advanced,omitempty"`
}

// Schema renders the spec as a JSON Schema Draft 7 document (D9). It returns
// an error rather than panicking when the spec itself is invalid — Schema
// reuses Validate() so a caller cannot render a schema for a declaration
// that ValidateConfig/LoadConfig would themselves reject as ill-formed.
func (s *FieldSpec) Schema() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	root := schemaRoot{
		Schema:       "http://json-schema.org/draft-07/schema#",
		Type:         "object",
		Properties:   map[string]schemaProperty{},
		FieldOrder:   make([]string, 0, len(s.fields)),
		RequiredMode: "create",
	}
	for _, e := range s.fields {
		root.FieldOrder = append(root.FieldOrder, e.name)
		if !e.optional {
			root.Required = append(root.Required, e.name)
		}
		prop := schemaProperty{
			Type:        e.kind.jsonType(),
			Description: e.description,
			WriteOnly:   e.secret,
			FieldKind:   e.kind.tag(),
			Secret:      e.secret,
			Advanced:    e.advanced,
		}
		if e.hasDefault {
			prop.Default = render(e.kind, e.defaultVal)
		}
		if e.hasExample {
			prop.Examples = []any{render(e.kind, e.exampleVal)}
		}
		root.Properties[e.name] = prop
	}
	sort.Strings(root.Required)
	return json.Marshal(root)
}

// coerceInt accepts the JSON representations D10 lists for an int field:
// int, int64 (already-typed input, e.g. a test constructing a Config
// directly), or a JSON number (decoded by encoding/json as float64) that is
// integral. Anything else, including a non-integral float64, is rejected.
func coerceInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		if n < math.MinInt || n > math.MaxInt {
			return 0, false
		}
		return int(n), true
	case float64:
		if n != float64(int64(n)) || n < math.MinInt || n > math.MaxInt {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

// coerceDuration accepts the JSON representations D10 lists for a duration
// field: a Go-syntax string ("30s"), or an already-typed time.Duration.
func coerceDuration(v any) (time.Duration, bool) {
	switch d := v.(type) {
	case time.Duration:
		return d, true
	case string:
		parsed, err := time.ParseDuration(d)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// coerceValue coerces a raw decoded value to kind's canonical Go
// representation, per D10's accepted-representations table. It returns
// ok=false (never an error) so callers can produce a field-scoped
// ErrCodeWrongType FieldError with their own context.
func coerceValue(kind FieldKind, v any) (any, bool) {
	switch kind {
	case FieldKindString:
		s, ok := v.(string)
		return s, ok
	case FieldKindBool:
		b, ok := v.(bool)
		return b, ok
	case FieldKindInt:
		return coerceInt(v)
	case FieldKindDuration:
		return coerceDuration(v)
	default:
		return nil, false
	}
}
