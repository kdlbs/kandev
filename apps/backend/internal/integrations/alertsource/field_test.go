package alertsource

import (
	"testing"
	"time"
)

func TestFieldKind_TagAndJSONType(t *testing.T) {
	cases := []struct {
		kind     FieldKind
		wantTag  string
		wantJSON string
	}{
		{FieldKindString, "string", "string"},
		{FieldKindInt, "int", "integer"},
		{FieldKindBool, "bool", "boolean"},
		{FieldKindDuration, "duration", "string"},
	}
	for _, c := range cases {
		if got := c.kind.tag(); got != c.wantTag {
			t.Errorf("kind %v: tag() = %q, want %q", c.kind, got, c.wantTag)
		}
		if got := c.kind.jsonType(); got != c.wantJSON {
			t.Errorf("kind %v: jsonType() = %q, want %q", c.kind, got, c.wantJSON)
		}
	}
}

func TestNewFieldConstructors_DefaultState(t *testing.T) {
	f := NewStringField("api_token")
	if f.name != "api_token" || f.kind != FieldKindString {
		t.Fatalf("unexpected field state: %+v", f)
	}
	if f.optional || f.secret || f.advanced || f.hasDefault || f.hasExample {
		t.Fatalf("new field should have no modifiers set: %+v", f)
	}
}

func TestField_ModifiersChainAndMutateInPlace(t *testing.T) {
	f := NewIntField("poll_interval").Secret().Optional().Advanced().Description("desc")
	if !f.secret || !f.optional || !f.advanced || f.description != "desc" {
		t.Fatalf("chained modifiers did not apply: %+v", f)
	}
}

func TestField_Default_ImpliesOptional(t *testing.T) {
	f := NewIntField("retries").Default(3)
	if !f.optional {
		t.Fatal("Default() should imply Optional()")
	}
	if !f.hasDefault || f.defaultVal != 3 {
		t.Fatalf("Default value not recorded: %+v", f)
	}
}

func TestField_Default_LastWriteWins(t *testing.T) {
	f := NewIntField("retries").Default(3).Default(5)
	if f.defaultVal != 5 {
		t.Fatalf("expected last Default() to win, got %v", f.defaultVal)
	}
}

func TestField_Example_LastWriteWins(t *testing.T) {
	f := NewStringField("region").Example("us-east").Example("us-west")
	if f.exampleVal != "us-west" {
		t.Fatalf("expected last Example() to win, got %v", f.exampleVal)
	}
}

func TestKindMatches(t *testing.T) {
	cases := []struct {
		name string
		kind FieldKind
		v    any
		want bool
	}{
		{"string ok", FieldKindString, "x", true},
		{"string wrong type", FieldKindString, 1, false},
		{"int ok", FieldKindInt, 5, true},
		{"int wrong type", FieldKindInt, "5", false},
		{"int rejects float64", FieldKindInt, float64(5), false},
		{"bool ok", FieldKindBool, true, true},
		{"bool wrong type", FieldKindBool, "true", false},
		{"duration ok", FieldKindDuration, 5 * time.Second, true},
		{"duration wrong type", FieldKindDuration, "5s", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := kindMatches(c.kind, c.v); got != c.want {
				t.Errorf("kindMatches(%v, %v) = %v, want %v", c.kind, c.v, got, c.want)
			}
		})
	}
}

func TestRender_DurationBecomesString(t *testing.T) {
	got := render(FieldKindDuration, 90*time.Second)
	if got != "1m30s" {
		t.Fatalf("render(duration) = %v, want %q", got, "1m30s")
	}
}

func TestRender_OtherKindsAreIdentity(t *testing.T) {
	if got := render(FieldKindString, "x"); got != "x" {
		t.Fatalf("render(string) = %v, want %q", got, "x")
	}
	if got := render(FieldKindInt, 7); got != 7 {
		t.Fatalf("render(int) = %v, want %v", got, 7)
	}
	if got := render(FieldKindBool, true); got != true {
		t.Fatalf("render(bool) = %v, want %v", got, true)
	}
}
