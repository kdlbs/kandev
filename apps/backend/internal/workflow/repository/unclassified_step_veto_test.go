package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestUnclassifiedStepVetoRoundTrip(t *testing.T) {
	repo, db := setupTestRepoWithDB(t)
	ctx := context.Background()
	step := &models.WorkflowStep{WorkflowID: "wf-test", Name: "Working", Position: 1}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("create workflow step: %v", err)
	}
	if _, err := db.Exec(`UPDATE workflow_steps SET disable_unclassified_fallback = 1 WHERE id = ?`, step.ID); err != nil {
		t.Fatalf("set workflow step veto: %v", err)
	}
	got, err := repo.GetStep(ctx, step.ID)
	if err != nil {
		t.Fatalf("get workflow step: %v", err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal workflow step: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal workflow step: %v", err)
	}
	var veto bool
	if err := json.Unmarshal(fields["disable_unclassified_fallback"], &veto); err != nil {
		t.Fatalf("unmarshal workflow step veto: %v", err)
	}
	if !veto {
		t.Fatal("disable_unclassified_fallback = false, want true")
	}
}
