package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresTakeTaskMetadataKeyIfDestinationStepUsesJSONB proves
// TakeTaskMetadataKeyIfDestinationStep's dialect.IsPostgres branch
// (jsonb_extract_path_text / #-) behaves the same as the SQLite json_extract
// path already covered in step_handoff_carry_claim_test.go: a matching
// step_id and stamp claims and returns the token, a token naming another
// step is left in place, and a stale stamp on the same step leaves a
// same-step replacement token in place.
func TestPostgresTakeTaskMetadataKeyIfDestinationStepUsesJSONB(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	seed := func(taskID string, metadata string) {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO tasks (id, title, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`), taskID, "Postgres CAS task", metadata, now, now); err != nil {
			t.Fatalf("seed task %s: %v", taskID, err)
		}
	}

	t.Run("match claims and returns", func(t *testing.T) {
		const taskID = "task-pg-handoff-carry-match"
		seed(taskID, `{"step_handoff_carry":{"handoff":"watch the migration","step_id":"step-b","stamp":"stamp-1"}}`)

		raw, ok, err := repo.TakeTaskMetadataKeyIfDestinationStep(
			ctx, taskID, models.MetaKeyStepHandoffCarry, "step-b", "stamp-1",
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

		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("reload task: %v", err)
		}
		if _, present := task.Metadata[models.MetaKeyStepHandoffCarry]; present {
			t.Fatal("claimed token must be removed from task metadata")
		}
	})

	t.Run("wrong step leaves token in place", func(t *testing.T) {
		const taskID = "task-pg-handoff-carry-wrong-step"
		seed(taskID, `{"step_handoff_carry":{"handoff":"for step C","step_id":"step-c","stamp":"stamp-2"}}`)

		raw, ok, err := repo.TakeTaskMetadataKeyIfDestinationStep(
			ctx, taskID, models.MetaKeyStepHandoffCarry, "step-b", "stamp-2",
		)
		if err != nil {
			t.Fatalf("TakeTaskMetadataKeyIfDestinationStep: %v", err)
		}
		if ok || raw != nil {
			t.Fatalf("expected no claim for a token naming another step, got ok=%v raw=%s", ok, raw)
		}

		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("reload task: %v", err)
		}
		stored, _ := task.Metadata[models.MetaKeyStepHandoffCarry].(map[string]interface{})
		if stored["step_id"] != "step-c" {
			t.Fatalf("stored token = %#v, want step-c untouched", stored)
		}
	})

	t.Run("stale stamp leaves replacement in place", func(t *testing.T) {
		const taskID = "task-pg-handoff-carry-stale-stamp"
		seed(taskID, `{"step_handoff_carry":{"handoff":"stale text","step_id":"step-b","stamp":"stamp-old"}}`)

		// Simulate a replacement landing between the caller's advisory read
		// and its claim attempt: same step, new stamp, new text.
		if err := repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyStepHandoffCarry, models.StepHandoffCarryToken{
			Handoff: "fresh text",
			StepID:  "step-b",
			Stamp:   "stamp-new",
		}); err != nil {
			t.Fatalf("simulate replacement: %v", err)
		}

		raw, ok, err := repo.TakeTaskMetadataKeyIfDestinationStep(
			ctx, taskID, models.MetaKeyStepHandoffCarry, "step-b", "stamp-old",
		)
		if err != nil {
			t.Fatalf("TakeTaskMetadataKeyIfDestinationStep: %v", err)
		}
		if ok || raw != nil {
			t.Fatalf("expected no claim against a stale stamp, got ok=%v raw=%s", ok, raw)
		}

		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("reload task: %v", err)
		}
		stored, _ := task.Metadata[models.MetaKeyStepHandoffCarry].(map[string]interface{})
		if stored["stamp"] != "stamp-new" {
			t.Fatalf("stored token = %#v, want the replacement (stamp-new) untouched", stored)
		}
	})
}
