package alertsource

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIsIdentifier(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"api_token", true},
		{"a", true},
		{"a1", true},
		{"", false},
		{"1abc", true},
		{"Abc", true},
		{"has-dash", false},
		{"has space", false},
		{"_leading", true},
		{"trailing_", true},
	}
	for _, c := range cases {
		if got := isIdentifier(c.s); got != c.want {
			t.Errorf("isIdentifier(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestFieldSpec_Validate_ValidSpecHasNoErrors(t *testing.T) {
	spec := NewFieldSpec().
		Field(NewStringField("api_token").Secret()).
		Field(NewIntField("poll_interval").Default(30)).
		Field(NewBoolField("verify_tls").Default(true))
	if err := spec.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestFieldSpec_Validate_NilField(t *testing.T) {
	spec := NewFieldSpec().Field(nil)
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeNilField)
}

func TestFieldSpec_Validate_EmptyName(t *testing.T) {
	spec := NewFieldSpec().Field(NewStringField(""))
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeEmptyFieldName)
}

func TestFieldSpec_Validate_InvalidName(t *testing.T) {
	spec := NewFieldSpec().Field(NewStringField("Bad-Name"))
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeInvalidFieldName)
}

func TestFieldSpec_Validate_DuplicateName(t *testing.T) {
	spec := NewFieldSpec().
		Field(NewStringField("region")).
		Field(NewIntField("region"))
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeDuplicateField)
}

func TestFieldSpec_Validate_SecretWithDefaultOrExample(t *testing.T) {
	spec := NewFieldSpec().
		Field(NewStringField("token").Secret().Default("literal")).
		Field(NewStringField("other").Secret().Example("literal"))
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeSecretHasLiteral)
}

// TestFieldSpec_Validate_SecretWithDefault_ReversedModifierOrder is R5-13:
// D5 records the defect "by the modifier", which is order-dependent
// language, but Validate() checks the field's final state, so .Default(x)
// before .Secret() must be flagged exactly like .Secret() before
// .Default(x).
func TestFieldSpec_Validate_SecretWithDefault_ReversedModifierOrder(t *testing.T) {
	spec := NewFieldSpec().Field(NewStringField("token").Default("literal").Secret())
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeSecretHasLiteral)
}

func TestFieldSpec_Validate_SecretNonString(t *testing.T) {
	spec := NewFieldSpec().Field(NewIntField("bad").Secret())
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeSecretNonStringKind)
}

func TestFieldSpec_Validate_DefaultKindMismatch(t *testing.T) {
	spec := NewFieldSpec().Field(NewIntField("bad").Default("not an int"))
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeDefaultKindMismatch)
}

func TestFieldSpec_Validate_ExampleKindMismatch(t *testing.T) {
	spec := NewFieldSpec().Field(NewBoolField("bad").Example("not a bool"))
	err := assertSpecError(t, spec.Validate())
	assertHasCode(t, err, ErrCodeExampleKindMismatch)
}

func TestFieldSpec_Validate_CollectsAllErrors(t *testing.T) {
	spec := NewFieldSpec().
		Field(nil).
		Field(NewStringField("")).
		Field(NewStringField("dup")).
		Field(NewIntField("dup"))
	verr, ok := spec.Validate().(*SpecError)
	if !ok {
		t.Fatalf("expected *SpecError, got %T", spec.Validate())
	}
	if len(verr.Errors) < 3 {
		t.Fatalf("expected multiple collected errors, got %d: %+v", len(verr.Errors), verr.Errors)
	}
}

// TestFieldSpec_Validate_DuplicateField_CardinalityAndOrdinal is A7's pinned
// rule: three declarations of the same name yield exactly two DuplicateField
// errors (never one, never three), the first declaration is never flagged,
// and each flagged occurrence's Message carries its own 1-based declaration
// ordinal so the two entries — which tie on both Field and Code — still sort
// deterministically under D10's third key.
func TestFieldSpec_Validate_DuplicateField_CardinalityAndOrdinal(t *testing.T) {
	spec := NewFieldSpec().
		Field(NewStringField("a")).
		Field(NewStringField("a")).
		Field(NewStringField("a"))
	serr := assertSpecError(t, spec.Validate())

	var dups []FieldError
	for _, e := range serr.Errors {
		if e.Code == ErrCodeDuplicateField {
			dups = append(dups, e)
		}
	}
	if len(dups) != 2 {
		t.Fatalf("expected exactly 2 DuplicateField errors for 3 declarations, got %d: %+v", len(dups), dups)
	}
	for _, d := range dups {
		if d.Field != "a" {
			t.Fatalf("DuplicateField.Field must be the field name, got %q", d.Field)
		}
	}
	if dups[0].Message == dups[1].Message {
		t.Fatalf("expected distinct ordinals in Message to break the Field+Code tie, got identical: %q", dups[0].Message)
	}
}

func TestFieldSpec_Schema_InvalidSpecReturnsError(t *testing.T) {
	spec := NewFieldSpec().Field(nil)
	if _, err := spec.Schema(); err == nil {
		t.Fatal("expected error rendering schema for an invalid spec")
	}
}

func TestFieldSpec_Schema_ShapeAndDeterminism(t *testing.T) {
	spec := NewFieldSpec().
		Field(NewStringField("api_token").Secret().Description("API token")).
		Field(NewIntField("poll_interval").Default(30).Example(60)).
		Field(NewDurationField("timeout").Default(5 * time.Second)).
		Field(NewBoolField("verify_tls").Default(true).Advanced())

	b1, err := spec.Schema()
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	b2, err := spec.Schema()
	if err != nil {
		t.Fatalf("Schema() error on second call: %v", err)
	}
	if string(b1) != string(b2) {
		t.Fatalf("Schema() is not deterministic across calls:\n%s\nvs\n%s", b1, b2)
	}

	var doc map[string]any
	if err := json.Unmarshal(b1, &doc); err != nil {
		t.Fatalf("Schema() did not produce valid JSON: %v", err)
	}
	if doc["$schema"] != "http://json-schema.org/draft-07/schema#" {
		t.Fatalf("unexpected $schema: %v", doc["$schema"])
	}
	if doc["type"] != "object" {
		t.Fatalf("unexpected type: %v", doc["type"])
	}
	if doc["x-kandev-required-mode"] != "create" {
		t.Fatalf("unexpected x-kandev-required-mode: %v", doc["x-kandev-required-mode"])
	}
	if doc["additionalProperties"] != false {
		t.Fatalf("unexpected additionalProperties: %v", doc["additionalProperties"])
	}
	order, ok := doc["x-kandev-field-order"].([]any)
	if !ok || len(order) != 4 || order[0] != "api_token" {
		t.Fatalf("unexpected x-kandev-field-order: %v", doc["x-kandev-field-order"])
	}
	required, ok := doc["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "api_token" {
		t.Fatalf("unexpected required (only api_token has no Default): %v", doc["required"])
	}

	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties is not an object: %v", doc["properties"])
	}
	tokenProp, ok := props["api_token"].(map[string]any)
	if !ok {
		t.Fatalf("missing api_token property")
	}
	if tokenProp["x-kandev-secret"] != true {
		t.Fatalf("expected api_token to be marked x-kandev-secret: %v", tokenProp)
	}
	if tokenProp["writeOnly"] != true {
		t.Fatalf("expected api_token to be marked writeOnly: %v", tokenProp)
	}
	if _, hasDefault := tokenProp["default"]; hasDefault {
		t.Fatalf("secret field must never emit a default in the schema: %v", tokenProp)
	}
	if _, hasExamples := tokenProp["examples"]; hasExamples {
		t.Fatalf("secret field must never emit examples in the schema: %v", tokenProp)
	}

	timeoutProp, ok := props["timeout"].(map[string]any)
	if !ok {
		t.Fatalf("missing timeout property")
	}
	if timeoutProp["type"] != "string" {
		t.Fatalf("duration field must render JSON type string, got %v", timeoutProp["type"])
	}
	if timeoutProp["x-kandev-field-kind"] != "duration" {
		t.Fatalf("unexpected x-kandev-field-kind: %v", timeoutProp["x-kandev-field-kind"])
	}
	if timeoutProp["default"] != "5s" {
		t.Fatalf("duration default must render as Go-syntax string, got %v", timeoutProp["default"])
	}

	verifyProp, ok := props["verify_tls"].(map[string]any)
	if !ok {
		t.Fatalf("missing verify_tls property")
	}
	if verifyProp["x-kandev-advanced"] != true {
		t.Fatalf("expected verify_tls to be marked x-kandev-advanced: %v", verifyProp)
	}

	intervalProp, ok := props["poll_interval"].(map[string]any)
	if !ok {
		t.Fatalf("missing poll_interval property")
	}
	if intervalProp["type"] != "integer" {
		t.Fatalf("int field must render JSON type integer, got %v", intervalProp["type"])
	}
	if intervalProp["x-kandev-field-kind"] != "int" {
		t.Fatalf("unexpected x-kandev-field-kind for int: %v", intervalProp["x-kandev-field-kind"])
	}
	examples, ok := intervalProp["examples"].([]any)
	if !ok || len(examples) != 1 || examples[0] != float64(60) {
		t.Fatalf("expected examples to be the Draft 7 array form [60], got %v", intervalProp["examples"])
	}
}

// TestFieldSpec_Schema_EmptySpecStillWellFormed is D9's "the empty spec still
// emits a well-formed root" clause: every root key except the
// omission-conditional "required" is present even with zero declared fields.
func TestFieldSpec_Schema_EmptySpecStillWellFormed(t *testing.T) {
	spec := NewFieldSpec()
	b, err := spec.Schema()
	if err != nil {
		t.Fatalf("Schema() error on empty spec: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("Schema() did not produce valid JSON: %v", err)
	}
	if _, present := doc["required"]; present {
		t.Fatalf("required must be omitted entirely when empty, got %v", doc["required"])
	}
	order, ok := doc["x-kandev-field-order"].([]any)
	if !ok || len(order) != 0 {
		t.Fatalf("x-kandev-field-order must be present and empty, got %v", doc["x-kandev-field-order"])
	}
	props, ok := doc["properties"].(map[string]any)
	if !ok || len(props) != 0 {
		t.Fatalf("properties must be present and empty, got %v", doc["properties"])
	}
	if doc["x-kandev-required-mode"] != "create" {
		t.Fatalf("x-kandev-required-mode must still be present, got %v", doc["x-kandev-required-mode"])
	}
	if doc["additionalProperties"] != false {
		t.Fatalf("additionalProperties must still be present, got %v", doc["additionalProperties"])
	}
}

func TestCoerceValue_String(t *testing.T) {
	v, ok := coerceValue(FieldKindString, "hello")
	if !ok || v != "hello" {
		t.Fatalf("coerceValue(string) = (%v, %v)", v, ok)
	}
	if _, ok := coerceValue(FieldKindString, 5); ok {
		t.Fatal("expected int to be rejected for a string field")
	}
}

func TestCoerceValue_Bool(t *testing.T) {
	v, ok := coerceValue(FieldKindBool, true)
	if !ok || v != true {
		t.Fatalf("coerceValue(bool) = (%v, %v)", v, ok)
	}
	if _, ok := coerceValue(FieldKindBool, "true"); ok {
		t.Fatal("expected string to be rejected for a bool field")
	}
}

func TestCoerceValue_Int(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{"go int", 42, 42, true},
		{"json number", float64(42), 42, true},
		{"non-integral float", 42.5, 0, false},
		{"string", "42", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := coerceValue(FieldKindInt, c.in)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestCoerceValue_Duration(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want time.Duration
		ok   bool
	}{
		{"go duration", 5 * time.Second, 5 * time.Second, true},
		{"go-syntax string", "5s", 5 * time.Second, true},
		{"invalid string", "not a duration", 0, false},
		{"bare number rejected", float64(5), 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := coerceValue(FieldKindDuration, c.in)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

// assertValidationError fails the test if err is not a non-nil
// *ValidationError, and returns it for further inspection.
func assertValidationError(t *testing.T, err error) *ValidationError {
	t.Helper()
	verr, ok := err.(*ValidationError)
	if !ok || verr == nil {
		t.Fatalf("expected *ValidationError, got %#v", err)
	}
	return verr
}

// assertSpecError fails the test if err is not a non-nil *SpecError, and
// returns it for further inspection.
func assertSpecError(t *testing.T, err error) *SpecError {
	t.Helper()
	serr, ok := err.(*SpecError)
	if !ok || serr == nil {
		t.Fatalf("expected *SpecError, got %#v", err)
	}
	return serr
}

// assertHasCode fails the test if none of serr's errors carry code.
func assertHasCode(t *testing.T, serr *SpecError, code ErrorCode) {
	t.Helper()
	for _, e := range serr.Errors {
		if e.Code == code {
			return
		}
	}
	t.Fatalf("expected an error with code %q, got %+v", code, serr.Errors)
}
