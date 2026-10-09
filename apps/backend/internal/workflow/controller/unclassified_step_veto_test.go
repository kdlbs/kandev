package controller

import (
	"encoding/json"
	"testing"
)

func TestUnclassifiedStepVetoUpdatePresence(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    *bool
		wantErr bool
	}{
		{name: "omitted preserves", body: `{"id":"step-1"}`},
		{name: "false clears", body: `{"id":"step-1","disable_unclassified_fallback":false}`, want: boolPointer(false)},
		{name: "true sets", body: `{"id":"step-1","disable_unclassified_fallback":true}`, want: boolPointer(true)},
		{name: "null is invalid", body: `{"id":"step-1","disable_unclassified_fallback":null}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request UpdateStepRequest
			err := json.Unmarshal([]byte(tt.body), &request)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unmarshal error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			encoded, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}
			raw, present := fields["disable_unclassified_fallback"]
			if tt.want == nil {
				if present {
					t.Fatalf("field should be omitted, got %s", raw)
				}
				return
			}
			if !present {
				t.Fatal("field presence was lost")
			}
			var got bool
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal field: %v", err)
			}
			if got != *tt.want {
				t.Fatalf("field = %v, want %v", got, *tt.want)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }
