package models

import "testing"

func TestPublicTaskMetadataRedactsStepHandoffCarry(t *testing.T) {
	metadata := map[string]interface{}{
		"ordinary": "visible",
		MetaKeyStepHandoffCarry: StepHandoffCarryToken{
			Handoff: "private handoff",
			StepID:  "step-next",
			Stamp:   "private-stamp",
		},
	}

	public := PublicTaskMetadata(metadata)
	if _, ok := public[MetaKeyStepHandoffCarry]; ok {
		t.Fatalf("public metadata must not expose %q", MetaKeyStepHandoffCarry)
	}
	if got := public["ordinary"]; got != "visible" {
		t.Fatalf("ordinary metadata = %v, want visible", got)
	}
	if _, ok := metadata[MetaKeyStepHandoffCarry]; !ok {
		t.Fatal("redaction must not mutate stored metadata")
	}
}
