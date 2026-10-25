package models

import (
	"encoding/json"
	"testing"
)

func TestCanonicalJSONSortsKeysAndStripsWhitespace(t *testing.T) {
	got, err := CanonicalJSON([]byte(`{ "b" : 1,  "a"  : { "d": true, "c": "x" } }`))
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"a":{"c":"x","d":true},"b":1}`
	if string(got) != want {
		t.Fatalf("CanonicalJSON = %s, want %s", got, want)
	}
}

// TestCanonicalJSONPreservesNumberLiterals is the regression this helper exists
// for: a float64 round-trip renders a Unix-second timestamp in exponent form and
// silently breaks every comparison built on the result.
func TestCanonicalJSONPreservesNumberLiterals(t *testing.T) {
	raw := []byte(`{"ceiling_queued_at":1757606400,"attempts":3,"ratio":0.5}`)
	got, err := CanonicalJSON(raw)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	want := `{"attempts":3,"ceiling_queued_at":1757606400,"ratio":0.5}`
	if string(got) != want {
		t.Fatalf("CanonicalJSON = %s, want %s", got, want)
	}
}

func TestCanonicalJSONTreatsAbsentEmptyAndNullAsOneValue(t *testing.T) {
	for name, raw := range map[string][]byte{
		"nil":        nil,
		"empty":      []byte(""),
		"whitespace": []byte("  \n"),
		"null":       []byte("null"),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := CanonicalJSON(raw)
			if err != nil {
				t.Fatalf("CanonicalJSON: %v", err)
			}
			if got != nil {
				t.Fatalf("CanonicalJSON(%q) = %s, want nil", raw, got)
			}
		})
	}
}

func TestCanonicalJSONIsStableAcrossMarshalOrder(t *testing.T) {
	first, err := CanonicalJSONOf(map[string]interface{}{"z": 1, "a": 2})
	if err != nil {
		t.Fatalf("CanonicalJSONOf: %v", err)
	}
	encoded, err := json.Marshal(map[string]interface{}{"a": 2, "z": 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	second, err := CanonicalJSON(encoded)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("canonical forms differ: %s vs %s", first, second)
	}
}

func TestCanonicalJSONRejectsInvalidInput(t *testing.T) {
	if _, err := CanonicalJSON([]byte(`{"a":`)); err == nil {
		t.Fatal("CanonicalJSON accepted malformed JSON")
	}
}
