package requiredstores

import (
	"strings"
	"testing"
)

func TestTrackerRejectsUnknownDuplicateAndOutOfOrderResults(t *testing.T) {
	tracker, err := NewTracker([]Descriptor{
		{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}},
		{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}, DependsOn: []string{"first"}},
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	assertTrackerError(t, tracker.Record("missing", nil), "unknown")
	assertTrackerError(t, tracker.Record("second", nil), "out of order")
	if err := tracker.Record("first", nil); err != nil {
		t.Fatalf("Record(first) error = %v", err)
	}
	assertTrackerError(t, tracker.Record("first", nil), "duplicate")
}

func TestTrackerSnapshotUsesCatalogOrderAndDetectsFailures(t *testing.T) {
	tracker, err := NewTracker([]Descriptor{
		{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}},
		{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}, DependsOn: []string{"first"}},
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	if err := tracker.Record("first", nil); err != nil {
		t.Fatalf("Record(first) error = %v", err)
	}
	if err := tracker.Record("second", errTestStore); err != nil {
		t.Fatalf("Record(second) error = %v", err)
	}

	snapshot := tracker.Snapshot()
	if len(snapshot) != 2 || snapshot[0].ID != "first" || snapshot[1].ID != "second" {
		t.Fatalf("Snapshot() IDs = %#v, want catalog order", snapshot)
	}
	if snapshot[0].State != StateHealthy {
		t.Errorf("first state = %q, want %q", snapshot[0].State, StateHealthy)
	}
	if snapshot[1].State != StateUnhealthy || snapshot[1].Error != errTestStore.Error() {
		t.Errorf("second status = %#v, want unhealthy error", snapshot[1])
	}
	if err := tracker.ValidateComplete(); err == nil || !strings.Contains(err.Error(), "unhealthy") {
		t.Fatalf("ValidateComplete() error = %v, want unhealthy error", err)
	}
}

func TestTrackerDetectsMissingResults(t *testing.T) {
	tracker, err := NewTracker([]Descriptor{
		{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}},
		{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}},
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	if err := tracker.Record("first", nil); err != nil {
		t.Fatalf("Record(first) error = %v", err)
	}
	if err := tracker.ValidateComplete(); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("ValidateComplete() error = %v, want missing error", err)
	}
}

func TestTrackerProbeStateRecoversAndReportsUnavailableStores(t *testing.T) {
	tracker, err := NewTracker([]Descriptor{
		{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}},
		{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}},
	})
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	if err := tracker.RecordSuccess("first"); err != nil {
		t.Fatalf("RecordSuccess(first) error = %v", err)
	}
	if err := tracker.RecordSuccess("second"); err != nil {
		t.Fatalf("RecordSuccess(second) error = %v", err)
	}
	if err := tracker.RecordProbe("first", errTestStore); err != nil {
		t.Fatalf("RecordProbe(first) error = %v", err)
	}
	if tracker.AggregateState() != StateUnhealthy {
		t.Fatalf("AggregateState() = %q, want %q", tracker.AggregateState(), StateUnhealthy)
	}
	if got := tracker.UnhealthyStoreIDs(); len(got) != 1 || got[0] != "first" {
		t.Fatalf("UnhealthyStoreIDs() = %#v, want [first]", got)
	}
	if err := tracker.RecordProbe("first", nil); err != nil {
		t.Fatalf("RecordProbe recovery error = %v", err)
	}
	if !tracker.Healthy() || tracker.AggregateState() != StateHealthy {
		t.Fatalf("tracker did not recover: state=%q healthy=%v", tracker.AggregateState(), tracker.Healthy())
	}
}

var errTestStore = testError("store initialization failed")

type testError string

func (e testError) Error() string { return string(e) }

func assertTrackerError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("tracker error = %v, want %q", err, want)
	}
}
