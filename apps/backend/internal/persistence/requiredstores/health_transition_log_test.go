package requiredstores

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/startup"
)

func TestHealthLogTransitionEmitsOnlyChangedSnapshot(t *testing.T) {
	tracker, err := NewTracker([]Descriptor{
		{ID: "store-z", OwnerPackage: "owner/z", RequiredTables: []string{"table_z"}, Sweep: startup.StepStoresRepositories},
		{ID: "store-a", OwnerPackage: "owner/a", RequiredTables: []string{"table_a"}, Sweep: startup.StepStoresRepositories},
	})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	for _, id := range []string{"store-z", "store-a"} {
		if err := tracker.RecordSuccess(id); err != nil {
			t.Fatalf("RecordSuccess(%q): %v", id, err)
		}
	}
	core, observed := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	health := NewHealth(tracker, nil, log)

	health.logTransition()
	health.logTransition()
	if got := observed.FilterMessage("required persistence state updated").Len(); got != 1 {
		t.Fatalf("unchanged healthy transition entries = %d, want 1", got)
	}

	if err := tracker.RecordProbe("store-z", errors.New("writer busy")); err != nil {
		t.Fatalf("RecordProbe(store-z): %v", err)
	}
	health.logTransition()
	health.logTransition()
	if got := observed.FilterMessage("required persistence state updated").Len(); got != 2 {
		t.Fatalf("unchanged failure transition entries = %d, want 2", got)
	}

	if err := tracker.RecordProbe("store-a", errors.New("table missing")); err != nil {
		t.Fatalf("RecordProbe(store-a): %v", err)
	}
	health.logTransition()
	entries := observed.FilterMessage("required persistence state updated").All()
	if len(entries) != 3 {
		t.Fatalf("changed failing-store set entries = %d, want 3", len(entries))
	}
	fields := entries[2].ContextMap()
	storeIDs, ok := fields["store_ids"].([]interface{})
	if !ok || len(storeIDs) != 2 || storeIDs[0] != "store-a" || storeIDs[1] != "store-z" {
		t.Fatalf("store_ids = %#v, want sorted [store-a store-z]", fields["store_ids"])
	}

	if err := tracker.RecordProbe("store-a", nil); err != nil {
		t.Fatalf("RecordProbe(recover store-a): %v", err)
	}
	health.logTransition()
	if err := tracker.RecordProbe("store-z", nil); err != nil {
		t.Fatalf("RecordProbe(recover store-z): %v", err)
	}
	health.logTransition()
	health.logTransition()
	entries = observed.FilterMessage("required persistence state updated").All()
	if len(entries) != 5 {
		t.Fatalf("transition entries after recovery = %d, want 5", len(entries))
	}
	got, ok := entries[3].ContextMap()["store_ids"].([]interface{})
	if !ok || len(got) != 1 || got[0] != "store-z" {
		t.Fatalf("partially recovered store_ids = %#v, want [store-z]", entries[3].ContextMap()["store_ids"])
	}
	if got := entries[4].ContextMap()["state"]; got != string(StateHealthy) {
		t.Fatalf("recovered state = %v, want %q", got, StateHealthy)
	}
}
