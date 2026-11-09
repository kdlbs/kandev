package turnchanges

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProvenanceHeadBindingJSONRoundTrip(t *testing.T) {
	id := uuid.NewString()
	provenance := Provenance{
		Version: ProvenanceVersion2, ChangeSetID: uuid.NewString(), CheckoutID: uuid.NewString(),
		Boundary: CheckpointStart, HashAlgorithm: "sha1", HeadRef: "refs/heads/feature/provisional",
		HeadBindingID: id,
	}
	encoded, err := json.Marshal(provenance)
	if err != nil {
		t.Fatalf("marshal provenance: %v", err)
	}
	if !strings.Contains(string(encoded), `"head_binding_id":"`+id+`"`) {
		t.Fatalf("encoded provenance = %s, want head_binding_id", encoded)
	}
	decoded := Provenance{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal provenance: %v", err)
	}
	if !decoded.Valid() || decoded.HeadBindingID != id || decoded.HeadRef != provenance.HeadRef {
		t.Fatalf("round-trip provenance = %+v", decoded)
	}

	withoutBinding := provenance
	withoutBinding.HeadBindingID = ""
	encoded, err = json.Marshal(withoutBinding)
	if err != nil {
		t.Fatalf("marshal provenance without binding: %v", err)
	}
	if strings.Contains(string(encoded), "head_binding_id") {
		t.Fatalf("encoded legacy provenance = %s, want omitted binding field", encoded)
	}
}

func TestSameBranchBindingRequiresMatchingCanonicalUUID(t *testing.T) {
	valid := uuid.NewString()
	tests := []struct {
		name  string
		start *Provenance
		end   *Provenance
		want  bool
	}{
		{name: "nil endpoints", start: nil, end: nil, want: false},
		{name: "missing ids", start: &Provenance{}, end: &Provenance{}, want: false},
		{name: "one missing id", start: &Provenance{HeadBindingID: valid}, end: &Provenance{}, want: false},
		{name: "malformed ids", start: &Provenance{HeadBindingID: "not-a-uuid"}, end: &Provenance{HeadBindingID: "not-a-uuid"}, want: false},
		{name: "uppercase ids", start: &Provenance{HeadBindingID: strings.ToUpper(valid)}, end: &Provenance{HeadBindingID: strings.ToUpper(valid)}, want: false},
		{name: "different ids", start: &Provenance{HeadBindingID: valid}, end: &Provenance{HeadBindingID: uuid.NewString()}, want: false},
		{name: "same canonical id", start: &Provenance{HeadBindingID: valid}, end: &Provenance{HeadBindingID: valid}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameBranchBinding(tc.start, tc.end); got != tc.want {
				t.Fatalf("SameBranchBinding() = %t, want %t", got, tc.want)
			}
		})
	}
}
