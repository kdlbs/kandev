package requiredstores

import "testing"

func TestCoordinatorReadinessRequiresRecordingTablesAndBooleanCapability(t *testing.T) {
	for _, d := range Catalog() {
		if d.ID != "coordinator" {
			continue
		}
		have := map[string]bool{}
		for _, table := range d.RequiredTables {
			have[table] = true
		}
		for _, table := range []string{
			"coordinator_turns", "coordinator_turn_snapshots", "coordinator_turn_calls",
			"coordinator_outcomes", "coordinator_feedback", "coordinator_moveback_seen",
			"coordinator_replay_results", "coordinator_dreams", "coordinator_dream_items", "coordinator_dream_ratings",
		} {
			if !have[table] {
				t.Errorf("readiness does not require recording table %s", table)
			}
		}
		for _, cap := range d.Capabilities {
			if cap == CapabilityBoolean {
				return
			}
		}
		t.Error("coordinator conformance omits the boolean capability")
		return
	}
	t.Fatal("coordinator catalog entry missing")
}
