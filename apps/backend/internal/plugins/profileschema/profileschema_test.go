package profileschema

import (
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, raw map[string]any) Property {
	t.Helper()
	property, err := ParseProperty(raw)
	if err != nil {
		t.Fatalf("ParseProperty(%v): %v", raw, err)
	}
	return property
}

func TestParsePropertyRejectsBadKeywords(t *testing.T) {
	cases := map[string]map[string]any{
		"minLength on integer":       {"type": "integer", "minLength": 1},
		"pattern on boolean":         {"type": "boolean", "pattern": "a"},
		"negative minLength":         {"type": "string", "minLength": -1},
		"maxLength over 4096":        {"type": "string", "maxLength": 4097},
		"minLength over maxLength":   {"type": "string", "minLength": 5, "maxLength": 4},
		"non-integer length":         {"type": "string", "maxLength": 1.5},
		"pattern does not compile":   {"type": "string", "pattern": "("},
		"pattern over 256 chars":     {"type": "string", "pattern": strings.Repeat("a", 257)},
		"pattern not a string":       {"type": "string", "pattern": 7},
		"lookahead is not RE2":       {"type": "string", "pattern": "(?=a)a"},
		"secret with default":        {"type": "string", "secret": true, "default": "x"},
		"default wrong type":         {"type": "integer", "default": "x"},
		"default outside enum":       {"type": "string", "enum": []any{"a", "b"}, "default": "c"},
		"default below minimum":      {"type": "integer", "minimum": 5, "default": 1},
		"default above maximum":      {"type": "number", "maximum": 5.5, "default": 6.0},
		"default shorter than min":   {"type": "string", "minLength": 3, "default": "ab"},
		"default longer than max":    {"type": "string", "maxLength": 2, "default": "abc"},
		"default fails pattern":      {"type": "string", "pattern": "[a-z]+", "default": "ABC"},
		"boolean default not a bool": {"type": "boolean", "default": "true"},
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProperty(raw); err == nil {
				t.Fatalf("ParseProperty(%v) accepted an invalid property", raw)
			}
		})
	}
}

func TestParsePropertyAcceptsAndCanonicalizesDefaults(t *testing.T) {
	cases := []struct {
		raw  map[string]any
		want string
	}{
		{map[string]any{"type": "string", "default": "eu-west"}, "eu-west"},
		{map[string]any{"type": "integer", "default": 8}, "8"},
		{map[string]any{"type": "integer", "default": int64(-3)}, "-3"},
		{map[string]any{"type": "number", "default": 2.5}, "2.5"},
		{map[string]any{"type": "boolean", "default": false}, "false"},
		{map[string]any{"type": "string", "pattern": "[a-z]{2}-[a-z]+", "default": "eu-west"}, "eu-west"},
	}
	for _, tc := range cases {
		property := mustParse(t, tc.raw)
		got, ok := property.DefaultText()
		if !ok || got != tc.want {
			t.Fatalf("DefaultText() = %q, %v; want %q", got, ok, tc.want)
		}
	}
	if _, ok := mustParse(t, map[string]any{"type": "string"}).DefaultText(); ok {
		t.Fatal("a property without a default reported one")
	}
}

func TestCheckValueTable(t *testing.T) {
	enum := mustParse(t, map[string]any{"type": "string", "enum": []any{"a", "b"}})
	bounded := mustParse(t, map[string]any{"type": "integer", "minimum": 1, "maximum": 10})
	short := mustParse(t, map[string]any{"type": "string", "minLength": 2, "maxLength": 3})
	pattern := mustParse(t, map[string]any{"type": "string", "pattern": "[a-z]+"})
	number := mustParse(t, map[string]any{"type": "number", "minimum": 0.5})
	boolean := mustParse(t, map[string]any{"type": "boolean"})
	integer := mustParse(t, map[string]any{"type": "integer"})
	cases := []struct {
		name     string
		property Property
		value    string
		want     string
	}{
		{"enum member", enum, "a", ""},
		{"enum miss", enum, "c", CodeNotInEnum},
		{"in range", bounded, "10", ""},
		{"above range", bounded, "11", CodeOutOfRange},
		{"below range", bounded, "0", CodeOutOfRange},
		{"unparseable integer", bounded, "ten", CodeInvalidType},
		{"integer with exponent", integer, "1e3", CodeInvalidType},
		{"integer with plus sign", integer, "+3", CodeInvalidType},
		{"integer past int64", integer, "9223372036854775808", CodeInvalidType},
		{"negative integer", integer, "-7", ""},
		{"number below minimum", number, "0.1", CodeOutOfRange},
		{"number decimal", number, "1.25", ""},
		{"number NaN", number, "NaN", CodeInvalidType},
		{"number infinity", number, "Inf", CodeInvalidType},
		{"number hex", number, "0x10", CodeInvalidType},
		{"number overflow", number, "1e999", CodeInvalidType},
		{"bool true", boolean, "true", ""},
		{"bool capitalised", boolean, "True", CodeInvalidType},
		{"too short", short, "a", CodeTooShort},
		{"too long", short, "abcd", CodeTooLong},
		{"length counts code points", short, "日本", ""},
		{"four code points", short, "日本語字", CodeTooLong},
		{"pattern match", pattern, "abc", ""},
		{"pattern must match the whole value", pattern, "abc1", CodePatternMismatch},
		{"pattern anchors at the start", pattern, "1abc", CodePatternMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.property.CheckValue(tc.value); got != tc.want {
				t.Fatalf("CheckValue(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestCheckValueWholeValueAnchorSurvivesAlternation(t *testing.T) {
	property := mustParse(t, map[string]any{"type": "string", "pattern": "a|b"})
	if got := property.CheckValue("ab"); got != CodePatternMismatch {
		t.Fatalf("alternation escaped the anchor: %q", got)
	}
	if got := property.CheckValue("b"); got != "" {
		t.Fatalf("alternation member rejected: %q", got)
	}
}

func TestPatternRunsInLinearTimeOnAMiBInput(t *testing.T) {
	property := mustParse(t, map[string]any{"type": "string", "pattern": "(a+)+b"})
	input := strings.Repeat("a", 1<<20)
	started := time.Now()
	if got := property.CheckValue(input); got != CodePatternMismatch {
		t.Fatalf("CheckValue = %q, want pattern_mismatch", got)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("hostile pattern took %s on a 1 MiB input", elapsed)
	}
}

func TestEmptyStringWithExplicitZeroMinLengthIsValid(t *testing.T) {
	property := mustParse(t, map[string]any{"type": "string", "minLength": 0})
	if !property.AllowsEmpty() {
		t.Fatal("explicit minLength 0 must keep an empty value")
	}
	if mustParse(t, map[string]any{"type": "string"}).AllowsEmpty() {
		t.Fatal("a string without an explicit minLength treats empty as absent")
	}
}
