package optional

import (
	"encoding/json"
	"testing"
)

func TestOptionalInt_UnmarshalJSON(t *testing.T) {
	type payload struct {
		Cap Int `json:"cap"`
	}

	t.Run("absent key leaves Present false", func(t *testing.T) {
		var p payload
		if err := json.Unmarshal([]byte(`{"other":1}`), &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if p.Cap.Present {
			t.Fatalf("expected Present=false for absent key, got Present=true")
		}
		if p.Cap.Value != nil {
			t.Fatalf("expected nil Value for absent key, got %v", *p.Cap.Value)
		}
	})

	t.Run("explicit null is present with nil value", func(t *testing.T) {
		var p payload
		if err := json.Unmarshal([]byte(`{"cap":null}`), &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !p.Cap.Present {
			t.Fatalf("expected Present=true for explicit null")
		}
		if p.Cap.Value != nil {
			t.Fatalf("expected nil Value for null, got %v", *p.Cap.Value)
		}
	})

	t.Run("number is present with that value", func(t *testing.T) {
		var p payload
		if err := json.Unmarshal([]byte(`{"cap":7}`), &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !p.Cap.Present || p.Cap.Value == nil || *p.Cap.Value != 7 {
			t.Fatalf("expected Present=true Value=7, got Present=%v Value=%v", p.Cap.Present, p.Cap.Value)
		}
	})

	t.Run("non-integer is an error", func(t *testing.T) {
		var p payload
		if err := json.Unmarshal([]byte(`{"cap":"nope"}`), &p); err == nil {
			t.Fatalf("expected error for non-integer cap")
		}
	})
}
