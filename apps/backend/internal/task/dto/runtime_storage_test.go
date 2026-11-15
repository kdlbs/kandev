package dto

import (
	"encoding/json"
	"testing"
	"time"
)

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
func TestRuntimeStorageHealthPressureFreshnessRoundTrip(t *testing.T) {
	observedAt := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	zero := int64(0)
	known := RuntimeStorageHealthDTO{
		State:              RuntimeStorageStateNormal,
		Location:           RuntimeStorageLocationLocal,
		PressureObservedAt: &observedAt,
		PressureAgeSeconds: &zero,
		Revision:           4,
		Epoch:              9,
	}
	encoded, err := json.Marshal(known)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["pressure_age_seconds"] != float64(0) {
		t.Fatalf("known zero pressure age serialized as %v", wire["pressure_age_seconds"])
	}
	if wire["pressure_observed_at"] != observedAt.Format(time.RFC3339) &&
		wire["pressure_observed_at"] != "2026-11-09T12:00:00Z" {
		t.Fatalf("pressure observed at = %v", wire["pressure_observed_at"])
	}

	unknown := RuntimeStorageHealthDTO{State: RuntimeStorageStateNormal, Location: RuntimeStorageLocationLocal}
	encoded, err = json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	wire = nil
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if _, present := wire["pressure_age_seconds"]; present {
		t.Fatal("unknown pressure age serialized as a zero claim")
	}
	if _, present := wire["pressure_observed_at"]; present {
		t.Fatal("unknown pressure observation serialized as a timestamp")
	}
}
