package startup

import (
	"encoding/json"
	"testing"
)

func TestSnapshotBootIsStableAndPositive(t *testing.T) {
	r := New(nil)
	first := r.Snapshot().Boot
	second := r.Snapshot().Boot
	if first != second {
		t.Fatalf("boot changed within one process: %d -> %d", first, second)
	}
	if first <= 0 || first >= (1<<53) {
		t.Fatalf("boot = %d, want a positive integer below 2^53", first)
	}
}

func TestSnapshotSeqStartsAtZeroAndIncrementsPerStepTransition(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	if got := r.Snapshot().Seq; got != 0 {
		t.Fatalf("seq = %d, want 0 before any step transition", got)
	}
	r.BeginStep(StepStoresRepositories)
	if got := r.Snapshot().Seq; got != 1 {
		t.Fatalf("seq = %d, want 1 after one BeginStep", got)
	}
	r.EndStep(StepStoresRepositories)
	if got := r.Snapshot().Seq; got != 2 {
		t.Fatalf("seq = %d, want 2 after one EndStep", got)
	}
}

func TestSnapshotSeqUnaffectedByPhaseTransitions(t *testing.T) {
	r := New(nil)
	r.Set(BackingUpDatabase)
	r.Set(ApplyingMigrations)
	if got := r.Snapshot().Seq; got != 0 {
		t.Fatalf("seq = %d, want 0: phase transitions alone must not move it", got)
	}
}

func TestSnapshotJSONOmitsStepWhenNoneActive(t *testing.T) {
	r := New(nil)
	raw, err := json.Marshal(r.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := decoded["step"]; present {
		t.Fatalf("step must be omitted entirely with none active, got %s", raw)
	}
	for _, field := range []string{"phase", "boot", "seq", "elapsed_ms", "phase_elapsed_ms"} {
		if _, present := decoded[field]; !present {
			t.Fatalf("expected top-level field %q in %s", field, raw)
		}
	}
}

func TestStepSnapshotJSONFieldPresenceByMeasure(t *testing.T) {
	r := New(nil)
	r.Set(ApplyingMigrations)
	r.BeginStep(StepStoresRepositories)
	r.SetTotal(StepStoresRepositories, 100)
	r.Advance(StepStoresRepositories, 10)

	raw, err := json.Marshal(r.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Step map[string]json.RawMessage `json:"step"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"id", "label_key", "measure", "unit", "elapsed_ms", "done", "total", "rate_per_second", "since_advance_ms", "stalled"} {
		if _, present := decoded.Step[field]; !present {
			t.Fatalf("counted, advanced step should carry %q, got %s", field, raw)
		}
	}
}
