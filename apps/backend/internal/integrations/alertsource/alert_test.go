package alertsource

import (
	"testing"
	"time"
)

func TestAlert_Ended(t *testing.T) {
	firing := Alert{}
	if firing.Ended() {
		t.Fatal("zero EndsAt should mean still firing")
	}
	resolved := Alert{EndsAt: time.Now()}
	if !resolved.Ended() {
		t.Fatal("non-zero EndsAt should mean ended")
	}
}

// TestAlert_Clone_MutationIsolation is the A6 regression test: an enricher
// that mutates a scratch copy's Raw, Labels or Annotations and then fails
// must never corrupt the original the task is created from.
func TestAlert_Clone_MutationIsolation(t *testing.T) {
	original := Alert{
		Labels:      map[string]string{"severity": "critical"},
		Annotations: map[string]string{"summary": "disk full"},
		Raw:         []byte(`{"a":1}`),
	}
	clone := original.Clone()

	clone.Labels["severity"] = "mutated"
	clone.Labels["new"] = "mutated"
	clone.Annotations["summary"] = "mutated"
	clone.Raw[2] = 'X'

	if original.Labels["severity"] != "critical" {
		t.Fatalf("Labels aliased: original = %v", original.Labels)
	}
	if _, ok := original.Labels["new"]; ok {
		t.Fatalf("Labels aliased: original gained a key added only to the clone: %v", original.Labels)
	}
	if original.Annotations["summary"] != "disk full" {
		t.Fatalf("Annotations aliased: original = %v", original.Annotations)
	}
	if string(original.Raw) != `{"a":1}` {
		t.Fatalf("Raw aliased: original = %s", original.Raw)
	}
}

func TestAlert_Clone_NilFieldsStayNil(t *testing.T) {
	clone := Alert{}.Clone()
	if clone.Labels != nil {
		t.Fatalf("nil Labels should stay nil, got %#v", clone.Labels)
	}
	if clone.Annotations != nil {
		t.Fatalf("nil Annotations should stay nil, got %#v", clone.Annotations)
	}
	if clone.Raw != nil {
		t.Fatalf("nil Raw should stay nil, got %#v", clone.Raw)
	}
}
