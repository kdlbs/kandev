package service

import (
	"encoding/json"
	"testing"
)

func TestCleanupInventoryRepairEvidenceSurvivesProgressSave(t *testing.T) {
	const evidence = `{"operation_id":"repair","predecessor_job_id":"old","observed_at":"2026-09-28T12:00:00Z","absent_worktree_ids":["gone"]}`
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(`{"inventory_repair":`+evidence+`}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.OrphanReapRoots = append(snapshot.OrphanReapRoots, "cleaned")
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]json.RawMessage
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if string(saved["inventory_repair"]) != evidence {
		t.Fatal("cleanup progress save discarded repair provenance")
	}
}
