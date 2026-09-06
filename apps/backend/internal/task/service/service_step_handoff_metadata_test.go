package service

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestProtectedTaskMetadataUpdatePreservesStepHandoffCarry(t *testing.T) {
	existing := map[string]interface{}{
		"ordinary": "old",
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "trusted handoff",
			StepID:  "step-next",
			Stamp:   "trusted-stamp",
		},
	}
	requested := map[string]interface{}{
		"ordinary": "new",
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "forged handoff",
			StepID:  "step-next",
			Stamp:   "forged-stamp",
		},
	}

	updated := protectedTaskMetadataUpdate(existing, requested)
	if got := updated["ordinary"]; got != "new" {
		t.Fatalf("ordinary metadata = %v, want new", got)
	}
	if got := updated[models.MetaKeyStepHandoffCarry]; got != existing[models.MetaKeyStepHandoffCarry] {
		t.Fatalf("carry token = %#v, want existing token", got)
	}
}

func TestProtectedTaskMetadataUpdateRejectsNewStepHandoffCarry(t *testing.T) {
	requested := map[string]interface{}{
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "forged handoff",
			StepID:  "step-next",
			Stamp:   "forged-stamp",
		},
	}

	updated := protectedTaskMetadataUpdate(nil, requested)
	if _, ok := updated[models.MetaKeyStepHandoffCarry]; ok {
		t.Fatalf("new carry token must not be accepted from client metadata")
	}
}
