package alertsource

import "time"

// FieldKind is the declared type of a configuration field.
type FieldKind int

// The four declarable field kinds.
const (
	FieldKindString FieldKind = iota
	FieldKindInt
	FieldKindBool
	FieldKindDuration
)

// tag is the x-kandev-field-kind value for the kind (A3). It matches the
// constructor's kind name: NewIntField -> "int". It deliberately differs
// from jsonType for Int (whose JSON Schema "type" is "integer") and for
// Duration (whose JSON Schema "type" is "string") — the extension names the
// DECLARED kind, "type" names the JSON shape.
func (k FieldKind) tag() string {
	switch k {
	case FieldKindString:
		return "string"
	case FieldKindInt:
		return "int"
	case FieldKindBool:
		return "bool"
	case FieldKindDuration:
		return "duration"
	default:
		return ""
	}
}

// jsonType is the JSON Schema "type" keyword for the kind (D9/A3).
func (k FieldKind) jsonType() string {
	switch k {
	case FieldKindString, FieldKindDuration:
		return "string"
	case FieldKindInt:
		return "integer"
	case FieldKindBool:
		return "boolean"
	default:
		return ""
	}
}

// Field is a single configuration field declaration, built through one of
// the NewXField constructors and its chained modifiers. Fields are REQUIRED
// by default; Optional() or Default() relaxes that.
//
// A *Field is a construction-time-only handle: FieldSpec.Field copies *f by
// value into the spec, so a pointer obtained from a constructor stops
// aliasing anything inside the spec once it has been passed there. Mutating
// a builder pointer after calling FieldSpec.Field on it, or sharing one
// concurrently with any other method, is undefined.
type Field struct {
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

func newField(name string, kind FieldKind) *Field {
	return &Field{name: name, kind: kind}
}

// NewStringField declares a string-valued field.
func NewStringField(name string) *Field { return newField(name, FieldKindString) }

// NewIntField declares an integer-valued field.
func NewIntField(name string) *Field { return newField(name, FieldKindInt) }

// NewBoolField declares a boolean-valued field.
func NewBoolField(name string) *Field { return newField(name, FieldKindBool) }

// NewDurationField declares a Go-syntax duration field (e.g. "30s", "5m").
func NewDurationField(name string) *Field { return newField(name, FieldKindDuration) }

// Secret marks the field as sensitive: its value is stored through the
// shared secret store and never written into config_json or emitted
// configuration. Legal only on a string field — declaring it on any other
// kind is a declaration error surfaced by FieldSpec.Validate(), not here,
// because a chained builder cannot itself return an error mid-chain.
func (f *Field) Secret() *Field {
	f.secret = true
	return f
}

// Optional marks the field as not required. Idempotent.
func (f *Field) Optional() *Field {
	f.optional = true
	return f
}

// Advanced marks the field as advanced (surfaced by x-kandev-advanced in the
// emitted schema, for a collapsed section in the rendered form). Idempotent.
func (f *Field) Advanced() *Field {
	f.advanced = true
	return f
}

// Description sets the field's human-readable description. Last-write-wins.
func (f *Field) Description(s string) *Field {
	f.description = s
	return f
}

// Default sets the field's default value and implies Optional. The value
// must match the field's kind (string, int, bool, time.Duration); a
// mismatch, and declaring a default on a Secret field, are both declaration
// errors surfaced by FieldSpec.Validate(). Last-write-wins.
func (f *Field) Default(v any) *Field {
	f.defaultVal = v
	f.hasDefault = true
	f.optional = true
	return f
}

// Example sets the field's example value, shown in the emitted schema and
// documentation. The value must match the field's kind; declaring an
// example on a Secret field is a declaration error surfaced by
// FieldSpec.Validate(). Last-write-wins.
func (f *Field) Example(v any) *Field {
	f.exampleVal = v
	f.hasExample = true
	return f
}

// kindMatches reports whether v is the Go value type the kind declares.
func kindMatches(kind FieldKind, v any) bool {
	switch kind {
	case FieldKindString:
		_, ok := v.(string)
		return ok
	case FieldKindInt:
		_, ok := v.(int)
		return ok
	case FieldKindBool:
		_, ok := v.(bool)
		return ok
	case FieldKindDuration:
		_, ok := v.(time.Duration)
		return ok
	default:
		return false
	}
}

// render turns a declared Go value into its portable (JSON-safe) form. It is
// the single per-kind function D4/D9 require: identity for
// String/Int/Bool, and time.Duration.String() for Duration — used both when
// emitting a schema's default/examples and when marshalling a resolved
// Duration value into config_json, so the two call sites cannot diverge.
func render(kind FieldKind, v any) any {
	if kind == FieldKindDuration {
		if d, ok := v.(time.Duration); ok {
			return d.String()
		}
	}
	return v
}
