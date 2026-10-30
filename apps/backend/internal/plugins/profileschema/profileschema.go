// Package profileschema owns the scalar property rules of an executor provider
// profile schema: keyword parsing for the manifest check and value checking
// for the host, so the two cannot drift.
package profileschema

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"unicode/utf8"
)

// Field error codes returned by CheckValue.
const (
	CodeInvalidType     = "invalid_type"
	CodeNotInEnum       = "not_in_enum"
	CodeOutOfRange      = "out_of_range"
	CodeTooShort        = "too_short"
	CodeTooLong         = "too_long"
	CodePatternMismatch = "pattern_mismatch"
)

// Property types of the profile schema.
const (
	TypeString  = "string"
	TypeBoolean = "boolean"
	TypeNumber  = "number"
	TypeInteger = "integer"
)

const (
	// MaxLengthBound is the highest value minLength and maxLength may take.
	MaxLengthBound = 4096
	// MaxPatternChars bounds the pattern source.
	MaxPatternChars = 256
)

var (
	integerText = regexp.MustCompile(`\A-?[0-9]+\z`)
	numberText  = regexp.MustCompile(`\A-?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?\z`)
)

// Property is the parsed form of one scalar profile property.
type Property struct {
	Type      string
	Secret    bool
	Enum      []any
	Minimum   *float64
	Maximum   *float64
	MinLength *int
	MaxLength *int
	Pattern   *regexp.Regexp
	defaultV  any
	hasDef    bool
}

// ParseProperty reads the keywords of a property map and validates their
// structure and the default against the property's own constraints. The error
// text carries no property name; the caller prefixes it.
func ParseProperty(raw map[string]any) (Property, error) {
	typeName, _ := raw["type"].(string)
	property := Property{Type: typeName}
	property.Secret, _ = raw["secret"].(bool)
	property.Enum, _ = anySlice(raw["enum"])
	property.Minimum = number(raw["minimum"])
	property.Maximum = number(raw["maximum"])
	if err := property.parseLength(raw); err != nil {
		return Property{}, err
	}
	if err := property.parsePattern(raw["pattern"]); err != nil {
		return Property{}, err
	}
	if value, present := raw["default"]; present {
		if err := property.setDefault(value); err != nil {
			return Property{}, err
		}
	}
	return property, nil
}

func (p *Property) parseLength(raw map[string]any) error {
	_, hasMin := raw["minLength"]
	_, hasMax := raw["maxLength"]
	if (hasMin || hasMax) && p.Type != TypeString {
		return errors.New("minLength and maxLength require a string field")
	}
	var err error
	if p.MinLength, err = lengthKeyword("minLength", raw["minLength"], hasMin); err != nil {
		return err
	}
	if p.MaxLength, err = lengthKeyword("maxLength", raw["maxLength"], hasMax); err != nil {
		return err
	}
	if p.MinLength != nil && p.MaxLength != nil && *p.MinLength > *p.MaxLength {
		return errors.New("minLength must not exceed maxLength")
	}
	return nil
}

func lengthKeyword(name string, raw any, present bool) (*int, error) {
	if !present {
		return nil, nil
	}
	value := number(raw)
	if value == nil || *value != math.Trunc(*value) || *value < 0 || *value > MaxLengthBound {
		return nil, fmt.Errorf("%s must be an integer from 0 to %d", name, MaxLengthBound)
	}
	length := int(*value)
	return &length, nil
}

func (p *Property) parsePattern(raw any) error {
	if raw == nil {
		return nil
	}
	if p.Type != TypeString {
		return errors.New("pattern requires a string field")
	}
	source, ok := raw.(string)
	if !ok || source == "" || utf8.RuneCountInString(source) > MaxPatternChars {
		return fmt.Errorf("pattern must be a string of 1 to %d characters", MaxPatternChars)
	}
	compiled, err := regexp.Compile(`\A(?:` + source + `)\z`)
	if err != nil {
		return errors.New("pattern is not valid RE2 syntax")
	}
	p.Pattern = compiled
	return nil
}

func (p *Property) setDefault(value any) error {
	if p.Secret {
		return errors.New("a secret field cannot have a default")
	}
	text, ok := canonicalText(value, p.Type)
	if !ok {
		return errors.New("default does not match the field type")
	}
	if code := p.CheckValue(text); code != "" {
		return fmt.Errorf("default violates the field constraints (%s)", code)
	}
	p.defaultV, p.hasDef = text, true
	return nil
}

// DefaultText returns the canonical text of the default, when one is declared.
func (p Property) DefaultText() (string, bool) {
	if !p.hasDef {
		return "", false
	}
	return p.defaultV.(string), true
}

// AllowsEmpty reports whether an empty value is a real value: a string with an
// explicit minLength of 0.
func (p Property) AllowsEmpty() bool {
	return p.Type == TypeString && p.MinLength != nil && *p.MinLength == 0
}

// CheckValue returns the field error code for a non-empty stored value, or
// the empty string when the value satisfies the property.
func (p Property) CheckValue(value string) string {
	numeric, code := p.parseScalar(value)
	if code != "" {
		return code
	}
	if code := p.checkEnum(value); code != "" {
		return code
	}
	if code := p.checkRange(numeric); code != "" {
		return code
	}
	return p.checkText(value)
}

func (p Property) parseScalar(value string) (float64, string) {
	switch p.Type {
	case TypeString:
		return 0, ""
	case TypeBoolean:
		if value != "true" && value != "false" {
			return 0, CodeInvalidType
		}
		return 0, ""
	case TypeInteger:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if !integerText.MatchString(value) || err != nil {
			return 0, CodeInvalidType
		}
		return float64(parsed), ""
	case TypeNumber:
		parsed, err := strconv.ParseFloat(value, 64)
		if !numberText.MatchString(value) || err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, CodeInvalidType
		}
		return parsed, ""
	default:
		return 0, CodeInvalidType
	}
}

func (p Property) checkEnum(value string) string {
	if len(p.Enum) == 0 {
		return ""
	}
	for _, candidate := range p.Enum {
		if text, ok := canonicalText(candidate, p.Type); ok && text == value {
			return ""
		}
	}
	return CodeNotInEnum
}

func (p Property) checkRange(numeric float64) string {
	if p.Type != TypeInteger && p.Type != TypeNumber {
		return ""
	}
	if p.Minimum != nil && numeric < *p.Minimum || p.Maximum != nil && numeric > *p.Maximum {
		return CodeOutOfRange
	}
	return ""
}

func (p Property) checkText(value string) string {
	if p.Type != TypeString {
		return ""
	}
	length := utf8.RuneCountInString(value)
	if p.MinLength != nil && length < *p.MinLength {
		return CodeTooShort
	}
	if p.MaxLength != nil && length > *p.MaxLength {
		return CodeTooLong
	}
	if p.Pattern != nil && !p.Pattern.MatchString(value) {
		return CodePatternMismatch
	}
	return ""
}

func canonicalText(value any, typeName string) (string, bool) {
	switch typeName {
	case TypeString:
		text, ok := value.(string)
		return text, ok
	case TypeBoolean:
		flag, ok := value.(bool)
		return strconv.FormatBool(flag), ok
	case TypeInteger:
		return integerValueText(value)
	case TypeNumber:
		if text, ok := integerValueText(value); ok {
			return text, true
		}
		if parsed, ok := value.(float64); ok && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
			return strconv.FormatFloat(parsed, 'f', -1, 64), true
		}
	}
	return "", false
}

func integerValueText(value any) (string, bool) {
	switch typed := value.(type) {
	case int:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case float64:
		if typed == math.Trunc(typed) && math.Abs(typed) < 1<<53 {
			return strconv.FormatInt(int64(typed), 10), true
		}
	}
	return "", false
}

func number(value any) *float64 {
	var parsed float64
	switch typed := value.(type) {
	case int:
		parsed = float64(typed)
	case int64:
		parsed = float64(typed)
	case uint64:
		parsed = float64(typed)
	case float64:
		parsed = typed
	default:
		return nil
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil
	}
	return &parsed
}

func anySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = typed[index]
		}
		return out, true
	default:
		return nil, false
	}
}
