package dto

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskPlanFromModelIncludesImplementationMarker(t *testing.T) {
	startedAt := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	sessionID := "session-1"
	actor := "user"

	out := TaskPlanFromModel(&models.TaskPlan{
		ID:                             "plan-1",
		TaskID:                         "task-1",
		Title:                          "Plan",
		Content:                        "Implement it",
		CreatedBy:                      "user",
		CreatedAt:                      startedAt.Add(-time.Hour),
		UpdatedAt:                      startedAt,
		ImplementationStartedAt:        &startedAt,
		ImplementationStartedSessionID: &sessionID,
		ImplementationStartedBy:        &actor,
	})

	if out.ImplementationStartedAt == nil || !out.ImplementationStartedAt.Equal(startedAt) {
		t.Fatalf("expected implementation_started_at %s, got %v", startedAt, out.ImplementationStartedAt)
	}
	if out.ImplementationStartedSessionID == nil || *out.ImplementationStartedSessionID != sessionID {
		t.Fatalf("expected implementation_started_session_id %q, got %v", sessionID, out.ImplementationStartedSessionID)
	}
	if out.ImplementationStartedBy == nil || *out.ImplementationStartedBy != actor {
		t.Fatalf("expected implementation_started_by %q, got %v", actor, out.ImplementationStartedBy)
	}
}
