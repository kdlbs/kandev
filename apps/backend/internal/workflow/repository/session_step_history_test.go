package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestCreateHistory_RoundTrip(t *testing.T) {
	repo, sqlxDB := setupTestRepoWithDB(t)
	ctx := context.Background()

	if _, err := sqlxDB.Exec(`INSERT INTO task_sessions (id) VALUES ('sess-1')`); err != nil {
		t.Fatalf("failed to insert test session: %v", err)
	}

	fromStepID := "step-a"
	actorID := "user-1"
	history := &models.SessionStepHistory{
		SessionID:  "sess-1",
		FromStepID: &fromStepID,
		ToStepID:   "step-b",
		Trigger:    models.StepTransitionTriggerManual,
		ActorID:    &actorID,
		Metadata: map[string]interface{}{
			"signal_source":  "agent",
			"signal_summary": "did the thing",
		},
	}

	if err := repo.CreateHistory(ctx, history); err != nil {
		t.Fatalf("CreateHistory failed: %v", err)
	}
	if history.ID == 0 {
		t.Fatalf("expected CreateHistory to populate ID, got 0")
	}
	if history.CreatedAt.IsZero() {
		t.Fatalf("expected CreateHistory to populate CreatedAt")
	}

	result, err := repo.ListHistoryBySession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("ListHistoryBySession failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(result))
	}

	got := result[0]
	if got.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", got.SessionID)
	}
	if got.FromStepID == nil || *got.FromStepID != "step-a" {
		t.Errorf("FromStepID = %v, want step-a", got.FromStepID)
	}
	if got.ToStepID != "step-b" {
		t.Errorf("ToStepID = %q, want step-b", got.ToStepID)
	}
	if got.Trigger != models.StepTransitionTriggerManual {
		t.Errorf("Trigger = %q, want manual", got.Trigger)
	}
	if got.ActorID == nil || *got.ActorID != "user-1" {
		t.Errorf("ActorID = %v, want user-1", got.ActorID)
	}
	if got.Metadata["signal_source"] != "agent" {
		t.Errorf("Metadata[signal_source] = %v, want agent", got.Metadata["signal_source"])
	}
	if got.Metadata["signal_summary"] != "did the thing" {
		t.Errorf("Metadata[signal_summary] = %v, want 'did the thing'", got.Metadata["signal_summary"])
	}
}

func TestCreateHistory_NullableFromStepAndActor(t *testing.T) {
	repo, sqlxDB := setupTestRepoWithDB(t)
	ctx := context.Background()

	if _, err := sqlxDB.Exec(`INSERT INTO task_sessions (id) VALUES ('sess-2')`); err != nil {
		t.Fatalf("failed to insert test session: %v", err)
	}

	history := &models.SessionStepHistory{
		SessionID: "sess-2",
		ToStepID:  "step-first",
		Trigger:   models.StepTransitionTriggerAutoComplete,
	}

	if err := repo.CreateHistory(ctx, history); err != nil {
		t.Fatalf("CreateHistory failed: %v", err)
	}

	result, err := repo.ListHistoryBySession(ctx, "sess-2")
	if err != nil {
		t.Fatalf("ListHistoryBySession failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(result))
	}
	got := result[0]
	if got.FromStepID != nil {
		t.Errorf("FromStepID = %v, want nil", got.FromStepID)
	}
	if got.ActorID != nil {
		t.Errorf("ActorID = %v, want nil", got.ActorID)
	}
	if got.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", got.Metadata)
	}
}

func TestListHistoryBySession_OrderedByCreatedAt(t *testing.T) {
	repo, sqlxDB := setupTestRepoWithDB(t)
	ctx := context.Background()

	if _, err := sqlxDB.Exec(`INSERT INTO task_sessions (id) VALUES ('sess-3')`); err != nil {
		t.Fatalf("failed to insert test session: %v", err)
	}

	steps := []string{"step-1", "step-2", "step-3"}
	for _, to := range steps {
		h := &models.SessionStepHistory{
			SessionID: "sess-3",
			ToStepID:  to,
			Trigger:   models.StepTransitionTriggerManual,
		}
		if err := repo.CreateHistory(ctx, h); err != nil {
			t.Fatalf("CreateHistory(%s) failed: %v", to, err)
		}
	}

	result, err := repo.ListHistoryBySession(ctx, "sess-3")
	if err != nil {
		t.Fatalf("ListHistoryBySession failed: %v", err)
	}
	if len(result) != len(steps) {
		t.Fatalf("expected %d history rows, got %d", len(steps), len(result))
	}
	for i, want := range steps {
		if result[i].ToStepID != want {
			t.Errorf("result[%d].ToStepID = %q, want %q", i, result[i].ToStepID, want)
		}
	}
}

func TestListHistoryBySession_ScopedToSession(t *testing.T) {
	repo, sqlxDB := setupTestRepoWithDB(t)
	ctx := context.Background()

	if _, err := sqlxDB.Exec(`INSERT INTO task_sessions (id) VALUES ('sess-a'), ('sess-b')`); err != nil {
		t.Fatalf("failed to insert test sessions: %v", err)
	}

	for _, sid := range []string{"sess-a", "sess-b"} {
		h := &models.SessionStepHistory{
			SessionID: sid,
			ToStepID:  "step-x",
			Trigger:   models.StepTransitionTriggerManual,
		}
		if err := repo.CreateHistory(ctx, h); err != nil {
			t.Fatalf("CreateHistory failed: %v", err)
		}
	}

	result, err := repo.ListHistoryBySession(ctx, "sess-a")
	if err != nil {
		t.Fatalf("ListHistoryBySession failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 history row scoped to sess-a, got %d", len(result))
	}
	if result[0].SessionID != "sess-a" {
		t.Errorf("SessionID = %q, want sess-a", result[0].SessionID)
	}
}
