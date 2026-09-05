package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestTakeTaskMetadataKeyIfDestinationStep_MatchClaimsAndReturns covers the
// happy path: a token naming the step being entered, with the stamp read
// alongside it, is removed and its content returned.
func TestTakeTaskMetadataKeyIfDestinationStep_MatchClaimsAndReturns(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "watch the migration",
			StepID:  "step-b",
			Stamp:   "stamp-1",
		},
	})
	ctx := context.Background()

	raw, ok, err := repo.TakeTaskMetadataKeyIfDestinationStep(
		ctx, casTaskID, models.MetaKeyStepHandoffCarry, "step-b", "stamp-1",
	)
	if err != nil {
		t.Fatalf("TakeTaskMetadataKeyIfDestinationStep: %v", err)
	}
	if !ok {
		t.Fatal("expected the matching token to be claimed")
	}
	var token models.StepHandoffCarryToken
	if err := json.Unmarshal(raw, &token); err != nil {
		t.Fatalf("unmarshal claimed token: %v", err)
	}
	if token.Handoff != "watch the migration" || token.StepID != "step-b" || token.Stamp != "stamp-1" {
		t.Fatalf("claimed token = %#v, want the seeded token", token)
	}

	if _, found := metadataValue(t, repo, models.MetaKeyStepHandoffCarry); found {
		t.Fatal("claimed token must be removed from task metadata")
	}
}

// TestTakeTaskMetadataKeyIfDestinationStep_WrongStepLeavesTokenInPlace
// covers AC-001.5: a token naming a different step is left untouched so a
// later entry into the step it does name can still receive it.
func TestTakeTaskMetadataKeyIfDestinationStep_WrongStepLeavesTokenInPlace(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "for step C",
			StepID:  "step-c",
			Stamp:   "stamp-2",
		},
	})
	ctx := context.Background()

	raw, ok, err := repo.TakeTaskMetadataKeyIfDestinationStep(
		ctx, casTaskID, models.MetaKeyStepHandoffCarry, "step-b", "stamp-2",
	)
	if err != nil {
		t.Fatalf("TakeTaskMetadataKeyIfDestinationStep: %v", err)
	}
	if ok || raw != nil {
		t.Fatalf("expected no claim for a token naming another step, got ok=%v raw=%s", ok, raw)
	}

	stored, found := metadataValue(t, repo, models.MetaKeyStepHandoffCarry)
	if !found {
		t.Fatal("token naming another step must remain in place")
	}
	storedMap, _ := stored.(map[string]interface{})
	if storedMap["step_id"] != "step-c" {
		t.Fatalf("stored token = %#v, want step-c untouched", stored)
	}
}

// TestTakeTaskMetadataKeyIfDestinationStep_StaleStampLeavesReplacementInPlace
// covers the same-step race the stamp compare exists for: a caller holding a
// stale stamp for the CORRECT step must not remove a token that was replaced
// in between, and must not deliver the stale (already-superseded) text.
func TestTakeTaskMetadataKeyIfDestinationStep_StaleStampLeavesReplacementInPlace(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{
		models.MetaKeyStepHandoffCarry: models.StepHandoffCarryToken{
			Handoff: "stale text",
			StepID:  "step-b",
			Stamp:   "stamp-old",
		},
	})
	ctx := context.Background()

	// Simulate a replacement landing between the caller's advisory read and
	// its claim attempt: same step, new stamp, new text.
	if err := repo.SetTaskMetadataKey(ctx, casTaskID, models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
		Handoff: "fresh text",
		StepID:  "step-b",
		Stamp:   "stamp-new",
	}); err != nil {
		t.Fatalf("simulate replacement: %v", err)
	}

	raw, ok, err := repo.TakeTaskMetadataKeyIfDestinationStep(
		ctx, casTaskID, models.MetaKeyStepHandoffCarry, "step-b", "stamp-old",
	)
	if err != nil {
		t.Fatalf("TakeTaskMetadataKeyIfDestinationStep: %v", err)
	}
	if ok || raw != nil {
		t.Fatalf("expected no claim against a stale stamp, got ok=%v raw=%s", ok, raw)
	}

	stored, found := metadataValue(t, repo, models.MetaKeyStepHandoffCarry)
	if !found {
		t.Fatal("the replacement token must survive a stale-stamp claim attempt")
	}
	storedMap, _ := stored.(map[string]interface{})
	if storedMap["stamp"] != "stamp-new" {
		t.Fatalf("stored token = %#v, want the replacement (stamp-new) untouched", stored)
	}
}

// TestTakeTaskMetadataKeyIfDestinationStep_NoTokenIsANoOp covers the empty
// case: nothing is stored, nothing is claimed, no error.
func TestTakeTaskMetadataKeyIfDestinationStep_NoTokenIsANoOp(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	seedMetadataCASTask(t, repo, map[string]interface{}{})
	ctx := context.Background()

	raw, ok, err := repo.TakeTaskMetadataKeyIfDestinationStep(
		ctx, casTaskID, models.MetaKeyStepHandoffCarry, "step-b", "stamp-1",
	)
	if err != nil {
		t.Fatalf("TakeTaskMetadataKeyIfDestinationStep: %v", err)
	}
	if ok || raw != nil {
		t.Fatalf("expected no claim when no token exists, got ok=%v raw=%s", ok, raw)
	}
}
