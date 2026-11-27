package dto

import (
	"encoding/json"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestWorkflowStepProjectionRetainsUnclassifiedVeto(t *testing.T) {
	var step wfmodels.WorkflowStep
	if err := json.Unmarshal([]byte(`{"id":"step-1","disable_unclassified_fallback":true}`), &step); err != nil {
		t.Fatalf("unmarshal workflow step: %v", err)
	}
	encoded, err := json.Marshal(FromWorkflowStep(&step))
	if err != nil {
		t.Fatalf("marshal projected workflow step: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal projected workflow step: %v", err)
	}
	var veto bool
	if err := json.Unmarshal(fields["disable_unclassified_fallback"], &veto); err != nil {
		t.Fatalf("unmarshal projected veto: %v", err)
	}
	if !veto {
		t.Fatal("projected disable_unclassified_fallback = false, want true")
	}
}
